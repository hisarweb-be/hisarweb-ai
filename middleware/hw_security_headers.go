package middleware

import (
	"crypto/sha256"
	"encoding/base64"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"

	"github.com/QuantumNous/new-api/common"

	"github.com/gin-gonic/gin"
)

// Browser security headers for every response this server sends.
//
// The header set is fixed rather than configured per deployment: only the
// Content-Security-Policy varies, because only the CSP depends on what the
// served page actually loads. HSTS is emitted only for requests that arrived
// over TLS (or that a trusted proxy marked as https), so a local http run keeps
// working.
const (
	hwHstsValue = "max-age=31536000; includeSubDomains"

	// Permissions-Policy: every powerful feature the dashboard does not use is
	// denied. fullscreen stays available to the application's own origin
	// because the console offers a full-screen log view.
	hwPermissionsPolicy = "accelerometer=(), autoplay=(), camera=(), display-capture=(), " +
		"encrypted-media=(), fullscreen=(self), geolocation=(), gyroscope=(), " +
		"magnetometer=(), microphone=(), midi=(), payment=(), usb=()"

	// hwTurnstileOrigin is a third party the dashboard loads while
	// TurnstileCheckEnabled is on: its api.js script, its challenge frame and
	// its verification requests all live on this origin.
	hwTurnstileOrigin = "https://challenges.cloudflare.com"

	// Telegram login (TelegramOAuthEnabled): telegram.org serves the widget
	// script that draws the button, the login itself runs in a frame on
	// oauth.telegram.org and that frame is also where the console sends its
	// requests.
	hwTelegramScriptOrigin = "https://telegram.org"
	hwTelegramOAuthOrigin  = "https://oauth.telegram.org"

	// Google Analytics 4: gtag.js comes from the tag manager host, the hits go
	// to the analytics host.
	hwGoogleTagManagerOrigin = "https://www.googletagmanager.com"
	hwGoogleAnalyticsOrigin  = "https://www.google-analytics.com"

	// hwUmamiDefaultScriptURL is where the Umami script lives when the operator
	// sets UMAMI_WEBSITE_ID without UMAMI_SCRIPT_URL.
	hwUmamiDefaultScriptURL = "https://analytics.umami.is/script.js"

	// HwCspExtraFrameSrcEnv and HwCspExtraConnectSrcEnv widen frame-src and
	// connect-src with a comma-separated list of https origins, for the few
	// pages that render an administrator-configured URL in a frame (home page,
	// about page, chat presets). Entries that are not a bare https origin are
	// ignored instead of breaking the policy.
	HwCspExtraFrameSrcEnv   = "HW_CSP_EXTRA_FRAME_SRC"
	HwCspExtraConnectSrcEnv = "HW_CSP_EXTRA_CONNECT_SRC"

	// HwCspPolicyEnv replaces the built policy with a full operator-supplied
	// one, for a deployment that adds a host we cannot know here (for example a
	// hosted logo or an analytics endpoint). An empty value means the default.
	HwCspPolicyEnv = "HW_CSP_POLICY"
)

var (
	hwIndexScriptHashesMu sync.RWMutex
	hwIndexScriptHashes   []string
)

// HwSetIndexPageScriptHashes records the hashes of the inline scripts of the
// index page that is actually served. main.go calls it after it has injected
// the optional analytics snippets, so the hashes always describe the real page.
// Without them the theme bootstrap in web/index.html would be blocked, and the
// page would paint in the wrong theme before React takes over.
func HwSetIndexPageScriptHashes(hashes []string) {
	hwIndexScriptHashesMu.Lock()
	defer hwIndexScriptHashesMu.Unlock()
	hwIndexScriptHashes = append([]string(nil), hashes...)
}

// HwIndexPageScriptHashes returns the CSP sha256 source expression for every
// inline script in page. A script with a src attribute is fetched, not hashed;
// whitespace-only bodies have nothing to execute and are skipped.
func HwIndexPageScriptHashes(page []byte) []string {
	matches := hwScriptTagPattern.FindAllSubmatch(page, -1)
	hashes := make([]string, 0, len(matches))
	for _, match := range matches {
		if hwScriptSrcAttributePattern.Match(match[1]) {
			continue
		}
		body := match[2]
		if len(strings.TrimSpace(string(body))) == 0 {
			continue
		}
		sum := sha256.Sum256(body)
		hashes = append(hashes, "'sha256-"+base64.StdEncoding.EncodeToString(sum[:])+"'")
	}
	return hashes
}

var (
	hwScriptTagPattern          = regexp.MustCompile(`(?is)<script\b([^>]*)>(.*?)</script\s*>`)
	hwScriptSrcAttributePattern = regexp.MustCompile(`(?i)(^|\s)src\s*=`)
)

// HwRequestIsHTTPS reports whether the browser reached this server over TLS.
// A reverse proxy terminates TLS and forwards plain http, so the proxy's
// X-Forwarded-Proto counts as well. The header is client-controllable when the
// server is exposed directly, which is harmless here: browsers ignore HSTS on
// an http response, so a forged header can never pin a scheme the operator did
// not want.
func HwRequestIsHTTPS(c *gin.Context) bool {
	if c.Request.TLS != nil {
		return true
	}
	proto, _, _ := strings.Cut(c.GetHeader("X-Forwarded-Proto"), ",")
	return strings.EqualFold(strings.TrimSpace(proto), "https")
}

// HwContentSecurityPolicy builds the policy the dashboard needs. Each exception
// is deliberate:
//
//   - script-src keeps the host list plus one sha256 hash per inline script of
//     the served index page. 'unsafe-inline' is never used, so an injected
//     inline script cannot run; 'unsafe-eval' is never used either, the React
//     build ships plain functions.
//   - style-src allows 'unsafe-inline' because React writes inline style
//     attributes (progress bars, virtualized rows) and the UI library injects
//     style elements at runtime. Styles cannot execute script, so this does not
//     weaken script-src.
//   - img-src allows data: and blob: because the console renders locally built
//     icons and previews. Uploaded or remote images are not allowed: the
//     operator can widen this through HW_CSP_EXTRA_IMG_SRC.
//   - connect-src is the own origin plus the hosts of whichever feature is
//     switched on: Turnstile, Telegram login, Google Analytics and Umami.
//   - frame-src is 'none' unless a feature needs a frame. Our Turkish console
//     configures no home page URL, about page URL or chat preset, so the only
//     frames it can load are the Turnstile challenge, the Telegram login and
//     whatever the operator names in HW_CSP_EXTRA_FRAME_SRC. frame-ancestors
//     'none' keeps the console out of any frame, which is stricter than the
//     X-Frame-Options: DENY that older browsers read.
func HwContentSecurityPolicy() string {
	if override := strings.TrimSpace(os.Getenv(HwCspPolicyEnv)); override != "" {
		return override
	}

	scriptSources := append([]string{"'self'"}, hwIndexScriptSources()...)
	connectSources := []string{"'self'"}
	frameSources := make([]string, 0, 4)

	directives := map[string]string{
		"default-src":     "'self'",
		"style-src":       "'self' 'unsafe-inline'",
		"img-src":         "'self' data: blob:",
		"font-src":        "'self' data:",
		"frame-ancestors": "'none'",
		"base-uri":        "'self'",
		"form-action":     "'self'",
		"object-src":      "'none'",
	}

	if common.TurnstileCheckEnabled {
		frameSources = append(frameSources, hwTurnstileOrigin)
		scriptSources = append(scriptSources, hwTurnstileOrigin)
		connectSources = append(connectSources, hwTurnstileOrigin)
		// Turnstile proves the challenge in a Web Worker it creates from a blob.
		directives["worker-src"] = "'self' blob:"
	}
	if common.TelegramOAuthEnabled {
		frameSources = append(frameSources, hwTelegramOAuthOrigin)
		scriptSources = append(scriptSources, hwTelegramScriptOrigin)
		connectSources = append(connectSources, hwTelegramOAuthOrigin)
	}
	if os.Getenv("GOOGLE_ANALYTICS_ID") != "" {
		scriptSources = append(scriptSources, hwGoogleTagManagerOrigin)
		connectSources = append(connectSources, hwGoogleTagManagerOrigin, hwGoogleAnalyticsOrigin)
	}
	if os.Getenv("UMAMI_WEBSITE_ID") != "" {
		umamiScriptURL := os.Getenv("UMAMI_SCRIPT_URL")
		if umamiScriptURL == "" {
			umamiScriptURL = hwUmamiDefaultScriptURL
		}
		if origin := hwHttpsURLOrigin(umamiScriptURL); origin != "" {
			scriptSources = append(scriptSources, origin)
			connectSources = append(connectSources, origin)
		}
	}
	frameSources = append(frameSources, hwCspExtraOrigins(HwCspExtraFrameSrcEnv)...)
	connectSources = append(connectSources, hwCspExtraOrigins(HwCspExtraConnectSrcEnv)...)

	directives["script-src"] = strings.Join(scriptSources, " ")
	directives["connect-src"] = strings.Join(connectSources, " ")
	if len(frameSources) == 0 {
		directives["frame-src"] = "'none'"
	} else {
		directives["frame-src"] = strings.Join(frameSources, " ")
	}

	order := []string{
		"default-src", "script-src", "style-src", "img-src", "font-src", "connect-src",
		"frame-src", "frame-ancestors", "base-uri", "form-action", "object-src", "worker-src",
	}
	parts := make([]string, 0, len(order))
	for _, name := range order {
		if value, ok := directives[name]; ok {
			parts = append(parts, name+" "+value)
		}
	}
	return strings.Join(parts, "; ")
}

func hwIndexScriptSources() []string {
	hwIndexScriptHashesMu.RLock()
	defer hwIndexScriptHashesMu.RUnlock()
	return append([]string(nil), hwIndexScriptHashes...)
}

// hwCspExtraOrigins reads the comma-separated list of an environment variable
// and keeps the entries that are a bare https origin. An entry that does not
// qualify is ignored: a typo must neither widen the policy nor break the page.
func hwCspExtraOrigins(envName string) []string {
	origins := make([]string, 0, 2)
	for raw := range strings.SplitSeq(os.Getenv(envName), ",") {
		origin, err := common.NormalizeOrigin(raw)
		if err != nil || !strings.HasPrefix(origin, "https://") {
			continue
		}
		origins = append(origins, origin)
	}
	return origins
}

// hwHttpsURLOrigin reduces a script URL to the origin a CSP source needs.
func hwHttpsURLOrigin(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return ""
	}
	return "https://" + parsed.Host
}

// HwSecurityHeaders applies the browser security headers to every response,
// including 404s and relay answers: it is registered as the first middleware of
// the server, before any route decides its own headers.
func HwSecurityHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.Writer.Header()
		if HwRequestIsHTTPS(c) {
			header.Set("Strict-Transport-Security", hwHstsValue)
		}
		header.Set("X-Content-Type-Options", "nosniff")
		header.Set("X-Frame-Options", "DENY")
		header.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		header.Set("Permissions-Policy", hwPermissionsPolicy)
		header.Set("Content-Security-Policy", HwContentSecurityPolicy())
		c.Next()
	}
}

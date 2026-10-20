package middleware

import (
	"crypto/sha256"
	"encoding/base64"
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

	// hwTurnstileOrigin is the only third party the dashboard loads, and only
	// while TurnstileCheckEnabled is on: its api.js script, its challenge frame
	// and its verification requests all live on this origin.
	hwTurnstileOrigin = "https://challenges.cloudflare.com"

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
//     operator can widen this through HW_CSP_POLICY.
//   - connect-src is the own origin only; the dashboard talks to nothing else
//     except Turnstile while it is enabled.
//   - frame-src is 'none' unless Turnstile is enabled; frame-ancestors 'none'
//     keeps the console out of any frame, which is stricter than the
//     X-Frame-Options: DENY that older browsers read.
func HwContentSecurityPolicy() string {
	if override := strings.TrimSpace(os.Getenv(HwCspPolicyEnv)); override != "" {
		return override
	}

	turnstile := common.TurnstileCheckEnabled

	directives := map[string]string{
		"default-src":     "'self'",
		"script-src":      "'self'" + hwIndexScriptSources() + hwOptionalOrigin(turnstile),
		"style-src":       "'self' 'unsafe-inline'",
		"img-src":         "'self' data: blob:",
		"font-src":        "'self' data:",
		"connect-src":     "'self'" + hwOptionalOrigin(turnstile),
		"frame-src":       "'none'",
		"frame-ancestors": "'none'",
		"base-uri":        "'self'",
		"form-action":     "'self'",
		"object-src":      "'none'",
	}
	if turnstile {
		directives["frame-src"] = hwTurnstileOrigin
		// Turnstile proves the challenge in a Web Worker it creates from a blob.
		directives["worker-src"] = "'self' blob:"
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

func hwIndexScriptSources() string {
	hwIndexScriptHashesMu.RLock()
	defer hwIndexScriptHashesMu.RUnlock()
	if len(hwIndexScriptHashes) == 0 {
		return ""
	}
	return " " + strings.Join(hwIndexScriptHashes, " ")
}

func hwOptionalOrigin(enabled bool) string {
	if !enabled {
		return ""
	}
	return " " + hwTurnstileOrigin
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

package middleware

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newHwSecurityHeadersEngine(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(HwSecurityHeaders())
	engine.GET("/api/example", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"success": true})
	})
	return engine
}

func hwServeAndRecord(engine *gin.Engine, request *http.Request) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)
	return recorder
}

// hwCspDirective returns the sources of one directive of a policy string.
func hwCspDirective(policy, name string) string {
	for part := range strings.SplitSeq(policy, ";") {
		part = strings.TrimSpace(part)
		if rest, ok := strings.CutPrefix(part, name+" "); ok {
			return rest
		}
	}
	return ""
}

// TestHwSecurityHeadersAreOnJSONAndStaticAnswers pins the header set on the two
// shapes a browser actually receives: a dashboard API answer and a served page.
func TestHwSecurityHeadersAreOnJSONAndStaticAnswers(t *testing.T) {
	previousTurnstile := common.TurnstileCheckEnabled
	common.TurnstileCheckEnabled = false
	t.Cleanup(func() {
		common.TurnstileCheckEnabled = previousTurnstile
		HwSetIndexPageScriptHashes(nil)
	})
	HwSetIndexPageScriptHashes(nil)

	engine := newHwSecurityHeadersEngine(t)
	answers := map[string]*httptest.ResponseRecorder{
		"json": hwServeAndRecord(engine, httptest.NewRequest(http.MethodGet, "/api/example", nil)),
	}
	staticDir := t.TempDir()
	page := []byte("<!doctype html><html><body>dashboard</body></html>")
	require.NoError(t, os.WriteFile(filepath.Join(staticDir, "dashboard.html"), page, 0o600))
	fileServer := http.StripPrefix("/static", http.FileServer(http.Dir(staticDir)))
	engine.GET("/static/dashboard.html", gin.WrapH(fileServer))
	answers["static page"] = hwServeAndRecord(engine, httptest.NewRequest(http.MethodGet, "/static/dashboard.html", nil))

	for name, recorder := range answers {
		header := recorder.Header()
		assert.Equal(t, "nosniff", header.Get("X-Content-Type-Options"), name)
		assert.Equal(t, "DENY", header.Get("X-Frame-Options"), name)
		assert.Equal(t, "strict-origin-when-cross-origin", header.Get("Referrer-Policy"), name)
		assert.Equal(t, hwPermissionsPolicy, header.Get("Permissions-Policy"), name)
		assert.NotEmpty(t, header.Get("Content-Security-Policy"), name)
		// Plain http: HSTS would pin https for a local run and is therefore absent.
		assert.Empty(t, header.Get("Strict-Transport-Security"), name)
	}
	assert.Contains(t, answers["static page"].Header().Get("Content-Type"), "text/html")
	assert.Contains(t, answers["json"].Header().Get("Content-Type"), "application/json")
}

// TestHwHstsOnlyOnHTTPS proves the HSTS header follows the request scheme and
// not the middleware's own opinion: a local http run must keep working.
func TestHwHstsOnlyOnHTTPS(t *testing.T) {
	engine := newHwSecurityHeadersEngine(t)
	for _, test := range []struct {
		name       string
		tls        bool
		forwarded  string
		wantHeader bool
	}{
		{name: "plain http", wantHeader: false},
		{name: "tls", tls: true, wantHeader: true},
		{name: "proxy says https", forwarded: "https", wantHeader: true},
		{name: "proxy says https among several", forwarded: "https, http", wantHeader: true},
		{name: "proxy says http", forwarded: "http", wantHeader: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/api/example", nil)
			if test.tls {
				request.TLS = &tls.ConnectionState{}
			}
			if test.forwarded != "" {
				request.Header.Set("X-Forwarded-Proto", test.forwarded)
			}
			value := hwServeAndRecord(engine, request).Header().Get("Strict-Transport-Security")
			if !test.wantHeader {
				assert.Empty(t, value)
				return
			}
			assert.Equal(t, "max-age=31536000; includeSubDomains", value)
		})
	}
}

// TestHwContentSecurityPolicyFitsTheDashboard pins the policy the React console
// needs and the one thing it must never contain: a script escape hatch.
func TestHwContentSecurityPolicyFitsTheDashboard(t *testing.T) {
	previousTurnstile := common.TurnstileCheckEnabled
	t.Cleanup(func() {
		common.TurnstileCheckEnabled = previousTurnstile
		HwSetIndexPageScriptHashes(nil)
		t.Setenv(HwCspPolicyEnv, "")
	})

	page := []byte(`<html><head><script>var theme = 1</script>` +
		`<script src="/assets/index.js"></script></head><body></body></html>`)
	hashes := HwIndexPageScriptHashes(page)
	require.Len(t, hashes, 1, "only the inline script is hashed, the fetched one is not")
	HwSetIndexPageScriptHashes(hashes)

	common.TurnstileCheckEnabled = false
	withoutTurnstile := HwContentSecurityPolicy()
	assert.NotContains(t, withoutTurnstile, "unsafe-eval")
	assert.NotContains(t, hwCspDirective(withoutTurnstile, "script-src"), "unsafe-inline", "script-src carries no unsafe-inline")
	assert.Contains(t, withoutTurnstile, hashes[0], "the inline theme script must be allowed by its hash")
	assert.Contains(t, withoutTurnstile, "script-src 'self'")
	assert.Contains(t, withoutTurnstile, "frame-ancestors 'none'")
	assert.Contains(t, withoutTurnstile, "object-src 'none'")
	assert.Contains(t, withoutTurnstile, "connect-src 'self'")
	assert.NotContains(t, withoutTurnstile, "challenges.cloudflare.com", "no third party while Turnstile is off")

	common.TurnstileCheckEnabled = true
	withTurnstile := HwContentSecurityPolicy()
	assert.NotContains(t, withTurnstile, "unsafe-eval")
	assert.Contains(t, withTurnstile, "script-src 'self' "+hashes[0]+" https://challenges.cloudflare.com")
	assert.Contains(t, withTurnstile, "frame-src https://challenges.cloudflare.com")
	assert.Contains(t, withTurnstile, "connect-src 'self' https://challenges.cloudflare.com")

	t.Setenv(HwCspPolicyEnv, "default-src 'none'")
	assert.Equal(t, "default-src 'none'", HwContentSecurityPolicy(), "the operator override wins")
}

// TestHwSecurityHeadersSurviveTheMaskedRelayPath proves that the supplier
// masking of customer routes does not drop the security headers: they belong to
// the answer, not to the supplier.
func TestHwSecurityHeadersSurviveTheMaskedRelayPath(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(HwSecurityHeaders())
	engine.GET("/api/user/self", HwMask(), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"success": true})
	})
	recorder := hwServeAndRecord(engine, httptest.NewRequest(http.MethodGet, "/api/user/self", nil))
	assert.Equal(t, "nosniff", recorder.Header().Get("X-Content-Type-Options"))
	assert.Equal(t, "DENY", recorder.Header().Get("X-Frame-Options"))
	assert.NotEmpty(t, recorder.Header().Get("Content-Security-Policy"))
}

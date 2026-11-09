package router

// The crawler files of the host, proven over HTTP on the router the server
// really builds: a fresh crawler asks /robots.txt on the API host and must get
// the rule that keeps it out of the API paths, not the dashboard index page that
// used to answer there.

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSeoRobotsTxt(t *testing.T) {
	gin.SetMode(gin.TestMode)

	engine := gin.New()
	SetSeoRouter(engine)
	// The web catch-all runs last on the real server; here it answers a marker,
	// so a robots.txt that fell through is visible as that marker instead of a
	// second text/html index page.
	engine.NoRoute(func(c *gin.Context) {
		c.Data(http.StatusOK, "text/html; charset=utf-8", []byte("<html></html>"))
	})

	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/robots.txt", nil))

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Header().Get("Content-Type"), "text/plain")
	assert.Contains(t, recorder.Body.String(), "Disallow: /")
	assert.Contains(t, recorder.Body.String(), "User-agent: *")
}

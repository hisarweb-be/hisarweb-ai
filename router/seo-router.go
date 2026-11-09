package router

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// hwRobotsTxt is the answer for every crawler on this host: the API host serves
// no indexable pages, so nothing here belongs in a search index. It used to fall
// through to the dashboard index page, which told a crawler nothing and left it
// probing API paths that answer 401 without a token.
const hwRobotsTxt = `# This is an API host; its paths are not meant to be indexed.
User-agent: *
Disallow: /
`

// SetSeoRouter registers the crawler files that belong to the host itself,
// before the web catch-all, so the dashboard index page does not answer them.
func SetSeoRouter(router *gin.Engine) {
	router.GET("/robots.txt", func(c *gin.Context) {
		c.Header("Cache-Control", "public, max-age=86400")
		c.Data(http.StatusOK, "text/plain; charset=utf-8", []byte(hwRobotsTxt))
	})
}

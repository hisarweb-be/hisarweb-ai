package controller

import (
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"

	"github.com/gin-gonic/gin"
)

// The dashboard's update check compares the running version with the upstream
// releases. The browser cannot ask GitHub itself: the Content-Security-Policy
// keeps connect-src at 'self'. So the server asks, at most once per cache
// window, and hands the release list through unchanged.
var hwUpstreamReleasesURL = "https://api.github.com/repos/QuantumNous/new-api/releases?per_page=100"

const (
	hwUpstreamReleasesTTL      = time.Hour
	hwUpstreamReleasesErrorTTL = 5 * time.Minute
	hwUpstreamReleasesMaxBody  = 4 << 20
)

var hwUpstreamReleases struct {
	sync.Mutex
	body      []byte
	status    int
	expiresAt time.Time
}

var hwUpstreamReleasesClient = &http.Client{Timeout: 10 * time.Second}

// HwUpstreamReleases serves the cached upstream release list to administrators.
// A GitHub rate limit becomes 429 and any other failure 502, so the dashboard
// can tell the two apart; failures are cached briefly so no tab can hammer
// GitHub while it is down.
func HwUpstreamReleases(c *gin.Context) {
	if c.GetInt("role") < common.RoleAdminUser {
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"message": "Only administrators can check for updates",
		})
		return
	}
	cache := &hwUpstreamReleases
	cache.Lock()
	defer cache.Unlock()
	if time.Now().After(cache.expiresAt) {
		cache.status, cache.body = hwFetchUpstreamReleases()
		ttl := hwUpstreamReleasesTTL
		if cache.status != http.StatusOK {
			ttl = hwUpstreamReleasesErrorTTL
		}
		cache.expiresAt = time.Now().Add(ttl)
	}
	if cache.status != http.StatusOK {
		c.AbortWithStatus(cache.status)
		return
	}
	c.Data(http.StatusOK, "application/json", cache.body)
}

func hwFetchUpstreamReleases() (int, []byte) {
	request, err := http.NewRequest(http.MethodGet, hwUpstreamReleasesURL, nil)
	if err != nil {
		return http.StatusBadGateway, nil
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	response, err := hwUpstreamReleasesClient.Do(request)
	if err != nil {
		common.SysLog("hw: upstream release check failed: " + err.Error())
		return http.StatusBadGateway, nil
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusForbidden || response.StatusCode == http.StatusTooManyRequests {
		return http.StatusTooManyRequests, nil
	}
	if response.StatusCode != http.StatusOK {
		return http.StatusBadGateway, nil
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, hwUpstreamReleasesMaxBody))
	if err != nil {
		return http.StatusBadGateway, nil
	}
	return http.StatusOK, body
}

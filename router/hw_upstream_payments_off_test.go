package router

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// hwUpstreamPaymentRoutes are the upstream payment entry points: provider
// webhooks, payment callbacks and the pay and amount calls. Our own Revolut and
// Stripe flow lives under /api/hw and is not on this list.
var hwUpstreamPaymentRoutes = []string{
	"POST /api/stripe/webhook",
	"POST /api/creem/webhook",
	"POST /api/waffo/webhook",
	"POST /api/waffo-pancake/webhook/:env",
	"POST /api/user/epay/notify",
	"GET /api/user/epay/notify",
	"POST /api/user/pay",
	"POST /api/user/amount",
	"POST /api/user/stripe/pay",
	"POST /api/user/stripe/amount",
	"POST /api/user/creem/pay",
	"POST /api/user/waffo/pay",
	"POST /api/user/waffo/amount",
	"POST /api/user/waffo-pancake/pay",
	"POST /api/user/waffo-pancake/amount",
	"POST /api/subscription/epay/pay",
	"POST /api/subscription/stripe/pay",
	"POST /api/subscription/creem/pay",
	"POST /api/subscription/waffo-pancake/pay",
	"POST /api/subscription/epay/notify",
	"GET /api/subscription/epay/notify",
	"GET /api/subscription/epay/return",
	"POST /api/subscription/epay/return",
}

func registeredAPIRoutes(t *testing.T) map[string]bool {
	t.Helper()
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	require.NotPanics(t, func() { SetApiRouter(engine) })
	registered := map[string]bool{}
	for _, route := range engine.Routes() {
		registered[route.Method+" "+route.Path] = true
	}
	return registered
}

// G14 (ASVS V11.1.4): with the default configuration none of the unused
// upstream payment routes exists, so they cannot be probed or abused.
func TestHwUpstreamPaymentRoutesAreAbsentByDefault(t *testing.T) {
	t.Setenv(common.HwUpstreamPaymentsEnv, "")
	registered := registeredAPIRoutes(t)
	for _, route := range hwUpstreamPaymentRoutes {
		assert.False(t, registered[route], "%s must not be registered by default", route)
	}
}

// With the switch on every listed route exists, so the list is not stale.
func TestHwUpstreamPaymentRouteListIsAccurate(t *testing.T) {
	t.Setenv(common.HwUpstreamPaymentsEnv, "true")
	registered := registeredAPIRoutes(t)
	for _, route := range hwUpstreamPaymentRoutes {
		assert.True(t, registered[route], "%s is listed but not registered when payments are on", route)
	}
}

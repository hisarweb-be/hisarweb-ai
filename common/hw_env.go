package common

import (
	"os"
	"strings"
)

// APP_ENV is the single switch that separates a real deployment from a local
// run. A production process refuses every convenience that is safe on a laptop
// but dangerous once strangers can reach the server: the default administrator
// password, insecure session cookies and the upstream payment routes we do not
// use.
const (
	// HwAppEnvProduction is the APP_ENV value that marks a real deployment.
	HwAppEnvProduction = "production"

	// HwUpstreamPaymentsEnv re-registers the payment routes of the upstream
	// project (Stripe, Creem, Waffo, Waffo Pancake, Epay and their webhooks).
	// They stay unregistered unless this is explicitly true: HisarWeb AI sells
	// through its own Revolut order flow, and every extra reachable webhook is
	// an attack surface nobody here monitors.
	HwUpstreamPaymentsEnv = "HW_UPSTREAM_PAYMENTS_ENABLED"
)

// HwIsProduction reports whether this process runs as a real deployment.
func HwIsProduction() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv("APP_ENV")), HwAppEnvProduction)
}

// HwUpstreamPaymentsEnabled reports whether the upstream payment routes are
// switched on. Default off, also in development: a developer who needs them
// sets the variable explicitly.
func HwUpstreamPaymentsEnabled() bool {
	return GetEnvOrDefaultBool(HwUpstreamPaymentsEnv, false)
}

// HwIsDefaultAdminCredential recognises the administrator username/password
// pair that old versions of this gateway created on an empty database. The pair
// is public knowledge (it is in the source and in every deployment guide), so a
// production server must never accept it, even when an upgraded database still
// carries such a row.
func HwIsDefaultAdminCredential(username, password string) bool {
	return strings.EqualFold(strings.TrimSpace(username), "root") && password == "123456"
}

package common

import (
	"errors"
	"os"
	"slices"
	"strings"
)

// hwSkipPhoneStepEnv and hwAppEnvEnv name the environment variables that gate
// the Telegram phone step of the free trial credit. Their values are never
// logged: the flag name is enough to explain a refusal.
const (
	hwSkipPhoneStepEnv = "HW_SKIP_PHONE_STEP"
	hwAppEnvEnv        = "APP_ENV"
	hwSignalSaltEnv    = "HW_SIGNAL_SALT"
)

// hwSignalSaltMinBytes mirrors model.hwSignalSaltMinBytes. A shorter salt would
// weaken every keyed abuse hash, so production refuses to start with one.
const hwSignalSaltMinBytes = 32

// hwSkipPhoneStepAppEnvs lists the only APP_ENV values in which
// HW_SKIP_PHONE_STEP may be switched on. Every other value, including an empty
// or misspelled one, refuses the flag, so a deployment that forgets APP_ENV
// keeps the phone step instead of silently losing it.
var hwSkipPhoneStepAppEnvs = []string{"development", "dev", "test", "local"}

// hwProductionAppEnvs lists the APP_ENV values that mean "real customers".
var hwProductionAppEnvs = []string{"production", "prod"}

var (
	// ErrHwPhoneStepSkipOutsideDevelopment is returned by HwValidateEnvironment
	// when HW_SKIP_PHONE_STEP is switched on while APP_ENV names no development
	// environment. The message names only the variables, never their values.
	ErrHwPhoneStepSkipOutsideDevelopment = errors.New("HW_SKIP_PHONE_STEP is only allowed when APP_ENV is development, dev, test or local: the phone step guards the free trial credit")

	// ErrHwSignalSaltRequired is returned when APP_ENV names no development
	// environment and HW_SIGNAL_SALT is missing or shorter than 32 bytes. There
	// is deliberately no fallback secret, and the message names no value.
	ErrHwSignalSaltRequired = errors.New("HW_SIGNAL_SALT must be set to at least 32 bytes unless APP_ENV names a development environment (development, dev, test, local)")
)

// hwAppEnv returns APP_ENV with surrounding whitespace removed and lowercased,
// so " Production " and "development" compare exactly after normalization.
func hwAppEnv() string {
	return strings.ToLower(strings.TrimSpace(os.Getenv(hwAppEnvEnv)))
}

// hwPhoneStepFlagSet reports whether HW_SKIP_PHONE_STEP asks for the skip. An
// empty value, "0" and "false" mean off; any other non-empty value means on. An
// unparsable value counts as on so the environment check, not the spelling,
// decides whether the skip may apply.
func hwPhoneStepFlagSet() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(hwSkipPhoneStepEnv))) {
	case "", "0", "false":
		return false
	}
	return true
}

// HwPhoneStepSkipped reports whether the phone step of the free trial credit is
// skipped in this process. The skip is a local development aid only: it applies
// only when APP_ENV exactly names a development environment and the flag is on.
func HwPhoneStepSkipped() bool {
	return hwPhoneStepFlagSet() && slices.Contains(hwSkipPhoneStepAppEnvs, hwAppEnv())
}

// HwAppEnvIsProduction reports whether APP_ENV names the production
// environment. The comparison is case-insensitive and ignores surrounding
// whitespace, so " Production " still counts as production.
func HwAppEnvIsProduction() bool {
	return slices.Contains(hwProductionAppEnvs, hwAppEnv())
}

// HwValidateEnvironment refuses an unsafe combination of test flags before the
// server starts doing any work. It must run after .env is loaded, because the
// values it reads may come from that file; checking earlier would test a
// different environment than the process runs in. On error the caller should
// exit without printing any environment value.
func HwValidateEnvironment() error {
	if hwPhoneStepFlagSet() && !slices.Contains(hwSkipPhoneStepAppEnvs, hwAppEnv()) {
		return ErrHwPhoneStepSkipOutsideDevelopment
	}
	if hwSignalSaltInsufficient() {
		return ErrHwSignalSaltRequired
	}
	return nil
}

// APP_ENV is the single switch that separates a real deployment from a local
// run. A production process refuses every convenience that is safe on a laptop
// but dangerous once strangers can reach the server: the default administrator
// password, insecure session cookies and the upstream payment routes we do not
// use.
const (
	// HwAppEnvProduction is the APP_ENV value that marks a real deployment.
	HwAppEnvProduction = "production"

	// HwAppEnvDevelopment and HwAppEnvTest are the only values that turn the
	// production guards off. Every other value, including an unset variable,
	// counts as production.
	HwAppEnvDevelopment = "development"
	HwAppEnvTest        = "test"

	// HwUpstreamPaymentsEnv re-registers the payment routes of the upstream
	// project (Stripe, Creem, Waffo, Waffo Pancake, Epay and their webhooks).
	// They stay unregistered unless this is explicitly true: HisarWeb AI sells
	// through its own Revolut order flow, and every extra reachable webhook is
	// an attack surface nobody here monitors.
	HwUpstreamPaymentsEnv = "HW_UPSTREAM_PAYMENTS_ENABLED"
)

// HwIsProduction reports whether this process runs as a real deployment. The
// guard fails closed: a server only counts as non-production when it says so
// explicitly, through development or test. An unset or misspelled APP_ENV
// therefore keeps every production guard on instead of silently dropping them.
func HwIsProduction() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("APP_ENV"))) {
	case HwAppEnvDevelopment, HwAppEnvTest:
		return false
	default:
		return true
	}
}

// HwUpstreamPaymentsEnabled reports whether the upstream payment routes are
// switched on. Default off, also in development: a developer who needs them
// sets the variable explicitly.
func HwUpstreamPaymentsEnabled() bool {
	return GetEnvOrDefaultBool(HwUpstreamPaymentsEnv, false)
}

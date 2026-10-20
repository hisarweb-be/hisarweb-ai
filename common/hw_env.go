package common

import (
	"errors"
	"os"
	"strings"
)

// hwSkipPhoneStepEnv and hwAppEnvEnv name the environment variables that gate
// the Telegram phone step of the free trial credit. Their values are never
// logged: the flag name is enough to explain a refusal.
const (
	hwSkipPhoneStepEnv = "HW_SKIP_PHONE_STEP"
	hwAppEnvEnv        = "APP_ENV"
)

// hwAppEnvProduction is the only APP_ENV value that means "real customers".
const hwAppEnvProduction = "production"

// ErrHwPhoneStepSkipInProduction is returned by HwValidateEnvironment when the
// test flag HW_SKIP_PHONE_STEP is set while APP_ENV is production. The message
// deliberately names only the variables, never their values.
var ErrHwPhoneStepSkipInProduction = errors.New("HW_SKIP_PHONE_STEP must not be set when APP_ENV is production: the phone step guards the free trial credit")

// HwPhoneStepSkipped reports whether the phone step of the free trial credit is
// skipped in this process. The skip is a local development aid only: it never
// applies when APP_ENV is production, regardless of the flag.
func HwPhoneStepSkipped() bool {
	if strings.TrimSpace(os.Getenv(hwSkipPhoneStepEnv)) == "" {
		return false
	}
	return !HwAppEnvIsProduction()
}

// HwAppEnvIsProduction reports whether APP_ENV names the production
// environment. The comparison is case-insensitive and ignores surrounding
// whitespace, so " Production " still counts as production.
func HwAppEnvIsProduction() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv(hwAppEnvEnv)), hwAppEnvProduction)
}

// HwValidateEnvironment refuses an unsafe combination of test flags before the
// server starts doing any work. A process that would skip the phone step in
// production must stop with a non-zero exit code, not silently weaken the
// abuse protection around the free trial credit. Call it early in main; on
// error the caller should exit without printing any environment value.
func HwValidateEnvironment() error {
	flagSet := strings.TrimSpace(os.Getenv(hwSkipPhoneStepEnv)) != ""
	if flagSet && HwAppEnvIsProduction() {
		return ErrHwPhoneStepSkipInProduction
	}
	return nil
}

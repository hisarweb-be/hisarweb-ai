package setting

import (
	"os"
	"strconv"
	"strings"
)

// The free trial group gets a fixed low request budget: a free account may try
// a few requests per minute and a bounded number per rolling day, because the
// trial credit is meant to let a customer try the product, not to serve as an
// unpaid relay. The defaults are deliberately low and are documented here; an
// operator raises or lowers them with the environment variables below, and a
// per-group entry in ModelRequestRateLimitGroup for the free group is honoured
// as the per-minute figure when the environment does not set one.
const (
	// HwFreeGroupName is the group every account holding the free trial credit
	// is moved to.
	HwFreeGroupName = "free"

	hwFreeGroupPerMinuteEnv = "HW_FREE_GROUP_PER_MINUTE"
	hwFreeGroupPerDayEnv    = "HW_FREE_GROUP_PER_DAY"

	// HwFreeGroupPerMinuteDefault is the safe default of requests per minute.
	HwFreeGroupPerMinuteDefault = 10
	// HwFreeGroupPerDayDefault is the safe default of requests per rolling day.
	HwFreeGroupPerDayDefault = 200
)

// hwPositiveEnvInt reads a strictly positive integer from the environment.
// A missing, malformed or non-positive value yields 0, so a typo falls back to
// the safe default instead of switching the limit off.
func hwPositiveEnvInt(name string) int {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return 0
	}
	parsed, err := strconv.Atoi(raw)
	if err != nil || parsed <= 0 {
		return 0
	}
	return parsed
}

// HwFreeGroupPerMinuteLimit is the per-minute request budget of a free account.
// Precedence: the environment, then an operator-configured per-group limit for
// the free group, then HwFreeGroupPerMinuteDefault.
func HwFreeGroupPerMinuteLimit() int {
	if limit := hwPositiveEnvInt(hwFreeGroupPerMinuteEnv); limit > 0 {
		return limit
	}
	if total, _, found := GetGroupRateLimit(HwFreeGroupName); found && total > 0 {
		return total
	}
	return HwFreeGroupPerMinuteDefault
}

// HwFreeGroupPerDayLimit is the request budget of a free account over one
// rolling day. Only the environment can raise it above the documented default:
// the per-group rate-limit setting has a minute-sized window and cannot express
// a day.
func HwFreeGroupPerDayLimit() int {
	if limit := hwPositiveEnvInt(hwFreeGroupPerDayEnv); limit > 0 {
		return limit
	}
	return HwFreeGroupPerDayDefault
}

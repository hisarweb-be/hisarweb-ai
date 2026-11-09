package common

// hwSessionIdleEnv is the idle limit in seconds. 0 (the default) keeps the
// current behaviour: only the absolute session lifetime applies. A session that
// has not refreshed within this many seconds is refused at its next refresh and
// the user signs in again (ASVS V3.3.2).
const hwSessionIdleEnv = "HW_SESSION_IDLE_SECONDS"

func HwSessionIdleExpired(lastActiveAt, now int64) bool {
	limit := int64(GetEnvOrDefault(hwSessionIdleEnv, 0))
	return limit > 0 && lastActiveAt > 0 && now-lastActiveAt > limit
}

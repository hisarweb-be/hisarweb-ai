package common

import (
	"strings"
	"sync"
	"time"
)

// Per-account throttle for password login (ASVS V2.2.1). It is keyed on the
// submitted identifier whether or not the account exists, so the answer never
// reveals which accounts are real. It is a growing delay and not a hard
// lockout, so an attacker cannot lock the owner out for long.
const (
	loginThrottleFreeFailures = 5
	loginThrottleBaseDelay    = 2 * time.Second
	loginThrottleMaxDelay     = 15 * time.Minute
	loginThrottleResetAfter   = time.Hour
	loginThrottleMaxEntries   = 100000
)

type loginThrottleEntry struct {
	failures  int
	blockedTo time.Time
	last      time.Time
}

var (
	loginThrottleMu      sync.Mutex
	loginThrottleEntries = map[string]*loginThrottleEntry{}
)

func loginThrottleKey(identifier string) string {
	k := strings.ToLower(strings.TrimSpace(identifier))
	if len(k) > 256 {
		k = k[:256]
	}
	return k
}

// LoginThrottleCheck reports how long the caller must wait before another
// password attempt for this identifier. Zero means allowed.
func LoginThrottleCheck(identifier string, now time.Time) time.Duration {
	loginThrottleMu.Lock()
	defer loginThrottleMu.Unlock()
	e := loginThrottleEntries[loginThrottleKey(identifier)]
	if e == nil {
		return 0
	}
	if now.Sub(e.last) > loginThrottleResetAfter {
		delete(loginThrottleEntries, loginThrottleKey(identifier))
		return 0
	}
	if now.Before(e.blockedTo) {
		return e.blockedTo.Sub(now)
	}
	return 0
}

// LoginThrottleFailure records a failed password attempt.
func LoginThrottleFailure(identifier string, now time.Time) {
	loginThrottleMu.Lock()
	defer loginThrottleMu.Unlock()
	if len(loginThrottleEntries) >= loginThrottleMaxEntries {
		for k, v := range loginThrottleEntries {
			if now.Sub(v.last) > loginThrottleResetAfter {
				delete(loginThrottleEntries, k)
			}
		}
		if len(loginThrottleEntries) >= loginThrottleMaxEntries {
			return // bounded memory: fail open rather than grow without limit
		}
	}
	key := loginThrottleKey(identifier)
	e := loginThrottleEntries[key]
	if e == nil || now.Sub(e.last) > loginThrottleResetAfter {
		e = &loginThrottleEntry{}
		loginThrottleEntries[key] = e
	}
	e.failures++
	e.last = now
	if e.failures > loginThrottleFreeFailures {
		delay := loginThrottleBaseDelay
		for i := loginThrottleFreeFailures + 1; i < e.failures && delay < loginThrottleMaxDelay; i++ {
			delay *= 2
		}
		if delay > loginThrottleMaxDelay {
			delay = loginThrottleMaxDelay
		}
		e.blockedTo = now.Add(delay)
	}
}

// LoginThrottleSuccess clears the counter after a successful login.
func LoginThrottleSuccess(identifier string) {
	loginThrottleMu.Lock()
	defer loginThrottleMu.Unlock()
	delete(loginThrottleEntries, loginThrottleKey(identifier))
}

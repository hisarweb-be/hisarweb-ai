package setting

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestHwFreeGroupLimitsPrecedence proves where the free trial budget comes
// from: a usable environment value wins, a typo or a non-positive number falls
// back to the safe default instead of switching the limit off, and only the
// per-minute figure may come from the per-group rate limit setting.
func TestHwFreeGroupLimitsPrecedence(t *testing.T) {
	ModelRequestRateLimitMutex.Lock()
	previous := ModelRequestRateLimitGroup
	ModelRequestRateLimitMutex.Unlock()
	t.Cleanup(func() {
		ModelRequestRateLimitMutex.Lock()
		ModelRequestRateLimitGroup = previous
		ModelRequestRateLimitMutex.Unlock()
	})
	setGroup := func(groups map[string][2]int) {
		ModelRequestRateLimitMutex.Lock()
		ModelRequestRateLimitGroup = groups
		ModelRequestRateLimitMutex.Unlock()
	}

	t.Run("nothing configured gives the documented defaults", func(t *testing.T) {
		t.Setenv(hwFreeGroupPerMinuteEnv, "")
		t.Setenv(hwFreeGroupPerDayEnv, "")
		setGroup(nil)
		assert.Equal(t, HwFreeGroupPerMinuteDefault, HwFreeGroupPerMinuteLimit())
		assert.Equal(t, HwFreeGroupPerDayDefault, HwFreeGroupPerDayLimit())
	})
	t.Run("the environment wins over the group setting", func(t *testing.T) {
		t.Setenv(hwFreeGroupPerMinuteEnv, " 25 ")
		t.Setenv(hwFreeGroupPerDayEnv, "900")
		setGroup(map[string][2]int{HwFreeGroupName: {40, 0}})
		assert.Equal(t, 25, HwFreeGroupPerMinuteLimit())
		assert.Equal(t, 900, HwFreeGroupPerDayLimit())
	})
	t.Run("a free-group entry sets the minute figure and never the day figure", func(t *testing.T) {
		t.Setenv(hwFreeGroupPerMinuteEnv, "")
		t.Setenv(hwFreeGroupPerDayEnv, "")
		setGroup(map[string][2]int{HwFreeGroupName: {40, 0}})
		assert.Equal(t, 40, HwFreeGroupPerMinuteLimit())
		assert.Equal(t, HwFreeGroupPerDayDefault, HwFreeGroupPerDayLimit())
	})
	t.Run("a zero group entry or a typo cannot switch the limit off", func(t *testing.T) {
		for _, raw := range []string{"abc", "0", "-5", "1.5"} {
			t.Setenv(hwFreeGroupPerMinuteEnv, raw)
			t.Setenv(hwFreeGroupPerDayEnv, raw)
			setGroup(map[string][2]int{HwFreeGroupName: {0, 0}})
			assert.Equal(t, HwFreeGroupPerMinuteDefault, HwFreeGroupPerMinuteLimit(), raw)
			assert.Equal(t, HwFreeGroupPerDayDefault, HwFreeGroupPerDayLimit(), raw)
		}
	})
}

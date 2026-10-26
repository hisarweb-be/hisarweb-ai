package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The reserve rule is tested where it is decided: the window, the limit
// formula, the bureau measurement, the customer ceiling and the customer-facing
// texts. The HTTP behaviour of the gate is covered in router/hw_reserve_test.go.

var hwReserveTestColumnsOnce sync.Once

func hwReserveTestSetup(t *testing.T) {
	t.Helper()
	hwReserveTestColumnsOnce.Do(func() {
		require.NoError(t, model.InitLogDB())
	})
	require.NoError(t, model.LOG_DB.AutoMigrate(&model.Log{}))
	require.NoError(t, model.LOG_DB.Exec("DELETE FROM logs").Error)
	hwReserveTestSetOption(t, model.HwWindowCapacityUSDOption, "")
	hwReserveTestSetOption(t, model.HwPhase0MeasurementDoneOption, "false")
	hwReserveTestSetOption(t, model.HwCustomerWindowCeilingUSDOption, "")
	t.Setenv(model.HwWindowCapacityUSDEnv, "")
	t.Setenv(model.HwBureauUsageURLEnv, "")
	t.Setenv(model.HwBureauUsageTokenEnv, "")
	t.Setenv(model.HwReserveModeEnv, "")
	HwSetBureauUsageSource(nil)
	t.Cleanup(func() { HwSetBureauUsageSource(nil) })
}

func hwReserveTestSetOption(t *testing.T, key string, value string) {
	t.Helper()
	common.OptionMapRWMutex.Lock()
	if common.OptionMap == nil {
		common.OptionMap = make(map[string]string)
	}
	previous, had := common.OptionMap[key]
	common.OptionMap[key] = value
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		defer common.OptionMapRWMutex.Unlock()
		if had {
			common.OptionMap[key] = previous
		} else {
			delete(common.OptionMap, key)
		}
	})
}

func hwReserveTestInsertLog(t *testing.T, userID int, group string, quota int, createdAt int64, logType int) {
	t.Helper()
	require.NoError(t, model.LOG_DB.Create(&model.Log{
		UserId:    userID,
		CreatedAt: createdAt,
		Type:      logType,
		Quota:     quota,
		Group:     group,
	}).Error)
}

type hwReserveFakeSource struct {
	used  float64
	err   error
	calls int
}

func (source *hwReserveFakeSource) UsedUSD(ctx context.Context, windowStart int64, windowEnd int64) (float64, error) {
	source.calls++
	return source.used, source.err
}

// TestHwReserveWindowBoundaries pins the five-hour UTC windows: a time one
// second before a boundary still belongs to the previous window, the boundary
// itself opens the next one, and the end is exactly five hours later.
func TestHwReserveWindowBoundaries(t *testing.T) {
	day := time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name        string
		now         time.Time
		windowStart time.Time
	}{
		{name: "23:59:59 belongs to the 20:00 window", now: day.Add(-time.Second), windowStart: day.Add(-4 * time.Hour)},
		{name: "00:00:00 opens the midnight window", now: day, windowStart: day},
		{name: "04:59:59 still belongs to the midnight window", now: day.Add(5*time.Hour - time.Second), windowStart: day},
		{name: "05:00:00 opens the 05:00 window", now: day.Add(5 * time.Hour), windowStart: day.Add(5 * time.Hour)},
		{name: "09:59:59 still belongs to the 05:00 window", now: day.Add(10*time.Hour - time.Second), windowStart: day.Add(5 * time.Hour)},
		{name: "10:00:00 opens the 10:00 window", now: day.Add(10 * time.Hour), windowStart: day.Add(10 * time.Hour)},
		{name: "19:59:59 still belongs to the 15:00 window", now: day.Add(20*time.Hour - time.Second), windowStart: day.Add(15 * time.Hour)},
		{name: "20:00:00 opens the last window of the day", now: day.Add(20 * time.Hour), windowStart: day.Add(20 * time.Hour)},
		{name: "23:59:59 of the last window ends at midnight", now: day.Add(24*time.Hour - time.Second), windowStart: day.Add(20 * time.Hour)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			start, end := model.HwReserveWindow(test.now)
			assert.Equal(t, test.windowStart.Unix(), start)
			assert.True(t, end-start == 5*3600 || end-start == 4*3600, "window must be five hours, or four for the last window of the day")
			assert.GreaterOrEqual(t, test.now.Unix(), start)
			assert.Less(t, test.now.Unix(), end)
		})
	}

	// The same instant in another timezone must give the same window: the
	// reserve is UTC, not server local time.
	local := time.Date(2026, 10, 20, 3, 30, 0, 0, time.FixedZone("UTC+3", 3*3600))
	start, _ := model.HwReserveWindow(local)
	assert.Equal(t, day.Unix(), start)
}

// TestHwCustomerLimitUSDRule pins the limit formula: one third until the phase-0
// measurement is done, then the rest minus a fifth of the capacity, never below
// zero.
func TestHwCustomerLimitUSDRule(t *testing.T) {
	tests := []struct {
		name        string
		capacity    float64
		bureauUsed  float64
		phase0Done  bool
		mode        string
		expectedUSD float64
	}{
		{name: "unknown capacity leaves no room", capacity: 0, bureauUsed: 0, expectedUSD: 0},
		{name: "before measurement, bureau idle", capacity: 30, bureauUsed: 0, expectedUSD: 10},
		{name: "before measurement, bureau at half", capacity: 30, bureauUsed: 15, expectedUSD: 10},
		{name: "before measurement, bureau at the full capacity", capacity: 30, bureauUsed: 30, expectedUSD: 10},
		{name: "after measurement, bureau idle", capacity: 30, bureauUsed: 0, phase0Done: true, expectedUSD: 24},
		{name: "after measurement, bureau at half", capacity: 30, bureauUsed: 15, phase0Done: true, expectedUSD: 9},
		{name: "after measurement, bureau at full capacity", capacity: 30, bureauUsed: 30, phase0Done: true, expectedUSD: 0},
		{name: "after measurement, bureau above capacity stays at zero", capacity: 30, bureauUsed: 45, phase0Done: true, expectedUSD: 0},
		{name: "pinned third mode ignores the measurement", capacity: 30, bureauUsed: 0, phase0Done: true, mode: HwReserveModeThird, expectedUSD: 10},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := HwCustomerLimitUSD(test.capacity, test.bureauUsed, test.phase0Done, test.mode)
			assert.InDelta(t, test.expectedUSD, got, 1e-9)
			assert.GreaterOrEqual(t, got, 0.0)
		})
	}
}

// TestHwBureauReserveSnapshotRefusesWithoutMeasurement proves that every gap in
// the measurement closes the window for customers instead of opening it: no
// capacity, no source, a failing source. It also pins the values of a working
// measurement and the window change.
func TestHwBureauReserveSnapshotRefusesWithoutMeasurement(t *testing.T) {
	hwReserveTestSetup(t)
	now := time.Now().UTC()

	t.Run("unknown capacity refuses", func(t *testing.T) {
		snapshot := HwBureauReserveSnapshot(context.Background(), now)
		assert.Equal(t, HwReserveDenyCapacityUnknown, snapshot.DenyReason)
		assert.Positive(t, snapshot.RetryAfterSeconds)
		assert.LessOrEqual(t, snapshot.RetryAfterSeconds, int(24*3600))
	})

	t.Run("capacity without a source refuses", func(t *testing.T) {
		hwReserveTestSetOption(t, model.HwWindowCapacityUSDOption, "30")
		snapshot := HwBureauReserveSnapshot(context.Background(), now)
		assert.Equal(t, HwReserveDenySourceUnavailable, snapshot.DenyReason)
		assert.False(t, snapshot.BureauSourceConfigured)
	})

	t.Run("failing source refuses", func(t *testing.T) {
		hwReserveTestSetOption(t, model.HwWindowCapacityUSDOption, "30")
		HwSetBureauUsageSource(&hwReserveFakeSource{err: errors.New("measurement unavailable")})
		snapshot := HwBureauReserveSnapshot(context.Background(), now)
		assert.Equal(t, HwReserveDenySourceUnavailable, snapshot.DenyReason)
	})

	t.Run("working source uses one third before the measurement", func(t *testing.T) {
		hwReserveTestSetup(t)
		hwReserveTestSetOption(t, model.HwWindowCapacityUSDOption, "30")
		source := &hwReserveFakeSource{used: 12}
		HwSetBureauUsageSource(source)
		snapshot := HwBureauReserveSnapshot(context.Background(), now)
		assert.Empty(t, snapshot.DenyReason)
		assert.InDelta(t, 10, snapshot.CustomerLimitUSD, 1e-9)
		assert.InDelta(t, 12, snapshot.BureauUsedUSD, 1e-9)
		assert.Equal(t, 1, source.calls)
	})

	t.Run("working source uses the rest minus the margin after the measurement", func(t *testing.T) {
		hwReserveTestSetup(t)
		hwReserveTestSetOption(t, model.HwWindowCapacityUSDOption, "30")
		hwReserveTestSetOption(t, model.HwPhase0MeasurementDoneOption, "true")
		HwSetBureauUsageSource(&hwReserveFakeSource{used: 6})
		snapshot := HwBureauReserveSnapshot(context.Background(), now)
		assert.Empty(t, snapshot.DenyReason)
		assert.InDelta(t, 18, snapshot.CustomerLimitUSD, 1e-9)
	})

	t.Run("customer usage at the limit refuses", func(t *testing.T) {
		hwReserveTestSetup(t)
		hwReserveTestSetOption(t, model.HwWindowCapacityUSDOption, "30")
		HwSetBureauUsageSource(&hwReserveFakeSource{used: 0})
		windowStart, _ := model.HwReserveWindow(now)
		// Ten dollars is exactly one third of the capacity.
		hwReserveTestInsertLog(t, 1, model.HwPaidGroupName, common.QuotaFromFloat(10*common.QuotaPerUnit), windowStart, model.LogTypeConsume)
		snapshot := HwBureauReserveSnapshot(context.Background(), now)
		assert.Equal(t, HwReserveDenyLimitReached, snapshot.DenyReason)
	})

	t.Run("customer usage below the limit passes", func(t *testing.T) {
		hwReserveTestSetup(t)
		hwReserveTestSetOption(t, model.HwWindowCapacityUSDOption, "30")
		HwSetBureauUsageSource(&hwReserveFakeSource{used: 0})
		windowStart, _ := model.HwReserveWindow(now)
		hwReserveTestInsertLog(t, 1, model.HwPaidGroupName, common.QuotaFromFloat(9*common.QuotaPerUnit), windowStart, model.LogTypeConsume)
		snapshot := HwBureauReserveSnapshot(context.Background(), now)
		assert.Empty(t, snapshot.DenyReason)
	})

	t.Run("pinned third mode needs no source", func(t *testing.T) {
		hwReserveTestSetup(t)
		t.Setenv(model.HwReserveModeEnv, HwReserveModeThird)
		hwReserveTestSetOption(t, model.HwWindowCapacityUSDOption, "30")
		snapshot := HwBureauReserveSnapshot(context.Background(), now)
		assert.Empty(t, snapshot.DenyReason)
		assert.InDelta(t, 10, snapshot.CustomerLimitUSD, 1e-9)
	})

	t.Run("an unknown mode follows the phase rule and stays closed", func(t *testing.T) {
		hwReserveTestSetup(t)
		t.Setenv(model.HwReserveModeEnv, "wide-open")
		hwReserveTestSetOption(t, model.HwWindowCapacityUSDOption, "30")
		hwReserveTestSetOption(t, model.HwPhase0MeasurementDoneOption, "true")
		snapshot := HwBureauReserveSnapshot(context.Background(), now)
		assert.Equal(t, HwReserveDenySourceUnavailable, snapshot.DenyReason)
	})
}

// TestHwCustomerWindowQuotaCountsOnlyCustomersInTheWindow proves the usage sum
// reads consume rows of free and paid customers inside the window only: bureau
// groups, other windows and error rows never count.
func TestHwCustomerWindowQuotaCountsOnlyCustomersInTheWindow(t *testing.T) {
	hwReserveTestSetup(t)
	now := time.Now().UTC()
	windowStart, windowEnd := model.HwReserveWindow(now)

	hwReserveTestInsertLog(t, 1, model.HwFreeGroupName, 100, windowStart, model.LogTypeConsume)
	hwReserveTestInsertLog(t, 2, model.HwPaidGroupName, 200, windowStart+1, model.LogTypeConsume)
	hwReserveTestInsertLog(t, 3, "default", 1000, windowStart+2, model.LogTypeConsume)
	hwReserveTestInsertLog(t, 2, model.HwPaidGroupName, 300, windowStart-1, model.LogTypeConsume)
	hwReserveTestInsertLog(t, 2, model.HwPaidGroupName, 5000, windowStart+3, model.LogTypeError)

	total, err := model.HwCustomerWindowQuota(windowStart, windowEnd)
	require.NoError(t, err)
	assert.Equal(t, int64(300), total)

	userTotal, err := model.HwUserWindowQuota(2, windowStart, windowEnd)
	require.NoError(t, err)
	assert.Equal(t, int64(200), userTotal, "another customer's usage must not count for this one")
}

// TestHwCustomerCeilingAndStatus pins the per-customer ceiling: derived from the
// capacity when the option is empty, overridable, lower for free than for paid,
// and reset by a new window.
func TestHwCustomerCeilingAndStatus(t *testing.T) {
	hwReserveTestSetup(t)
	now := time.Now().UTC()
	windowStart, _ := model.HwReserveWindow(now)

	assert.InDelta(t, 5, HwCustomerCeilingUSD(100, model.HwPaidGroupName), 1e-9)
	assert.InDelta(t, 2.5, HwCustomerCeilingUSD(100, model.HwFreeGroupName), 1e-9)
	assert.InDelta(t, hwReserveFallbackCeilingUSD, HwCustomerCeilingUSD(0, model.HwPaidGroupName), 1e-9)
	assert.Less(t, HwCustomerCeilingUSD(100, model.HwFreeGroupName), HwCustomerCeilingUSD(100, model.HwPaidGroupName))

	hwReserveTestSetOption(t, model.HwCustomerWindowCeilingUSDOption, "8")
	assert.InDelta(t, 8, HwCustomerCeilingUSD(100, model.HwPaidGroupName), 1e-9)
	assert.InDelta(t, 4, HwCustomerCeilingUSD(100, model.HwFreeGroupName), 1e-9)

	hwReserveTestSetup(t)
	// Free customer at its ceiling of 2.50 USD; the window capacity is 100, so
	// the global limit is far away and only the ceiling can refuse.
	hwReserveTestInsertLog(t, 11, model.HwFreeGroupName, common.QuotaFromFloat(2.5*common.QuotaPerUnit), windowStart, model.LogTypeConsume)
	hwReserveTestInsertLog(t, 12, model.HwPaidGroupName, common.QuotaFromFloat(4.9*common.QuotaPerUnit), windowStart, model.LogTypeConsume)

	free, err := HwCustomerReserveStatusNow(11, model.HwFreeGroupName, now)
	require.NoError(t, err)
	assert.True(t, free.Limited)
	assert.InDelta(t, 2.5, free.CeilingUSD, 1e-9)

	paid, err := HwCustomerReserveStatusNow(12, model.HwPaidGroupName, now)
	require.NoError(t, err)
	assert.False(t, paid.Limited)

	// The next window starts with an empty count for both customers.
	nextWindow := now.Add(5 * time.Hour)
	freeNext, err := HwCustomerReserveStatusNow(11, model.HwFreeGroupName, nextWindow)
	require.NoError(t, err)
	assert.False(t, freeNext.Limited)
	paidNext, err := HwCustomerReserveStatusNow(12, model.HwPaidGroupName, nextWindow)
	require.NoError(t, err)
	assert.False(t, paidNext.Limited)
}

// TestHwReserveMessagesNameNoSupplier proves the customer texts: Turkish and
// English, whole minutes, at least one, and free of supplier names.
func TestHwReserveMessagesNameNoSupplier(t *testing.T) {
	gin.SetMode(gin.TestMode)
	contextFor := func(language string) *gin.Context {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
		if language != "" {
			c.Request.Header.Set("Accept-Language", language)
		}
		return c
	}

	english := HwReserveBusyMessage(contextFor("en-US"), 600)
	assert.Contains(t, english, "10 minutes")
	assert.NotContains(t, strings.ToLower(english), "lumia")
	assert.NotContains(t, strings.ToLower(english), "openrouter")

	turkish := HwReserveBusyMessage(contextFor("tr-TR,tr;q=0.9"), 600)
	assert.Contains(t, turkish, "10 dakika")
	assert.NotContains(t, strings.ToLower(turkish), "lumia")
	assert.NotContains(t, strings.ToLower(turkish), "openrouter")

	ceilingTurkish := HwReserveCeilingMessage(contextFor("tr"), 0)
	assert.Contains(t, ceilingTurkish, "1 dakika", "a zero wait still says one minute")
	assert.NotContains(t, strings.ToLower(ceilingTurkish), "openrouter")

	ceilingEnglish := HwReserveCeilingMessage(contextFor("en"), 61)
	assert.Contains(t, ceilingEnglish, "2 minutes")
	assert.NotContains(t, strings.ToLower(ceilingEnglish), "lumia")

	assert.Equal(t, 1, HwReserveMinutes(0))
	assert.Equal(t, 1, HwReserveMinutes(60))
	assert.Equal(t, 2, HwReserveMinutes(61))
}

// The production usage source decides whether customer traffic may pass, so
// every odd answer of the bureau endpoint must end as an error (the gate stays
// closed) and the bearer token must travel only as a header.
func TestHwBureauUsageHTTPMeasureAcceptsOnlyAUsableAnswer(t *testing.T) {
	const window = int64(1_800_000_000)
	type answer struct {
		status int
		body   string
	}
	measure := func(t *testing.T, response answer) (float64, error, string) {
		t.Helper()
		var authorization string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authorization = r.Header.Get("Authorization")
			w.WriteHeader(response.status)
			_, _ = w.Write([]byte(response.body))
		}))
		t.Cleanup(server.Close)
		source := &hwBureauUsageHTTP{url: server.URL, token: "test-token", client: server.Client()}
		value, err := source.measure(context.Background(), window)
		return value, err, authorization
	}

	value, err, authorization := measure(t, answer{http.StatusOK, `{"window_start":1800000000,"used_usd":12.5,"capacity_usd":100}`})
	require.NoError(t, err)
	assert.Equal(t, 12.5, value)
	assert.Equal(t, "Bearer test-token", authorization)

	value, err, _ = measure(t, answer{http.StatusOK, `{"used_usd":3}`})
	require.NoError(t, err, "an answer without a window is accepted")
	assert.Equal(t, 3.0, value)

	for name, response := range map[string]answer{
		"other window": {http.StatusOK, `{"window_start":1799990000,"used_usd":1}`},
		"negative":     {http.StatusOK, `{"used_usd":-1}`},
		"not json":     {http.StatusOK, `<html>`},
		"server error": {http.StatusInternalServerError, `{"used_usd":1}`},
		"unauthorized": {http.StatusUnauthorized, ``},
	} {
		value, err, _ := measure(t, response)
		require.Error(t, err, name)
		assert.Zero(t, value, name)
		assert.NotContains(t, err.Error(), "test-token", name)
	}

	unreachable := &hwBureauUsageHTTP{url: "http://127.0.0.1:1", client: &http.Client{Timeout: time.Second}}
	_, err = unreachable.measure(context.Background(), window)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unreachable")
	assert.NotContains(t, err.Error(), "127.0.0.1", "the endpoint never appears in the error")

	_, err = (&hwBureauUsageHTTP{url: "http://bad host/", client: http.DefaultClient}).measure(context.Background(), window)
	require.Error(t, err)

	// Without a token no Authorization header is sent.
	var header string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{"used_usd":0}`))
	}))
	t.Cleanup(server.Close)
	_, err = (&hwBureauUsageHTTP{url: server.URL, client: server.Client()}).measure(context.Background(), window)
	require.NoError(t, err)
	assert.Empty(t, header)
}

// A burst of customer requests must not become a burst of measurements: one
// window is cached for the TTL, the error with it, and a new window or an
// expired entry measures again.
func TestHwBureauUsageHTTPCachesOneWindowForTheTTL(t *testing.T) {
	var calls int
	status := http.StatusOK
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(status)
		_, _ = w.Write([]byte(`{"used_usd":4}`))
	}))
	t.Cleanup(server.Close)
	source := &hwBureauUsageHTTP{url: server.URL, client: server.Client()}
	ctx := context.Background()

	value, err := source.UsedUSD(ctx, 100, 200)
	require.NoError(t, err)
	assert.Equal(t, 4.0, value)
	_, _ = source.UsedUSD(ctx, 100, 200)
	assert.Equal(t, 1, calls, "the second read comes from the cache")

	_, _ = source.UsedUSD(ctx, 300, 400)
	assert.Equal(t, 2, calls, "another window measures again")

	source.cacheAt = time.Now().Add(-2 * hwBureauUsageCacheTTL)
	_, _ = source.UsedUSD(ctx, 300, 400)
	assert.Equal(t, 3, calls, "an expired entry measures again")

	status = http.StatusBadGateway
	source.cacheAt = time.Now().Add(-2 * hwBureauUsageCacheTTL)
	_, err = source.UsedUSD(ctx, 300, 400)
	require.Error(t, err)
	_, err = source.UsedUSD(ctx, 300, 400)
	require.Error(t, err, "the failure is cached too")
	assert.Equal(t, 4, calls)
}

// The reserve and order settings come from the options table or the
// environment, both free text. A value that cannot be trusted must fall back
// instead of becoming a zero, negative or infinite limit.
func TestHwSettingParsersFallBackOnUntrustworthyValues(t *testing.T) {
	t.Run("environment float", func(t *testing.T) {
		const name = "HW_TEST_ENV_FLOAT"
		for value, want := range map[string]float64{
			"":         0,
			"  ":       0,
			"12.5":     12.5,
			" 3 ":      3,
			"-4":       -4,
			"abc":      0,
			"NaN":      0,
			"Inf":      0,
			"-Inf":     0,
			"1e999999": 0,
		} {
			t.Setenv(name, value)
			assert.Equal(t, want, hwEnvFloat(name), "value %q", value)
		}
	})

	t.Run("integer option", func(t *testing.T) {
		const key = "HwTestIntOption"
		assert.Equal(t, 7, hwIntOption(key, 7), "unset option")
		for value, want := range map[string]int{
			"":     7,
			"15":   15,
			" 20 ": 20,
			"0":    7,
			"-3":   7,
			"2.5":  7,
			"many": 7,
		} {
			hwReserveTestSetOption(t, key, value)
			assert.Equal(t, want, hwIntOption(key, 7), "value %q", value)
		}
	})

	t.Run("bool option", func(t *testing.T) {
		const key = "HwTestBoolOption"
		assert.True(t, hwBoolOption(key, true), "unset option")
		assert.False(t, hwBoolOption(key, false), "unset option")
		for value, want := range map[string]bool{"true": true, "TRUE": true, "1": true, "false": false, "False": false, "0": false} {
			hwReserveTestSetOption(t, key, value)
			assert.Equal(t, want, hwBoolOption(key, !want), "value %q", value)
		}
		hwReserveTestSetOption(t, key, "yes")
		assert.True(t, hwBoolOption(key, true), "unknown word keeps the fallback")
		assert.False(t, hwBoolOption(key, false), "unknown word keeps the fallback")
	})
}

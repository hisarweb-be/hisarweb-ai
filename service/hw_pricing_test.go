package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// hwPricingTestSetup brings the pricing tables and options into the shared test
// database and clears the HisarWeb pricing state, so every test starts from a
// known point.
func hwPricingTestSetup(t *testing.T) {
	t.Helper()
	require.NoError(t, model.DB.AutoMigrate(&model.Option{}, &model.HwPriceSnapshot{}))
	require.NoError(t, model.DB.Exec("DELETE FROM hw_price_snapshots").Error)
	hwDeletePricingOptions(t)
	if common.OptionMap == nil {
		common.OptionMap = make(map[string]string)
	}
	saved := map[string]string{}
	for _, key := range []string{HwOptionPriceListURL, HwOptionFxURL, HwOptionTryPerUsd, HwOptionTryPerUsdDate} {
		saved[key] = hwOptionValue(key)
		delete(common.OptionMap, key)
	}
	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		for key, value := range saved {
			common.OptionMap[key] = value
		}
		common.OptionMapRWMutex.Unlock()
		model.DB.Exec("DELETE FROM hw_price_snapshots")
		hwDeletePricingOptions(t)
	})
}

// hwDeletePricingOptions removes the persisted pricing options of this suite.
func hwDeletePricingOptions(t *testing.T) {
	t.Helper()
	require.NoError(t, model.DB.Where("key IN ?", []string{
		HwOptionPriceListURL, HwOptionFxURL, HwOptionTryPerUsd, HwOptionTryPerUsdDate,
	}).Delete(&model.Option{}).Error)
}

// hwSetOption puts one option into the option map for the duration of the test.
func hwSetOption(t *testing.T, key string, value string) {
	t.Helper()
	common.OptionMapRWMutex.Lock()
	common.OptionMap[key] = value
	common.OptionMapRWMutex.Unlock()
}

// hwTestPriceListBody is a miniature of the open price list with the real
// notation (prices per token in scientific notation).
const hwTestPriceListBody = `{
  "claude-sonnet-5-5": {"input_cost_per_token": 2e-06, "output_cost_per_token": 1e-05, "mode": "chat"},
  "claude-haiku-4-5": {"input_cost_per_token": 1e-06, "output_cost_per_token": 5e-06, "mode": "chat"},
  "claude-haiku-5-5": {"input_cost_per_token": 1e-07, "output_cost_per_token": 5e-07, "mode": "chat"},
  "some-other-model": {"input_cost_per_token": 3e-06, "output_cost_per_token": 1.5e-05, "mode": "chat"}
}`

// hwTestFxBody mirrors the TCMB daily bulletin: Date is MM/DD/YYYY, Tarih is
// DD.MM.YYYY, the USD forex-selling rate is the one we take.
const hwTestFxBody = `<?xml version="1.0" encoding="UTF-8"?>
<Tarih_Date Tarih="19.10.2026" Date="10/19/2026" Bulten_No="2026/197">
	<Currency Kod="AUD" CurrencyCode="AUD">
		<Isim>AVUSTRALYA DOLARI</Isim>
		<ForexSelling>34.6716</ForexSelling>
	</Currency>
	<Currency Kod="USD" CurrencyCode="USD">
		<Isim>ABD DOLARI</Isim>
		<CurrencyName>US DOLLAR</CurrencyName>
		<ForexBuying>49.4632</ForexBuying>
		<ForexSelling>49.5523</ForexSelling>
	</Currency>
</Tarih_Date>
`

func hwStartTestServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return server
}

// TestHwPriceSnapshotFetchStoresSourceTimeAndRateDate pins the snapshot
// content of one successful fetch: both source URLs, the fetch time, the rate
// validity date, the rate itself, and the sold models' prices in micro-dollars.
func TestHwPriceSnapshotFetchStoresSourceTimeAndRateDate(t *testing.T) {
	hwPricingTestSetup(t)
	priceServer := hwStartTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(hwTestPriceListBody))
	})
	fxServer := hwStartTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(hwTestFxBody))
	})
	fetchedAt := time.Date(2026, 10, 20, 6, 30, 0, 0, time.UTC)

	snapshot, err := HwFetchPriceSnapshot(context.Background(), priceServer.Client(), priceServer.URL, fxServer.URL, fetchedAt)
	require.NoError(t, err)

	assert.Equal(t, priceServer.URL, snapshot.PriceSourceUrl)
	assert.Equal(t, fxServer.URL, snapshot.FxSourceUrl)
	assert.Equal(t, fetchedAt.Unix(), snapshot.FetchedAt)
	assert.Equal(t, "2026-10-19", snapshot.FxDate)
	assert.Equal(t, "49.5523", snapshot.TryPerUsd)
	assert.Equal(t, 4, snapshot.ListedCount)
	prices, err := snapshot.HwSnapshotModelPrices()
	require.NoError(t, err)
	assert.Equal(t, model.HwModelPrice{InputMicroUsd: 2_000_000, OutputMicroUsd: 10_000_000}, prices["claude-sonnet-5-5"])
	// Haiku 4.5 is billed as its successor Haiku 5.5, never at its own older, higher price.
	assert.Equal(t, model.HwModelPrice{InputMicroUsd: 100_000, OutputMicroUsd: 500_000}, prices["claude-haiku-4-5"])
	assert.Equal(t, model.HwModelPrice{InputMicroUsd: 100_000, OutputMicroUsd: 500_000}, prices["claude-haiku-5-5"])
	assert.NotContains(t, prices, "some-other-model", "only sold models belong in the snapshot")
}

// TestHwPriceRefreshFailedFetchKeepsOldSnapshotAndMarksStale pins the failure
// path: a failed refresh leaves the previous snapshot in place, with its dates,
// and marks it stale instead of replacing anything.
func TestHwPriceRefreshFailedFetchKeepsOldSnapshotAndMarksStale(t *testing.T) {
	hwPricingTestSetup(t)
	priceServer := hwStartTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(hwTestPriceListBody))
	})
	fxServer := hwStartTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(hwTestFxBody))
	})
	hwSetOption(t, HwOptionPriceListURL, priceServer.URL)
	hwSetOption(t, HwOptionFxURL, fxServer.URL)

	firstFetch := time.Date(2026, 10, 19, 6, 0, 0, 0, time.UTC)
	require.NoError(t, HwRefreshPriceSnapshot(context.Background(), priceServer.Client(), firstFetch))
	original, err := model.HwLatestPriceSnapshot()
	require.NoError(t, err)
	require.Equal(t, int64(0), original.StaleAt)

	// The price source now fails; the rate source keeps working, which proves
	// the cycle fails as a whole and keeps the old data.
	failingServer := hwStartTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	})
	hwSetOption(t, HwOptionPriceListURL, failingServer.URL)

	err = HwRefreshPriceSnapshot(context.Background(), failingServer.Client(), time.Date(2026, 10, 20, 6, 0, 0, 0, time.UTC))
	require.Error(t, err)

	after, err := model.HwLatestPriceSnapshot()
	require.NoError(t, err)
	assert.Equal(t, original.Id, after.Id, "the old snapshot must stay the current one")
	assert.Equal(t, original.FetchedAt, after.FetchedAt)
	assert.Equal(t, original.FxDate, after.FxDate)
	assert.Equal(t, original.TryPerUsd, after.TryPerUsd)
	assert.Greater(t, after.StaleAt, int64(0), "the old snapshot must be marked stale")
}

// TestHwPriceRefreshRejectsInvalidRate refuses a zero, negative, or non-numeric
// rate: no snapshot is written and no rate is published.
func TestHwPriceRefreshRejectsInvalidRate(t *testing.T) {
	hwPricingTestSetup(t)
	priceServer := hwStartTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(hwTestPriceListBody))
	})
	hwSetOption(t, HwOptionPriceListURL, priceServer.URL)

	for name, rate := range map[string]string{
		"zero":         "0",
		"negative":     "-3.5",
		"not-a-number": "abc",
		"empty":        "",
	} {
		t.Run(name, func(t *testing.T) {
			fxBody := `<?xml version="1.0"?><Tarih_Date Tarih="19.10.2026" Date="10/19/2026">` +
				`<Currency Kod="USD" CurrencyCode="USD"><ForexSelling>` + rate + `</ForexSelling></Currency></Tarih_Date>`
			fxServer := hwStartTestServer(t, func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(fxBody))
			})
			_, err := HwFetchPriceSnapshot(context.Background(), fxServer.Client(), priceServer.URL, fxServer.URL, time.Now())
			require.Error(t, err)
		})
	}
	var count int64
	require.NoError(t, model.DB.Model(&model.HwPriceSnapshot{}).Count(&count).Error)
	assert.Equal(t, int64(0), count, "no snapshot may be written from an invalid rate")
	assert.Equal(t, "", hwOptionValue(HwOptionTryPerUsd), "no rate may be published from an invalid rate")
}

// TestHwTryPriceExactKurus pins the TRY conversion: dollar value x rate x 1.2
// (20 % KDV included), rounded to whole kuruş, with decimal arithmetic only.
// The expectations were computed independently with exact decimal math.
func TestHwTryPriceExactKurus(t *testing.T) {
	cases := []struct {
		usd     string
		rate    string
		wantTry string
	}{
		{"1", "10", "12"},
		{"0.0375", "1", "0.05"}, // 0.045 rounds half away from zero to 0.05
		{"0.0125", "1", "0.02"}, // 0.015 rounds up
		{"0.123456", "49.5523", "7.34"},
		{"12345.6789", "49.5523", "734108.14"},
		{"0.0000001", "49.5523", "0"}, // below one kuruş rounds to zero
		{"1000.005", "34.5678", "41481.57"},
		{"7", "49.5523", "416.24"},
	}
	for _, tc := range cases {
		got, err := HwTryPrice(decimal.RequireFromString(tc.usd), decimal.RequireFromString(tc.rate))
		require.NoError(t, err, "usd=%s rate=%s", tc.usd, tc.rate)
		assert.Equal(t, tc.wantTry, got.String(), "usd=%s rate=%s", tc.usd, tc.rate)
	}

	_, err := HwTryPrice(decimal.NewFromInt(1), decimal.Zero)
	assert.ErrorIs(t, err, ErrHwInvalidRate)
	_, err = HwTryPrice(decimal.NewFromInt(-1), decimal.NewFromInt(10))
	assert.ErrorIs(t, err, ErrHwNegativeUsdValue)
}

// TestHwApplyTryRateOptionOnlyNewerValid pins the option update rule: a valid,
// strictly newer rate replaces the published one; an equal or older date
// changes nothing, and an invalid rate is refused.
func TestHwApplyTryRateOptionOnlyNewerValid(t *testing.T) {
	hwPricingTestSetup(t)

	require.NoError(t, hwApplyTryRateOption(decimal.NewFromInt(40), "2026-10-18"))
	assert.Equal(t, "40", hwOptionValue(HwOptionTryPerUsd))
	assert.Equal(t, "2026-10-18", hwOptionValue(HwOptionTryPerUsdDate))

	// Older date: the published rate stays.
	require.NoError(t, hwApplyTryRateOption(decimal.NewFromInt(41), "2026-10-17"))
	assert.Equal(t, "40", hwOptionValue(HwOptionTryPerUsd))
	assert.Equal(t, "2026-10-18", hwOptionValue(HwOptionTryPerUsdDate))

	// Same date: no rewrite, so a repeated fetch of the same bulletin is a no-op.
	require.NoError(t, hwApplyTryRateOption(decimal.NewFromInt(42), "2026-10-18"))
	assert.Equal(t, "40", hwOptionValue(HwOptionTryPerUsd))

	// Newer date: both options move.
	require.NoError(t, hwApplyTryRateOption(decimal.NewFromFloat(43.25), "2026-10-19"))
	assert.Equal(t, "43.25", hwOptionValue(HwOptionTryPerUsd))
	assert.Equal(t, "2026-10-19", hwOptionValue(HwOptionTryPerUsdDate))

	// An invalid rate is refused and changes nothing.
	require.ErrorIs(t, hwApplyTryRateOption(decimal.Zero, "2026-10-20"), ErrHwInvalidRate)
	assert.Equal(t, "43.25", hwOptionValue(HwOptionTryPerUsd))

	// The published pair is also persisted as options.
	for key, want := range map[string]string{HwOptionTryPerUsd: "43.25", HwOptionTryPerUsdDate: "2026-10-19"} {
		var stored model.Option
		require.NoError(t, model.DB.Where("key = ?", key).First(&stored).Error)
		assert.Equal(t, want, stored.Value)
	}
}

const hwTestFxBodyWithEur = `<?xml version="1.0" encoding="UTF-8"?>
<Tarih_Date Tarih="19.10.2026" Date="10/19/2026" Bulten_No="2026/197">
	<Currency Kod="EUR" CurrencyCode="EUR"><ForexSelling>57.5</ForexSelling></Currency>
	<Currency Kod="USD" CurrencyCode="USD"><ForexSelling>49.5523</ForexSelling></Currency>
</Tarih_Date>
`

// The EUR per USD rate is the cross rate of the two lira rates in the same
// bulletin, rounded to six decimals; a missing or broken EUR line is an error
// of its own and never touches the lira.
func TestHwParseTcmbEurPerUsd(t *testing.T) {
	usdTry := decimal.RequireFromString("49.5523")
	rate, err := hwParseTcmbEurPerUsd([]byte(hwTestFxBodyWithEur), usdTry)
	require.NoError(t, err)
	assert.Equal(t, "0.861779", rate.String())

	_, err = hwParseTcmbEurPerUsd([]byte(hwTestFxBody), usdTry)
	assert.ErrorIs(t, err, errHwEurRateMissing, "no EUR line")
	_, err = hwParseTcmbEurPerUsd([]byte("not xml"), usdTry)
	assert.ErrorIs(t, err, errHwEurRateMissing)
	for _, selling := range []string{"abc", "0", "-3"} {
		body := strings.Replace(hwTestFxBodyWithEur, "57.5", selling, 1)
		_, err = hwParseTcmbEurPerUsd([]byte(body), usdTry)
		assert.ErrorIs(t, err, errHwEurRateInvalid, selling)
	}
	_, err = hwParseTcmbEurPerUsd([]byte(hwTestFxBodyWithEur), decimal.Zero)
	assert.ErrorIs(t, err, errHwEurRateInvalid)
}

func hwEurTestCleanOptions(t *testing.T) {
	t.Helper()
	require.NoError(t, model.DB.AutoMigrate(&model.Option{}))
	keys := []string{model.HwEurPerUsdOption, model.HwEurPerUsdDateOption, model.HwEurPresetBaseOption}
	clean := func() {
		model.DB.Where("key IN ?", keys).Delete(&model.Option{})
		common.OptionMapRWMutex.Lock()
		for _, key := range keys {
			delete(common.OptionMap, key)
		}
		common.OptionMapRWMutex.Unlock()
	}
	clean()
	t.Cleanup(clean)
}

// Same guard as the lira rate (valid, strictly newer), and the preset base rate
// follows only a move of more than 3 percent so the presets do not jump daily.
func TestHwApplyEurRateOptionDateRuleAndPresetBase(t *testing.T) {
	hwPricingTestSetup(t)
	hwEurTestCleanOptions(t)

	require.NoError(t, hwApplyEurRateOption(decimal.RequireFromString("0.86"), "2026-10-18"))
	assert.Equal(t, "0.86", hwOptionValue(model.HwEurPerUsdOption))
	assert.Equal(t, "2026-10-18", hwOptionValue(model.HwEurPerUsdDateOption))
	assert.Equal(t, "0.86", hwOptionValue(model.HwEurPresetBaseOption), "the first rate is the base")

	// Older and equal dates change nothing.
	require.NoError(t, hwApplyEurRateOption(decimal.RequireFromString("0.90"), "2026-10-17"))
	require.NoError(t, hwApplyEurRateOption(decimal.RequireFromString("0.90"), "2026-10-18"))
	assert.Equal(t, "0.86", hwOptionValue(model.HwEurPerUsdOption))

	// A small move updates the rate but keeps the base.
	require.NoError(t, hwApplyEurRateOption(decimal.RequireFromString("0.87"), "2026-10-19"))
	assert.Equal(t, "0.87", hwOptionValue(model.HwEurPerUsdOption))
	assert.Equal(t, "0.86", hwOptionValue(model.HwEurPresetBaseOption))

	// More than 3 percent away from the base moves the base too.
	require.NoError(t, hwApplyEurRateOption(decimal.RequireFromString("0.90"), "2026-10-20"))
	assert.Equal(t, "0.9", hwOptionValue(model.HwEurPresetBaseOption))
	assert.Equal(t, "2026-10-20", hwOptionValue(model.HwEurPerUsdDateOption))

	require.ErrorIs(t, hwApplyEurRateOption(decimal.Zero, "2026-10-21"), ErrHwInvalidRate)
	assert.Equal(t, "0.9", hwOptionValue(model.HwEurPerUsdOption))
}

func hwCurrencyTestOptions(t *testing.T, values map[string]string) {
	t.Helper()
	common.OptionMapRWMutex.Lock()
	if common.OptionMap == nil {
		common.OptionMap = make(map[string]string)
	}
	saved := map[string]string{}
	present := map[string]bool{}
	for key, value := range values {
		saved[key], present[key] = common.OptionMap[key]
		common.OptionMap[key] = value
	}
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		defer common.OptionMapRWMutex.Unlock()
		for key := range values {
			if present[key] {
				common.OptionMap[key] = saved[key]
			} else {
				delete(common.OptionMap, key)
			}
		}
	})
}

func TestHwCurrencyRateAndRange(t *testing.T) {
	now := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)
	hwCurrencyTestOptions(t, map[string]string{
		model.HwTryPerUsdOption:     "40",
		model.HwEurPerUsdOption:     "0.8",
		model.HwEurPerUsdDateOption: "2026-10-17", // seven days old: still fresh
		model.HwMinTopUpTryOption:   "",
		model.HwMaxTopUpTryOption:   "",
	})

	for currency, want := range map[string]string{"USD": "1", "TRY": "40", "EUR": "0.8"} {
		rate, err := hwCurrencyRate(currency, now)
		require.NoError(t, err, currency)
		assert.Equal(t, want, rate.String(), currency)
	}
	_, err := hwCurrencyRate("GBP", now)
	assert.ErrorIs(t, err, ErrHwCurrencyUnsupported)
	_, err = hwCurrencyRate("EUR", now.Add(25*time.Hour))
	assert.ErrorIs(t, err, ErrHwTopUpRateMissing, "older than seven days")

	// 100..20000 lira, converted through the dollar: the minimum rounds up, the
	// maximum down.
	for currency, want := range map[string][2]int{"TRY": {100, 20000}, "EUR": {2, 400}, "USD": {3, 500}} {
		minAmount, maxAmount, err := HwTopUpRange(currency, now)
		require.NoError(t, err, currency)
		assert.Equal(t, want, [2]int{minAmount, maxAmount}, currency)
	}
	_, _, err = HwTopUpRange("EUR", now.Add(25*time.Hour))
	assert.ErrorIs(t, err, ErrHwTopUpRateMissing)

	hwCurrencyTestOptions(t, map[string]string{model.HwEurPerUsdOption: ""})
	_, err = hwCurrencyRate("EUR", now)
	assert.ErrorIs(t, err, ErrHwTopUpRateMissing, "no rate")
}

func TestHwTidyFloorAndPresets(t *testing.T) {
	for in, want := range map[string]int{
		"7.9": 7, "19.99": 19, "20": 20, "22": 20, "97": 95, "99.9": 95, "100": 100,
		"234": 230, "999": 990, "1234": 1200, "9999": 9950, "12345": 12000,
	} {
		assert.Equal(t, want, hwTidyFloor(decimal.RequireFromString(in)), in)
	}
	one := decimal.NewFromInt(1)
	assert.Equal(t, []int{250, 500, 1000, 2000, 5000}, hwPresets("TRY", one, 100, 20000))
	assert.Equal(t, []int{10, 25, 50, 100, 250}, hwPresets("USD", one, 3, 500))
	// 10, 25, 50, 100 and 250 dollar at 0.86: 8, 21 (20), 43 (40), 86 (85), 215 (210).
	assert.Equal(t, []int{8, 20, 40, 85, 210}, hwPresets("EUR", decimal.RequireFromString("0.86"), 2, 400))
	// Presets outside the bounds drop out.
	assert.Equal(t, []int{40, 85}, hwPresets("EUR", decimal.RequireFromString("0.86"), 30, 100))
}

func TestHwCurrenciesStatus(t *testing.T) {
	hwStripeTestSetup(t)
	now := time.Now()
	hwStripeTestSetEurRate(t, "0.8", now)
	hwCurrencyTestOptions(t, map[string]string{model.HwEurPresetBaseOption: "0.86"})

	status := HwCurrencies(now)
	assert.Equal(t, []string{"TRY", "EUR", "USD"}, status.Currencies)
	assert.Equal(t, map[string]string{"TRY": "40", "EUR": "0.8", "USD": "1"}, status.Rates)
	assert.Equal(t, HwCurrencyBounds{Min: 100, Max: 20000, Presets: []int{250, 500, 1000, 2000, 5000}}, status.TopUp["TRY"])
	assert.Equal(t, HwCurrencyBounds{Min: 2, Max: 400, Presets: []int{8, 20, 40, 85, 210}}, status.TopUp["EUR"], "presets follow the stored base rate")
	assert.Equal(t, HwCurrencyBounds{Min: 3, Max: 500, Presets: []int{10, 25, 50, 100, 250}}, status.TopUp["USD"])

	// A stale EUR rate drops only EUR.
	stale := HwCurrencies(now.Add(8 * 24 * time.Hour))
	assert.Equal(t, []string{"TRY", "USD"}, stale.Currencies)
	assert.NotContains(t, stale.Rates, "EUR")

	// Without Stripe only the lira is sold.
	t.Setenv(HwStripeSecretKeyEnv, "")
	assert.Equal(t, []string{"TRY"}, HwCurrencies(now).Currencies)
}

// A rate that is dated after today (or cannot be read) would stay "newer" than
// every real bulletin, so publishing refuses it and leaves the options alone.
func TestHwApplyRateOptionsRefuseFutureAndUnreadableDates(t *testing.T) {
	hwPricingTestSetup(t)
	hwEurTestCleanOptions(t)
	require.NoError(t, hwApplyTryRateOption(decimal.NewFromInt(40), "2026-10-18"))
	require.NoError(t, hwApplyEurRateOption(decimal.RequireFromString("0.86"), "2026-10-18"))

	tomorrow := time.Now().UTC().Add(24 * time.Hour).Format("2006-01-02")
	for _, date := range []string{tomorrow, "2099-01-01", "18.10.2026", "", "garbage"} {
		require.ErrorIs(t, hwApplyTryRateOption(decimal.NewFromInt(99), date), ErrHwInvalidRate, "TRY %q", date)
		require.ErrorIs(t, hwApplyEurRateOption(decimal.RequireFromString("0.5"), date), ErrHwInvalidRate, "EUR %q", date)
	}
	assert.Equal(t, "40", hwOptionValue(HwOptionTryPerUsd))
	assert.Equal(t, "2026-10-18", hwOptionValue(HwOptionTryPerUsdDate))
	assert.Equal(t, "0.86", hwOptionValue(model.HwEurPerUsdOption))
	assert.Equal(t, "2026-10-18", hwOptionValue(model.HwEurPerUsdDateOption))

	today := time.Now().UTC().Format("2006-01-02")
	require.NoError(t, hwApplyTryRateOption(decimal.NewFromInt(41), today), "today is allowed")
	assert.Equal(t, "41", hwOptionValue(HwOptionTryPerUsd))
}

func TestHwRateDateUsable(t *testing.T) {
	now := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		date     string
		required bool
		want     bool
	}{
		{"2026-10-24", true, true},
		{"2026-10-17", true, true},   // seven days: still fresh
		{"2026-10-16", true, false},  // eight days: stale
		{"2026-10-25", true, false},  // tomorrow
		{"2099-01-01", false, false}, // far future, even when a date is optional
		{"24.10.2026", false, false},
		{"", true, false},
		{"", false, true}, // a hand-set rate has no date
	} {
		assert.Equal(t, tc.want, hwRateDateUsable(tc.date, now, tc.required), "%q required=%v", tc.date, tc.required)
	}
}

// The lira follows the same age and future rules as the euro: a stale or future
// dated rate reads as "rate missing" and TRY is not sold. A rate without a date
// (set by hand) stays usable.
func TestHwTryRateNeedsAFreshDate(t *testing.T) {
	now := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name, date string
		ok         bool
	}{
		{"fresh", "2026-10-23", true},
		{"seven days old", "2026-10-17", true},
		{"stale", "2026-10-16", false},
		{"in the future", "2026-10-25", false},
		{"unreadable", "soon", false},
		{"no date at all", "", true},
	} {
		hwCurrencyTestOptions(t, map[string]string{
			model.HwTryPerUsdOption: "40",
			HwOptionTryPerUsdDate:   tc.date,
		})
		rate, err := hwCurrencyRate("TRY", now)
		if tc.ok {
			require.NoError(t, err, tc.name)
			assert.Equal(t, "40", rate.String(), tc.name)
		} else {
			assert.ErrorIs(t, err, ErrHwTopUpRateMissing, tc.name)
		}
		// The other limits are converted through the lira, so they follow it.
		_, _, rangeErr := HwTopUpRange("USD", now)
		assert.Equal(t, tc.ok, rangeErr == nil, tc.name)
	}
}

// A stale lira rate takes TRY out of the list the shop sells in. The euro and
// dollar limits and the discount cap are worked out through the lira, so they
// wait for a fresh lira rate too (both come from the same bulletin).
func TestHwCurrenciesDropsTryWithAStaleRate(t *testing.T) {
	hwStripeTestSetup(t)
	now := time.Now()
	hwStripeTestSetEurRate(t, "0.8", now)
	hwCurrencyTestOptions(t, map[string]string{
		model.HwTryPerUsdOption: "40",
		HwOptionTryPerUsdDate:   now.UTC().Add(-8 * 24 * time.Hour).Format("2006-01-02"),
	})
	status := HwCurrencies(now)
	assert.NotContains(t, status.Currencies, "TRY")
	assert.NotContains(t, status.Rates, "TRY")
	assert.Empty(t, status.Currencies)
}

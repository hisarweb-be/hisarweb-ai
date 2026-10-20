package service

import (
	"context"
	"net/http"
	"net/http/httptest"
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
	assert.Equal(t, 3, snapshot.ListedCount)
	prices, err := snapshot.HwSnapshotModelPrices()
	require.NoError(t, err)
	assert.Equal(t, model.HwModelPrice{InputMicroUsd: 2_000_000, OutputMicroUsd: 10_000_000}, prices["claude-sonnet-5-5"])
	assert.Equal(t, model.HwModelPrice{InputMicroUsd: 1_000_000, OutputMicroUsd: 5_000_000}, prices["claude-haiku-4-5"])
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

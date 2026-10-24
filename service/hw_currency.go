package service

// Multi-currency top-up: rates, sellable currencies, bounds and presets.
//
// Credit stays in USD quota. A rate is the number of units of a currency per one
// US dollar (USD = 1, TRY = HwTryPerUsd, EUR = HwEurPerUsd). Costs of the card
// processor are carried by the seller: no fee is added to a price and none is
// taken off a credit.

import (
	"errors"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/model"

	"github.com/shopspring/decimal"
)

const (
	// hwEurRateMaxAgeDays is how old the EUR rate may be before EUR is no
	// longer sold.
	hwEurRateMaxAgeDays = 7

	// hwRateMaxAgeDays is how old a published rate may be.
	hwRateMaxAgeDays = hwEurRateMaxAgeDays
)

// hwRateDateUsable tells whether the validity date of a published rate (a
// YYYY-MM-DD date in UTC) can be used at now: not unparseable, not after today
// and not older than hwRateMaxAgeDays. An empty date is usable only when the
// rate does not need one.
func hwRateDateUsable(date string, now time.Time, dateRequired bool) bool {
	date = strings.TrimSpace(date)
	if date == "" {
		return !dateRequired
	}
	published, err := time.Parse("2006-01-02", date)
	if err != nil {
		return false
	}
	today := now.UTC().Truncate(24 * time.Hour)
	return !published.After(today) && today.Sub(published) <= hwRateMaxAgeDays*24*time.Hour
}

// hwPresetHysteresis is the relative rate change that is needed before the
// presets of a converted currency are made again.
var hwPresetHysteresis = decimal.RequireFromString("0.03")

// hwPresetAnchorsUSD are the preset sizes in dollars; other currencies use the
// anchors multiplied by the rate and rounded down to a tidy number.
var hwPresetAnchorsUSD = []int{10, 25, 50, 100, 250}

// hwPresetsTRY are the fixed lira presets.
var hwPresetsTRY = []int{250, 500, 1000, 2000, 5000}

// ErrHwCurrencyUnsupported rejects a currency the shop does not sell at all.
var ErrHwCurrencyUnsupported = errors.New("currency is not supported")

// HwNormalizeCurrency upper-cases a client currency value; empty means TRY, the
// currency of the old request shape.
func HwNormalizeCurrency(currency string) string {
	currency = strings.ToUpper(strings.TrimSpace(currency))
	if currency == "" {
		return model.HwCurrencyTRY
	}
	return currency
}

// HwRequestCurrency is the currency a top-up request asks for.
func HwRequestCurrency(request HwTopUpRequest) string {
	return HwNormalizeCurrency(request.Currency)
}

// hwCurrencyRate returns the units of currency per one dollar. TRY needs the
// published rate; EUR needs a valid rate that is at most seven days old; USD is
// always 1. An unknown currency is unsupported, a missing rate is a missing rate.
func hwCurrencyRate(currency string, now time.Time) (decimal.Decimal, error) {
	switch currency {
	case model.HwCurrencyUSD:
		return decimal.NewFromInt(1), nil
	case model.HwCurrencyTRY:
		return hwTryPerUsd(now)
	case model.HwCurrencyEUR:
		rate, err := decimal.NewFromString(hwOption(model.HwEurPerUsdOption))
		if err != nil || !rate.IsPositive() {
			return decimal.Decimal{}, ErrHwTopUpRateMissing
		}
		if !hwRateDateUsable(hwOption(model.HwEurPerUsdDateOption), now, true) {
			return decimal.Decimal{}, ErrHwTopUpRateMissing
		}
		return rate, nil
	default:
		return decimal.Decimal{}, ErrHwCurrencyUnsupported
	}
}

// hwDiscountCap is the first-purchase discount cap in currency: 1000 TRY, or the
// same value in dollars converted at the rate of that currency.
func hwDiscountCap(currency string, rate decimal.Decimal, now time.Time) (decimal.Decimal, error) {
	if currency == model.HwCurrencyTRY {
		return model.HwFirstPurchaseDiscountCapTRY, nil
	}
	tryRate, err := hwTryPerUsd(now)
	if err != nil {
		return decimal.Decimal{}, err
	}
	return model.HwFirstPurchaseDiscountCapTRY.Div(tryRate).Mul(rate).Round(2), nil
}

// HwTopUpRange returns the list-value bounds in whole units of currency. TRY
// uses the configured bounds; the others convert them through USD, the minimum
// rounded up and the maximum rounded down.
func HwTopUpRange(currency string, now time.Time) (minAmount int, maxAmount int, err error) {
	minTry := hwIntOption(model.HwMinTopUpTryOption, hwDefaultMinTopUpTry)
	maxTry := hwIntOption(model.HwMaxTopUpTryOption, hwDefaultMaxTopUpTry)
	if maxTry < minTry {
		maxTry = minTry
	}
	if currency == model.HwCurrencyTRY {
		return minTry, maxTry, nil
	}
	rate, err := hwCurrencyRate(currency, now)
	if err != nil {
		return 0, 0, err
	}
	tryRate, err := hwTryPerUsd(now)
	if err != nil {
		return 0, 0, err
	}
	toCurrency := rate.Div(tryRate)
	minAmount = int(decimal.NewFromInt(int64(minTry)).Mul(toCurrency).Ceil().IntPart())
	maxAmount = int(decimal.NewFromInt(int64(maxTry)).Mul(toCurrency).Floor().IntPart())
	if minAmount < 1 {
		minAmount = 1
	}
	if maxAmount < minAmount {
		maxAmount = minAmount
	}
	return minAmount, maxAmount, nil
}

// hwTidyFloor rounds a positive amount down to a tidy number: whole under 20,
// steps of 5 under 100, 10 under 1000, 50 under 10000 and 500 above that.
func hwTidyFloor(value decimal.Decimal) int {
	step := int64(1)
	switch {
	case value.LessThan(decimal.NewFromInt(20)):
	case value.LessThan(decimal.NewFromInt(100)):
		step = 5
	case value.LessThan(decimal.NewFromInt(1000)):
		step = 10
	case value.LessThan(decimal.NewFromInt(10000)):
		step = 50
	default:
		step = 500
	}
	whole := value.Floor().IntPart()
	return int(whole - whole%step)
}

// hwPresets lists the preset amounts of a currency inside [minAmount, maxAmount].
// base is the rate the presets are made with (see hwApplyEurRateOption).
func hwPresets(currency string, base decimal.Decimal, minAmount, maxAmount int) []int {
	var candidates []int
	switch currency {
	case model.HwCurrencyTRY:
		candidates = hwPresetsTRY
	case model.HwCurrencyUSD:
		candidates = hwPresetAnchorsUSD
	default:
		for _, anchor := range hwPresetAnchorsUSD {
			candidates = append(candidates, hwTidyFloor(decimal.NewFromInt(int64(anchor)).Mul(base)))
		}
	}
	presets := make([]int, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate < minAmount || candidate > maxAmount {
			continue
		}
		if len(presets) > 0 && presets[len(presets)-1] == candidate {
			continue
		}
		presets = append(presets, candidate)
	}
	return presets
}

// HwCurrencyBounds is the top-up bounds and presets of one currency in whole
// units, as /api/status publishes them.
type HwCurrencyBounds struct {
	Min     int   `json:"min"`
	Max     int   `json:"max"`
	Presets []int `json:"presets"`
}

// HwCurrencyStatus is the public multi-currency state of the shop.
type HwCurrencyStatus struct {
	Currencies []string
	Rates      map[string]string
	TopUp      map[string]HwCurrencyBounds
}

// HwCurrencies describes which currencies are sellable now. It reads options, so
// it must not be called while holding the option map lock. Without Stripe only
// TRY (Revolut) is sold; without a TRY rate nothing is, because the bounds of
// the other currencies derive from it.
func HwCurrencies(now time.Time) HwCurrencyStatus {
	status := HwCurrencyStatus{Currencies: []string{}, Rates: map[string]string{}, TopUp: map[string]HwCurrencyBounds{}}
	candidates := []string{model.HwCurrencyTRY}
	if HwStripePaymentAvailable() {
		candidates = append(candidates, model.HwCurrencyEUR, model.HwCurrencyUSD)
	}
	for _, currency := range candidates {
		rate, err := hwCurrencyRate(currency, now)
		if err != nil {
			continue
		}
		minAmount, maxAmount, err := HwTopUpRange(currency, now)
		if err != nil {
			continue
		}
		base := rate
		if currency == model.HwCurrencyEUR {
			if stored, err := decimal.NewFromString(hwOption(model.HwEurPresetBaseOption)); err == nil && stored.IsPositive() {
				base = stored
			}
		}
		status.Currencies = append(status.Currencies, currency)
		status.Rates[currency] = rate.String()
		status.TopUp[currency] = HwCurrencyBounds{Min: minAmount, Max: maxAmount, Presets: hwPresets(currency, base, minAmount, maxAmount)}
	}
	return status
}

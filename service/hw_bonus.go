package service

// HisarWeb AI top-up bonus: a payment at or above a threshold earns extra credit
// of the same currency. The thresholds and the bonus amounts are options (see
// model.HwBonus*Option), so they change in the admin settings without a deploy.
// The bonus is worked out once, when the order is created, and stored on the
// order; the settlement then books exactly that, so the price the customer
// agreed to is the credit the customer gets even if the settings change before
// the payment arrives.

import (
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/shopspring/decimal"
)

// HwBonusRule is the bonus of one currency, in whole units of that currency.
// A zero Min or Amount means the currency has no bonus.
type HwBonusRule struct {
	Currency string `json:"currency"`
	Min      int    `json:"min"`
	Amount   int    `json:"amount"`
}

// hwBonusOptionKeys names the two options of a currency.
func hwBonusOptionKeys(currency string) (minKey, amountKey string, ok bool) {
	switch strings.ToUpper(strings.TrimSpace(currency)) {
	case model.HwCurrencyUSD:
		return model.HwBonusUsdMinOption, model.HwBonusUsdAmountOption, true
	case model.HwCurrencyEUR:
		return model.HwBonusEurMinOption, model.HwBonusEurAmountOption, true
	case model.HwCurrencyTRY:
		return model.HwBonusTryMinOption, model.HwBonusTryAmountOption, true
	}
	return "", "", false
}

func hwBonusOption(key string) int {
	value, err := strconv.Atoi(hwOption(key))
	if err != nil || value < 0 {
		return 0
	}
	return value
}

// HwBonusRuleFor reads the current bonus rule of a currency. An unreadable or
// negative option counts as zero, which switches the bonus off rather than
// guessing a value.
func HwBonusRuleFor(currency string) HwBonusRule {
	minKey, amountKey, ok := hwBonusOptionKeys(currency)
	if !ok {
		return HwBonusRule{Currency: strings.ToUpper(strings.TrimSpace(currency))}
	}
	rule := HwBonusRule{
		Currency: strings.ToUpper(strings.TrimSpace(currency)),
		Min:      hwBonusOption(minKey),
		Amount:   hwBonusOption(amountKey),
	}
	if rule.Min <= 0 || rule.Amount <= 0 {
		rule.Min, rule.Amount = 0, 0
	}
	return rule
}

// HwBonusRules lists the rules of every currency that can be paid in, for the
// pages that announce the bonus before the customer pays.
func HwBonusRules() []HwBonusRule {
	return []HwBonusRule{
		HwBonusRuleFor(model.HwCurrencyTRY),
		HwBonusRuleFor(model.HwCurrencyUSD),
		HwBonusRuleFor(model.HwCurrencyEUR),
	}
}

// hwTopUpBonus returns the bonus for a list value in whole currency units: the
// credit in quota units and the same value in minor units of the currency (the
// number the receipt and the history show). Both are zero below the threshold.
// The threshold is checked on the list value, the amount the customer chose,
// so a first-purchase discount does not take the bonus away.
func hwTopUpBonus(currency string, listAmount int, rate decimal.Decimal) (quota int, minor int64, err error) {
	rule := HwBonusRuleFor(currency)
	if rule.Amount <= 0 || listAmount < rule.Min || !rate.IsPositive() {
		return 0, 0, nil
	}
	value := decimal.NewFromInt(int64(rule.Amount))
	quota, err = common.WalletQuotaFromDecimalStrict(value.Div(rate).Mul(decimal.NewFromFloat(common.QuotaPerUnit)))
	if err != nil || quota <= 0 {
		return 0, 0, err
	}
	return quota, int64(rule.Amount) * 100, nil
}

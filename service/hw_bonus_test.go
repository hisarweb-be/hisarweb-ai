package service

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func hwBonusTestEnable(t *testing.T) {
	t.Helper()
	hwSetOption(t, model.HwBonusTryMinOption, "500")
	hwSetOption(t, model.HwBonusTryAmountOption, "50")
	hwSetOption(t, model.HwBonusUsdMinOption, "10")
	hwSetOption(t, model.HwBonusUsdAmountOption, "1")
	hwSetOption(t, model.HwBonusEurMinOption, "10")
	hwSetOption(t, model.HwBonusEurAmountOption, "1")
}

func hwBonusTestOrder(t *testing.T, merchantRef string) model.HwOrder {
	t.Helper()
	var order model.HwOrder
	require.NoError(t, model.DB.Where("merchant_ref = ?", merchantRef).First(&order).Error)
	return order
}

func hwBonusTestPackages(t *testing.T, merchantRef string) (purchase, bonus []model.HwCreditPackage) {
	t.Helper()
	require.NoError(t, model.DB.Where("source = ? AND order_ref = ?", model.HwSourcePurchase, merchantRef).Find(&purchase).Error)
	require.NoError(t, model.DB.Where("source = ? AND order_ref = ?", model.HwSourceBonus, merchantRef).Find(&bonus).Error)
	return purchase, bonus
}

func TestHwTopUpBonusThresholds(t *testing.T) {
	hwOrderTestSetup(t)
	hwBonusTestEnable(t)
	rate := decimal.NewFromInt(40)
	cases := []struct {
		currency  string
		amount    int
		wantMinor int64
	}{
		{model.HwCurrencyTRY, 500, 5000},
		{model.HwCurrencyTRY, 499, 0},
		{model.HwCurrencyTRY, 20000, 5000},
		{model.HwCurrencyUSD, 10, 100},
		{model.HwCurrencyUSD, 9, 0},
		{model.HwCurrencyEUR, 10, 100},
		{model.HwCurrencyEUR, 9, 0},
	}
	for _, tc := range cases {
		quota, minor, err := hwTopUpBonus(tc.currency, tc.amount, rate)
		require.NoError(t, err, "%s %d", tc.currency, tc.amount)
		assert.Equal(t, tc.wantMinor, minor, "%s %d", tc.currency, tc.amount)
		assert.Equal(t, tc.wantMinor > 0, quota > 0, "%s %d", tc.currency, tc.amount)
	}
	// 50 TRY at 40 TRY per dollar is $1.25 of quota.
	quota, _, err := hwTopUpBonus(model.HwCurrencyTRY, 500, rate)
	require.NoError(t, err)
	assert.Equal(t, common.QuotaRound(1.25*common.QuotaPerUnit), quota)
}

func TestHwTopUpBonusFollowsTheSettings(t *testing.T) {
	hwOrderTestSetup(t)
	hwBonusTestEnable(t)
	rate := decimal.NewFromInt(40)

	hwSetOption(t, model.HwBonusTryMinOption, "1000")
	_, minor, err := hwTopUpBonus(model.HwCurrencyTRY, 500, rate)
	require.NoError(t, err)
	assert.Zero(t, minor, "a raised threshold takes the bonus away")

	hwSetOption(t, model.HwBonusTryMinOption, "500")
	hwSetOption(t, model.HwBonusTryAmountOption, "0")
	_, minor, err = hwTopUpBonus(model.HwCurrencyTRY, 500, rate)
	require.NoError(t, err)
	assert.Zero(t, minor, "an amount of zero switches the currency off")

	hwSetOption(t, model.HwBonusTryAmountOption, "not a number")
	_, minor, err = hwTopUpBonus(model.HwCurrencyTRY, 500, rate)
	require.NoError(t, err)
	assert.Zero(t, minor, "an unreadable value switches the bonus off instead of guessing")

	assert.Equal(t, HwBonusRule{Currency: model.HwCurrencyTRY}, HwBonusRuleFor("try"))
	assert.Equal(t, HwBonusRule{Currency: "GBP"}, HwBonusRuleFor("GBP"))
}

func TestHwTopUpBonusIsBookedOncePerOrder(t *testing.T) {
	fake := hwOrderTestSetup(t)
	hwBonusTestEnable(t)
	now := time.Now()
	buyer := hwOrderTestUser(t, 0)
	hwOrderTestIdentify(t, buyer.Id, "bonus-buyer")

	order := hwOrderTestCreate(t, buyer.Id, 500)
	stored := hwBonusTestOrder(t, order.MerchantRef)
	assert.EqualValues(t, 5000, stored.BonusMinor)
	require.Positive(t, stored.BonusQuota)

	// The settings change after the order exists: the order keeps its rule.
	hwSetOption(t, model.HwBonusTryAmountOption, "0")
	require.Equal(t, HwWebhookAccepted, hwOrderTestPaidWebhook(t, fake, order.MerchantRef, now))
	listQuota := hwOrderTestListQuota(t, 500)
	assert.Equal(t, listQuota+stored.BonusQuota, hwOrderTestQuota(t, buyer.Id), "500 TL is booked as 550 TL")
	assert.InDelta(t, hwOrderTestListQuota(t, 550), hwOrderTestQuota(t, buyer.Id), 1)

	purchase, bonus := hwBonusTestPackages(t, order.MerchantRef)
	require.Len(t, purchase, 1)
	require.Len(t, bonus, 1)
	assert.Equal(t, stored.BonusQuota, bonus[0].GrantedQuota)
	assert.Equal(t, purchase[0].ExpiresAt, bonus[0].ExpiresAt, "the bonus is valid as long as the purchase")
	assert.Less(t, bonus[0].Id, purchase[0].Id, "the bonus is spent before the paid credit")

	// A repeated webhook books nothing more.
	require.Equal(t, HwWebhookAccepted, hwOrderTestPaidWebhook(t, fake, order.MerchantRef, now.Add(time.Minute)))
	assert.Equal(t, listQuota+stored.BonusQuota, hwOrderTestQuota(t, buyer.Id))
	_, bonus = hwBonusTestPackages(t, order.MerchantRef)
	assert.Len(t, bonus, 1)

	// The payment history carries the bonus.
	history, err := model.HwListUserOrders(buyer.Id, 10)
	require.NoError(t, err)
	require.Len(t, history, 1)
	assert.EqualValues(t, 5000, history[0].BonusMinor)
}

func TestHwTopUpJustUnderTheThresholdEarnsNoBonus(t *testing.T) {
	fake := hwOrderTestSetup(t)
	hwBonusTestEnable(t)
	buyer := hwOrderTestUser(t, 0)
	hwOrderTestIdentify(t, buyer.Id, "bonus-under")

	order := hwOrderTestCreate(t, buyer.Id, 499)
	stored := hwBonusTestOrder(t, order.MerchantRef)
	assert.Zero(t, stored.BonusQuota)
	assert.Zero(t, stored.BonusMinor)
	require.Equal(t, HwWebhookAccepted, hwOrderTestPaidWebhook(t, fake, order.MerchantRef, time.Now()))
	assert.Equal(t, hwOrderTestListQuota(t, 499), hwOrderTestQuota(t, buyer.Id))
	_, bonus := hwBonusTestPackages(t, order.MerchantRef)
	assert.Empty(t, bonus)
}

func TestHwAdminRefundTakesTheBonusBack(t *testing.T) {
	fake := hwOrderTestSetup(t)
	hwBonusTestEnable(t)
	now := time.Now()
	buyer := hwOrderTestUser(t, 0)
	hwOrderTestIdentify(t, buyer.Id, "bonus-refund")

	order := hwOrderTestCreate(t, buyer.Id, 500)
	stored := hwBonusTestOrder(t, order.MerchantRef)
	require.Equal(t, HwWebhookAccepted, hwOrderTestPaidWebhook(t, fake, order.MerchantRef, now))
	listQuota := hwOrderTestListQuota(t, 500)
	require.Equal(t, listQuota+stored.BonusQuota, hwOrderTestQuota(t, buyer.Id))

	// The customer spent a part: the bonus goes first, so the paid credit is
	// intact and the bonus is partly gone.
	spent := stored.BonusQuota / 2
	require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", buyer.Id).
		Update("quota", listQuota+stored.BonusQuota-spent).Error)

	result, err := HwAdminRefundOrder(order.MerchantRef, now.Add(time.Minute))
	require.NoError(t, err)
	require.True(t, result.Refunded)
	assert.Equal(t, listQuota, result.ReturnedQuota, "the paid credit is whole")
	assert.Zero(t, result.SpentQuota, "spending the bonus creates no debt")
	assert.Equal(t, stored.BonusQuota-spent, result.BonusReturnedQuota)
	assert.Zero(t, hwOrderTestQuota(t, buyer.Id), "everything the order gave is gone")
	assert.Contains(t, result.Note, "bonus")

	purchase, bonus := hwBonusTestPackages(t, order.MerchantRef)
	assert.NotZero(t, purchase[0].RetiredAt)
	assert.NotZero(t, bonus[0].RetiredAt, "the bonus package is retired with the order")

	again, err := HwAdminRefundOrder(order.MerchantRef, now.Add(2*time.Minute))
	require.NoError(t, err)
	assert.False(t, again.Refunded)
	assert.Zero(t, hwOrderTestQuota(t, buyer.Id))
}

// TestHwBonusRulesAnnounceWhatTheOrderPays proves the rules shown before the
// customer pays are the rules the order books: every payable currency in a
// fixed order, a half-configured rule counts as no bonus, and a rate that
// cannot be trusted never produces credit.
func TestHwBonusRulesAnnounceWhatTheOrderPays(t *testing.T) {
	hwOrderTestSetup(t)
	hwBonusTestEnable(t)

	assert.Equal(t, []HwBonusRule{
		{Currency: model.HwCurrencyTRY, Min: 500, Amount: 50},
		{Currency: model.HwCurrencyUSD, Min: 10, Amount: 1},
		{Currency: model.HwCurrencyEUR, Min: 10, Amount: 1},
	}, HwBonusRules())

	t.Run("a rule with only one half set is switched off", func(t *testing.T) {
		hwSetOption(t, model.HwBonusUsdMinOption, "0")
		assert.Equal(t, HwBonusRule{Currency: model.HwCurrencyUSD}, HwBonusRuleFor("USD"), "a threshold of zero would pay the bonus on everything")
		hwSetOption(t, model.HwBonusUsdMinOption, "10")
		hwSetOption(t, model.HwBonusUsdAmountOption, "-5")
		assert.Equal(t, HwBonusRule{Currency: model.HwCurrencyUSD}, HwBonusRuleFor("USD"), "a negative amount is not a bonus")
		hwSetOption(t, model.HwBonusUsdAmountOption, "1")
		assert.Equal(t, HwBonusRule{Currency: model.HwCurrencyUSD, Min: 10, Amount: 1}, HwBonusRuleFor("  usd "), "the currency is matched without case and spaces")
	})
	t.Run("a rate that cannot be trusted pays no bonus", func(t *testing.T) {
		for _, rate := range []decimal.Decimal{decimal.Zero, decimal.NewFromInt(-40)} {
			quota, minor, err := hwTopUpBonus(model.HwCurrencyTRY, 500, rate)
			require.NoError(t, err)
			assert.Zero(t, quota, rate.String())
			assert.Zero(t, minor, rate.String())
		}
	})
	t.Run("a currency without a rule pays no bonus", func(t *testing.T) {
		quota, minor, err := hwTopUpBonus("GBP", 5000, decimal.NewFromInt(40))
		require.NoError(t, err)
		assert.Zero(t, quota)
		assert.Zero(t, minor)
	})
}

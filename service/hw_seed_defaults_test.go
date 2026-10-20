package service

import (
	"fmt"
	"maps"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"

	"github.com/glebarez/sqlite"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// hwSeedTestOptionValues decodes the stored option rows, so a test can compare
// what the seeding actually wrote instead of the in-memory state it left behind.
func hwSeedTestOptionValues(t *testing.T) map[string]string {
	t.Helper()
	rows, err := model.AllOption()
	require.NoError(t, err)
	values := make(map[string]string, len(rows))
	for _, row := range rows {
		values[row.Key] = row.Value
	}
	return values
}

// setupHwSeedDefaultsTest gives a test an empty options table plus the global
// settings the seeding writes into, and restores every touched global afterward.
func setupHwSeedDefaultsTest(t *testing.T) {
	t.Helper()
	originalDB := model.DB
	originalMaster := common.IsMasterNode
	originalSystemName := common.SystemName
	originalGeneral := *operation_setting.GetGeneralSetting()
	originalClaude := *model_setting.GetClaudeSettings()
	originalModelRatio := ratio_setting.ModelRatio2JSONString()
	originalCompletionRatio := ratio_setting.CompletionRatio2JSONString()
	originalGroupRatio := ratio_setting.GroupRatio2JSONString()
	originalUsableGroups := setting.UserUsableGroups2JSONString()
	originalOptions := make(map[string]string)
	common.OptionMapRWMutex.RLock()
	maps.Copy(originalOptions, common.OptionMap)
	common.OptionMapRWMutex.RUnlock()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Option{}, &model.Channel{}, &model.Ability{}))
	model.DB = db
	common.IsMasterNode = true
	common.OptionMapRWMutex.Lock()
	common.OptionMap = map[string]string{}
	common.OptionMapRWMutex.Unlock()

	t.Cleanup(func() {
		model.DB = originalDB
		common.IsMasterNode = originalMaster
		common.SystemName = originalSystemName
		*operation_setting.GetGeneralSetting() = originalGeneral
		*model_setting.GetClaudeSettings() = originalClaude
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(originalModelRatio))
		require.NoError(t, ratio_setting.UpdateCompletionRatioByJSONString(originalCompletionRatio))
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(originalGroupRatio))
		require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(originalUsableGroups))
		common.OptionMapRWMutex.Lock()
		common.OptionMap = originalOptions
		common.OptionMapRWMutex.Unlock()
		sqlDB, err := db.DB()
		if err == nil {
			require.NoError(t, sqlDB.Close())
		}
	})
}

// TestHwSeedDefaultOptions protects the production defaults a fresh database
// needs: without them every request fails with a model price error and the
// console shows dollars.
func TestHwSeedDefaultOptions(t *testing.T) {
	t.Run("a fresh database gets the sale groups, the list prices and the TL display", func(t *testing.T) {
		setupHwSeedDefaultsTest(t)
		common.OptionMapRWMutex.Lock()
		common.OptionMap[HwOptionTryPerUsd] = "40"
		common.OptionMapRWMutex.Unlock()

		require.NoError(t, HwSeedDefaultOptions())

		// The sale groups exist at the official price, next to the shipped ones.
		for _, group := range []string{"default", "vip", model.HwFreeGroupName, model.HwPaidGroupName} {
			assert.Equal(t, 1.0, ratio_setting.GetGroupRatio(group), "group ratio of %s", group)
		}
		// The console labels the sale groups; the shipped labels stay.
		assert.Equal(t, "Ücretsiz deneme", setting.GetUsableGroupDescription(model.HwFreeGroupName))
		assert.Equal(t, "Ücretli", setting.GetUsableGroupDescription(model.HwPaidGroupName))
		assert.Equal(t, "Default group", setting.GetUsableGroupDescription("default"))

		// Model ratio 1 is $2 per million tokens, so the ratio is the input
		// price per million divided by two.
		expectedModelRatios := map[string]float64{
			"claude-opus-5-5":   2,
			"claude-sonnet-5-5": 1,
			"claude-sonnet-5":   1,
			"claude-haiku-4-5":  0.5,
			"gpt-5.2":           0.875,
			"gpt-5.4":           1.25,
			"gpt-5.4-mini":      0.375,
			"gemini-3.5-flash":  0.75,
		}
		for name, expected := range expectedModelRatios {
			ratio, ok, _ := ratio_setting.GetModelRatio(name)
			require.True(t, ok, "model ratio of %s", name)
			assert.Equal(t, expected, ratio, "model ratio of %s", name)
		}
		// The completion ratio is the output price in units of the input price.
		expectedCompletionRatios := map[string]float64{
			"claude-opus-5-5":   5,
			"claude-sonnet-5-5": 5,
			"claude-sonnet-5":   5,
			"claude-haiku-4-5":  5,
			"gpt-5.2":           8,
			"gpt-5.4":           6,
			"gpt-5.4-mini":      6,
			"gemini-3.5-flash":  6,
		}
		for name, expected := range expectedCompletionRatios {
			assert.Equal(t, expected, ratio_setting.GetCompletionRatio(name), "completion ratio of %s", name)
		}

		// The thinking adapter would rewrite the model alias on its way to the
		// supplier, where that name does not exist.
		assert.False(t, model_setting.GetClaudeSettings().ThinkingAdapterEnabled)

		// The console shows TL through the same getters /api/status answers from.
		assert.Equal(t, operation_setting.QuotaDisplayTypeCustom, operation_setting.GetQuotaDisplayType())
		assert.Equal(t, HwTryCurrencySymbol, operation_setting.GetCurrencySymbol())
		assert.Equal(t, 40.0, operation_setting.GetGeneralSetting().CustomCurrencyExchangeRate)
		assert.Equal(t, HwTryCurrencySymbol, operation_setting.GetGeneralSetting().CustomCurrencySymbol)
	})

	t.Run("the TL display is set even before the first rate is published", func(t *testing.T) {
		setupHwSeedDefaultsTest(t)

		require.NoError(t, HwSeedDefaultOptions())

		assert.Equal(t, operation_setting.QuotaDisplayTypeCustom, operation_setting.GetQuotaDisplayType())
		assert.Equal(t, 1.0, operation_setting.GetGeneralSetting().CustomCurrencyExchangeRate,
			"the shipped rate stays until the daily job publishes one")
	})

	t.Run("operator values stay and a missing sold name is filled in", func(t *testing.T) {
		setupHwSeedDefaultsTest(t)
		// The operator priced one sold model itself, changed the free group and
		// chose the dollar display.
		require.NoError(t, model.UpdateOption(hwModelRatioOption, `{"claude-opus-5-5":3.5,"gpt-4o":1.25}`))
		require.NoError(t, model.UpdateOption(hwCompletionRatioOption, `{"claude-opus-5-5":4}`))
		require.NoError(t, model.UpdateOption(hwGroupRatioOption, `{"default":1,"vip":1,"free":0.9}`))
		require.NoError(t, model.UpdateOption(hwDisplayTypeOption, operation_setting.QuotaDisplayTypeUSD))

		require.NoError(t, HwSeedDefaultOptions())

		ratio, ok, _ := ratio_setting.GetModelRatio("claude-opus-5-5")
		require.True(t, ok)
		assert.Equal(t, 3.5, ratio, "an operator price is never replaced")
		assert.Equal(t, 4.0, ratio_setting.GetCompletionRatio("claude-opus-5-5"))
		ratio, ok, _ = ratio_setting.GetModelRatio("gpt-5.4")
		require.True(t, ok, "a missing sold model is filled in")
		assert.Equal(t, 1.25, ratio)
		ratio, ok, _ = ratio_setting.GetModelRatio("gpt-4o")
		require.True(t, ok, "a model outside the catalog stays")
		assert.Equal(t, 1.25, ratio)

		assert.Equal(t, 0.9, ratio_setting.GetGroupRatio(model.HwFreeGroupName))
		assert.Equal(t, 1.0, ratio_setting.GetGroupRatio(model.HwPaidGroupName))

		// A display the operator chose is left alone, down to the symbol.
		assert.Equal(t, operation_setting.QuotaDisplayTypeUSD, operation_setting.GetQuotaDisplayType())
		assert.Equal(t, "¤", operation_setting.GetGeneralSetting().CustomCurrencySymbol)
	})

	t.Run("an operator currency without a display type is kept", func(t *testing.T) {
		setupHwSeedDefaultsTest(t)
		require.NoError(t, model.UpdateOption(hwDisplaySymbolOption, "kr"))

		require.NoError(t, HwSeedDefaultOptions())

		assert.Equal(t, "kr", operation_setting.GetGeneralSetting().CustomCurrencySymbol)
		assert.Equal(t, operation_setting.QuotaDisplayTypeUSD, operation_setting.GetQuotaDisplayType(),
			"our TL display is not forced onto another currency")
	})

	t.Run("a second run writes the same values", func(t *testing.T) {
		setupHwSeedDefaultsTest(t)
		common.OptionMapRWMutex.Lock()
		common.OptionMap[HwOptionTryPerUsd] = "40"
		common.OptionMapRWMutex.Unlock()

		require.NoError(t, HwSeedDefaultOptions())
		first := hwSeedTestOptionValues(t)
		require.NoError(t, HwSeedDefaultOptions())
		second := hwSeedTestOptionValues(t)

		require.Len(t, second, len(first))
		for key, value := range first {
			require.Contains(t, second, key)
			// The ratio maps are re-encoded, so compare them decoded.
			if key == hwModelRatioOption || key == hwCompletionRatioOption || key == hwGroupRatioOption {
				assert.JSONEq(t, value, second[key], "option %s", key)
				continue
			}
			assert.Equal(t, value, second[key], "option %s", key)
		}
	})
}

// TestHwApplySnapshotModelRatios protects the link between the daily price list
// and the bill: the sold models are billed at the day's list price.
func TestHwApplySnapshotModelRatios(t *testing.T) {
	setupHwSeedDefaultsTest(t)
	// An operator priced two sold models itself and one model outside the catalog.
	require.NoError(t, model.UpdateOption(hwModelRatioOption,
		`{"claude-opus-5-5":3.5,"claude-sonnet-5":1,"gemini-3.5-flash":0.75,"gpt-4o":1.25}`))
	require.NoError(t, model.UpdateOption(hwCompletionRatioOption,
		`{"claude-opus-5-5":4,"claude-sonnet-5":5,"gemini-3.5-flash":6}`))

	snapshot := &model.HwPriceSnapshot{
		Models: `{"claude-opus-5-5":{"in":6000000,"out":30000000},` +
			`"gpt-5.2":{"in":2000000,"out":10000000},` +
			`"gemini-3.5-flash":{"in":0,"out":9000000}}`,
	}
	require.NoError(t, HwApplySnapshotModelRatios(snapshot))

	// A sold model is billed at the published list price.
	ratio, ok, _ := ratio_setting.GetModelRatio("claude-opus-5-5")
	require.True(t, ok)
	assert.Equal(t, 3.0, ratio)
	assert.Equal(t, 5.0, ratio_setting.GetCompletionRatio("claude-opus-5-5"))
	// A sold model the snapshot prices for the first time gets both ratios.
	ratio, ok, _ = ratio_setting.GetModelRatio("gpt-5.2")
	require.True(t, ok)
	assert.Equal(t, 1.0, ratio)
	assert.Equal(t, 5.0, ratio_setting.GetCompletionRatio("gpt-5.2"))
	// A model outside the catalog keeps its operator ratio.
	ratio, ok, _ = ratio_setting.GetModelRatio("gpt-4o")
	require.True(t, ok)
	assert.Equal(t, 1.25, ratio)
	// A sold model the snapshot does not price keeps its entry.
	ratio, ok, _ = ratio_setting.GetModelRatio("claude-sonnet-5")
	require.True(t, ok)
	assert.Equal(t, 1.0, ratio)
	assert.Equal(t, 5.0, ratio_setting.GetCompletionRatio("claude-sonnet-5"))
	// An unusable price never lowers the bill of a sold model to zero.
	ratio, ok, _ = ratio_setting.GetModelRatio("gemini-3.5-flash")
	require.True(t, ok)
	assert.Equal(t, 0.75, ratio)
	assert.Equal(t, 6.0, ratio_setting.GetCompletionRatio("gemini-3.5-flash"))
}

// TestHwSyncDisplayCurrencyRate protects the TL display against a stale rate:
// the first successful price fetch fills in the rate.
func TestHwSyncDisplayCurrencyRate(t *testing.T) {
	setupHwSeedDefaultsTest(t)

	require.NoError(t, hwSyncDisplayCurrencyRate(decimal.NewFromInt(41)))

	assert.Equal(t, operation_setting.QuotaDisplayTypeUSD, operation_setting.GetQuotaDisplayType(),
		"a dollar display is left alone")
	assert.Equal(t, 1.0, operation_setting.GetGeneralSetting().CustomCurrencyExchangeRate)

	require.NoError(t, model.UpdateOption(hwDisplayTypeOption, operation_setting.QuotaDisplayTypeCustom))
	require.NoError(t, model.UpdateOption(hwDisplaySymbolOption, HwTryCurrencySymbol))
	require.NoError(t, hwSyncDisplayCurrencyRate(decimal.NewFromInt(41)))

	assert.Equal(t, 41.0, operation_setting.GetGeneralSetting().CustomCurrencyExchangeRate,
		"a TL display follows the daily rate")

	require.NoError(t, model.UpdateOption(hwDisplaySymbolOption, "kr"))
	require.NoError(t, hwSyncDisplayCurrencyRate(decimal.NewFromInt(42)))

	assert.Equal(t, 41.0, operation_setting.GetGeneralSetting().CustomCurrencyExchangeRate,
		"a display the operator set to another currency is left alone")
}

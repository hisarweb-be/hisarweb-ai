package service

// Production defaults for a fresh database. The gateway works out of the box
// only when the sale groups, the billing ratios of the sold models, the TL
// display and the site identity are in place; an empty options table has none of
// them, so every request failed with "model price error" and the console showed
// dollars. This seeding runs once at startup, after the migrations and after the
// options are loaded, and it is idempotent: a second run changes nothing.
//
// It never overwrites an operator value. An entry is only added when the option
// row does not exist yet (nobody ever saved it) or when the value is empty; when
// it exists, the stored value wins even if it differs from our default. The one
// deliberate exception is the daily price refresh, which rewrites the ratios of
// the sold models from the published list price — that is billing, not seeding,
// and it lives in HwApplySnapshotModelRatios.

import (
	"errors"
	"maps"
	"slices"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"

	"github.com/shopspring/decimal"
)

const (
	// Option keys this file seeds. The general_setting and claude prefixes are
	// the config-module prefixes the option table uses for those settings.
	hwDisplayTypeOption       = "general_setting.quota_display_type"
	hwDisplaySymbolOption     = "general_setting.custom_currency_symbol"
	hwDisplayRateOption       = "general_setting.custom_currency_exchange_rate"
	hwDocsLinkOption          = "general_setting.docs_link"
	hwSystemNameOption        = "SystemName"
	hwThinkingAdapterOption   = "claude.thinking_adapter_enabled"
	hwModelRatioOption        = "ModelRatio"
	hwCompletionRatioOption   = "CompletionRatio"
	hwGroupRatioOption        = "GroupRatio"
	hwUserUsableGroupsOption  = "UserUsableGroups"
	hwCurrencySymbolPlacehold = "¤"

	// hwDefaultDocsLink is our own documentation page. The upstream default of
	// this option points at the project's wiki, which is not a page for our
	// customers.
	hwDefaultDocsLink = "https://api.hisarweb.be/docs"
	// hwDefaultSystemName is the brand the console, the mails and the API answer
	// with. The upstream default of this option names the upstream project.
	hwDefaultSystemName = "HisarWeb AI"
	// hwSalesGroupRatio is the markup of a sale group: the customer pays the
	// official list price.
	hwSalesGroupRatio = 1.0
)

// hwUserUsableGroupDescriptions are the sale groups the signup and the credit
// ledgers put customers in, with the label the console shows them.
var hwUserUsableGroupDescriptions = map[string]string{
	model.HwFreeGroupName: "Ücretsiz deneme",
	model.HwPaidGroupName: "Ücretli",
}

// HwSeedDefaultOptions fills every production default that is still missing.
// Only the master node writes them; every node reads the stored rows through
// the normal option load. Steps run independently: one failing step does not
// stop the others, and the joined error is reported by the caller.
func HwSeedDefaultOptions() error {
	if !common.IsMasterNode {
		return nil
	}
	stored, err := model.AllOption()
	if err != nil {
		return err
	}
	storedValues := make(map[string]string, len(stored))
	for _, option := range stored {
		storedValues[option.Key] = option.Value
	}
	var errs []error
	for _, step := range []func(map[string]string) error{
		hwSeedGroupRatios,
		hwSeedUserUsableGroups,
		hwSeedSoldModelRatios,
		hwSeedRetiredModels,
		hwSeedThinkingAdapter,
		hwSeedDisplayCurrency,
		hwSeedSiteIdentity,
		// The public identity, in this order: the passkey name, the passkey
		// domain and the legal texts hang off the address the site answers on.
		hwSeedSiteAddress,
		hwSeedPasskeyIdentity,
		hwSeedChatApps,
		hwSeedLegalTexts,
	} {
		if stepErr := step(storedValues); stepErr != nil {
			errs = append(errs, stepErr)
		}
	}
	// hwSeedRetiredModels takes names out of the channels and the abilities, so
	// the channel cache must be rebuilt before any request is routed: a cache
	// warmed before this seed keeps sending a retired model to a channel that no
	// longer offers it and no longer has a ratio. The rebuild is unconditional
	// because it is idempotent and also publishes whatever the other steps wrote.
	model.InitChannelCache()
	return errors.Join(errs...)
}

// hwSeedGroupRatios adds the sale groups to the group ratios. A group that is
// already priced keeps its operator value.
func hwSeedGroupRatios(map[string]string) error {
	ratios := ratio_setting.GetGroupRatioCopy()
	changed := false
	for _, group := range []string{"default", "vip", model.HwFreeGroupName, model.HwPaidGroupName} {
		if _, ok := ratios[group]; ok {
			continue
		}
		ratios[group] = hwSalesGroupRatio
		changed = true
	}
	if !changed {
		return nil
	}
	return hwWriteRatioOption(hwGroupRatioOption, ratios)
}

// hwSeedUserUsableGroups adds the sale groups to the groups a user may pick. An
// existing description is kept as written.
func hwSeedUserUsableGroups(map[string]string) error {
	groups := setting.GetUserUsableGroupsCopy()
	changed := false
	for group, description := range hwUserUsableGroupDescriptions {
		if _, ok := groups[group]; ok {
			continue
		}
		groups[group] = description
		changed = true
	}
	if !changed {
		return nil
	}
	encoded, err := common.Marshal(groups)
	if err != nil {
		return err
	}
	return model.UpdateOption(hwUserUsableGroupsOption, string(encoded))
}

// hwSeedSoldModelRatios adds the billing ratios of the sold models from the
// catalog prices. An entry the operator already priced is kept; a later price
// refresh replaces it (see HwApplySnapshotModelRatios).
func hwSeedSoldModelRatios(map[string]string) error {
	return hwSeedMissingSoldModelRatios(hwSoldModelFallbackPrices())
}

// hwSeedMissingSoldModelRatios adds only the ratio entries that are missing, so
// an operator price stays in place.
func hwSeedMissingSoldModelRatios(prices map[string]model.HwModelPrice) error {
	modelRatios := ratio_setting.GetModelRatioCopy()
	completionRatios := ratio_setting.GetCompletionRatioCopy()
	modelChanged, completionChanged := false, false
	for _, sold := range hwSoldModels {
		price, ok := prices[sold.Name]
		if !ok || price.InputMicroUsd <= 0 || price.OutputMicroUsd <= 0 {
			continue
		}
		if _, ok := modelRatios[sold.Name]; !ok {
			modelRatios[sold.Name] = hwModelRatioFromInputPrice(price.InputMicroUsd)
			modelChanged = true
		}
		if _, ok := completionRatios[sold.Name]; !ok {
			completionRatios[sold.Name] = hwCompletionRatioFromPrice(price)
			completionChanged = true
		}
	}
	if modelChanged {
		if err := hwWriteRatioOption(hwModelRatioOption, modelRatios); err != nil {
			return err
		}
	}
	if completionChanged {
		if err := hwWriteRatioOption(hwCompletionRatioOption, completionRatios); err != nil {
			return err
		}
	}
	return nil
}

// hwSeedRetiredModels takes the retired models out of every channel model list,
// the abilities and the billing ratios, so the catalog, the pricing page and the
// model list no longer offer them and a request for one finds no channel. It is
// idempotent: once the names are gone, a run changes nothing.
func hwSeedRetiredModels(map[string]string) error {
	var channels []model.Channel
	if err := model.DB.Select("id", "models").Find(&channels).Error; err != nil {
		return err
	}
	for _, channel := range channels {
		models := channel.GetModels()
		kept := slices.DeleteFunc(slices.Clone(models), func(name string) bool {
			_, retired := hwRetiredModels[strings.TrimSpace(name)]
			return retired
		})
		if len(kept) == len(models) {
			continue
		}
		if err := model.DB.Model(&model.Channel{}).Where("id = ?", channel.Id).Update("models", strings.Join(kept, ",")).Error; err != nil {
			return err
		}
	}
	retired := slices.Collect(maps.Keys(hwRetiredModels))
	if err := model.DB.Where("model IN ?", retired).Delete(&model.Ability{}).Error; err != nil {
		return err
	}
	for _, option := range []struct {
		key    string
		ratios map[string]float64
	}{
		{hwModelRatioOption, ratio_setting.GetModelRatioCopy()},
		{hwCompletionRatioOption, ratio_setting.GetCompletionRatioCopy()},
	} {
		changed := false
		for _, name := range retired {
			if _, ok := option.ratios[name]; ok {
				delete(option.ratios, name)
				changed = true
			}
		}
		if !changed {
			continue
		}
		if err := hwWriteRatioOption(option.key, option.ratios); err != nil {
			return err
		}
	}
	return nil
}

// hwSeedThinkingAdapter disables the thinking adapter once. The adapter rewrites
// a request of a thinking model name for the supplier, which turns the sold
// model alias into a model the supplier does not serve; with the adapter off the
// alias reaches the supplier unchanged.
func hwSeedThinkingAdapter(stored map[string]string) error {
	if strings.TrimSpace(stored[hwThinkingAdapterOption]) != "" {
		return nil
	}
	return model.UpdateOption(hwThinkingAdapterOption, "false")
}

// hwSeedDisplayCurrency makes the console show TL: the custom display type with
// our symbol and the daily rate. The rate is only written when the daily price
// job has already published one; until then the display stays on the shipped
// rate, and the first successful fetch fills it in (hwSyncDisplayCurrencyRate).
func hwSeedDisplayCurrency(stored map[string]string) error {
	if strings.TrimSpace(stored[hwDisplayTypeOption]) != "" {
		return nil
	}
	symbol := strings.TrimSpace(operation_setting.GetGeneralSetting().CustomCurrencySymbol)
	if symbol == "" || symbol == hwCurrencySymbolPlacehold {
		if err := model.UpdateOption(hwDisplaySymbolOption, HwTryCurrencySymbol); err != nil {
			return err
		}
		symbol = HwTryCurrencySymbol
	}
	// A console the operator set to another currency keeps it; forcing the TL
	// display would show the operator's amounts under our symbol.
	if symbol != HwTryCurrencySymbol {
		return nil
	}
	if strings.TrimSpace(stored[hwDisplayRateOption]) == "" {
		if rate, err := decimal.NewFromString(strings.TrimSpace(hwOptionValue(HwOptionTryPerUsd))); err == nil && rate.IsPositive() {
			if err := model.UpdateOption(hwDisplayRateOption, rate.String()); err != nil {
				return err
			}
		}
	}
	return model.UpdateOption(hwDisplayTypeOption, "CUSTOM")
}

// hwSeedSiteIdentity sets the brand and our documentation page once. Both
// options carry the upstream project's name and wiki by default, which is not
// what a customer of this site should see.
func hwSeedSiteIdentity(stored map[string]string) error {
	if strings.TrimSpace(stored[hwSystemNameOption]) == "" {
		if err := model.UpdateOption(hwSystemNameOption, hwDefaultSystemName); err != nil {
			return err
		}
	}
	if strings.TrimSpace(stored[hwDocsLinkOption]) == "" {
		return model.UpdateOption(hwDocsLinkOption, hwDefaultDocsLink)
	}
	return nil
}

package service

// A model that leaves the catalog has to leave the channel rows too. The public
// price list (/api/pricing) and the router read the abilities table, and the
// abilities of a channel mirror its Models column; shortening hwSoldModels alone
// keeps the old name sellable. This step removes a retired name from every
// channel row and rebuilds that channel's abilities, so the catalog list and the
// price list agree again. It touches no price: the ratios of the remaining
// models stay exactly as they are.

import (
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

// hwRetiredSoldModels are the names that were sold before and are not part of
// the catalog any more. A name stays here after it left hwSoldModels: the step
// below has to be able to clean a database that still carries it.
var hwRetiredSoldModels = []string{
	"gpt-6-astra",
	"gpt-6.1-sol",
	"gpt-6-luna",
}

// hwSeedRetiredSoldModels drops every retired name from the Models column of
// every channel and rebuilds the abilities of the channels it changed. It is
// idempotent: a channel without a retired name is left untouched, so a second
// run writes nothing.
func hwSeedRetiredSoldModels(map[string]string) error {
	if len(hwRetiredSoldModels) == 0 {
		return nil
	}
	retired := make(map[string]struct{}, len(hwRetiredSoldModels))
	for _, name := range hwRetiredSoldModels {
		retired[name] = struct{}{}
	}
	var channels []*model.Channel
	if err := model.DB.Omit("key").Find(&channels).Error; err != nil {
		return err
	}
	for _, channel := range channels {
		models := channel.GetModels()
		kept := make([]string, 0, len(models))
		removed := false
		for _, name := range models {
			trimmed := strings.TrimSpace(name)
			if trimmed == "" {
				continue
			}
			if _, ok := retired[trimmed]; ok {
				removed = true
				continue
			}
			kept = append(kept, trimmed)
		}
		if !removed {
			continue
		}
		channel.Models = strings.Join(kept, ",")
		if err := model.DB.Model(&model.Channel{}).Where("id = ?", channel.Id).
			Update("models", channel.Models).Error; err != nil {
			return err
		}
		common.SysLog(common.LogText("retired models removed from channel %d: %s", channel.Id, channel.Models))
		if err := channel.UpdateAbilities(nil); err != nil {
			return err
		}
	}
	return nil
}

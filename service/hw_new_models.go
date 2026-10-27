package service

// The daily price refresh also looks for models of a sold family that the
// catalog does not sell yet (a newer Claude, Gemini, GPT or Grok) and files
// them once on the Bureau V2 board for the Modelverkenner, with the official
// list price and the price we would sell at.

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/shopspring/decimal"
)

// HwOptionReportedNewModels keeps the names already filed as a JSON list, so a
// new model is reported once, also across restarts, without a new table.
const HwOptionReportedNewModels = "HwReportedNewModels"

// hwNewModelCardMax bounds one card; the rest follows on the next day.
const hwNewModelCardMax = 40

var hwNewModelUnwiredLog sync.Once

// hwNewCatalogModel is one candidate with its official list price.
type hwNewCatalogModel struct {
	Name  string
	Price model.HwModelPrice
}

// hwCatalogFamilyVersion splits a model name into its family (the leading
// tokens without a digit, like claude-opus or gpt), its version numbers (the
// numeric tokens that follow, a dot also splits) and whether a token is a date
// or snapshot stamp (four or more digits).
func hwCatalogFamilyVersion(name string) (family string, version []int, dated bool) {
	tokens := strings.Split(name, "-")
	i := 0
	for i < len(tokens) && !strings.ContainsAny(tokens[i], "0123456789") {
		i++
	}
	family = strings.Join(tokens[:i], "-")
	inVersion := true
	for _, token := range tokens[i:] {
		for _, part := range strings.Split(token, ".") {
			number, err := strconv.Atoi(part)
			if err != nil || part == "" {
				inVersion = false
				continue
			}
			if len(part) >= 4 {
				dated = true
				inVersion = false
				continue
			}
			if inVersion {
				version = append(version, number)
			}
		}
	}
	return family, version, dated
}

// hwNewCatalogModels returns, sorted by name, the chat models of the price list
// that belong to a sold family, are at least as new as the oldest sold model
// of that family and are neither sold nor retired. Routed or regional keys
// (bedrock/, vertex_ai/, us.anthropic.), :free, dated and -latest aliases of a
// sold name and entries without a positive price are never candidates.
func hwNewCatalogModels(list map[string]hwPriceListEntry) []hwNewCatalogModel {
	oldest := make(map[string][]int)
	for _, sold := range hwSoldModels {
		family, version, _ := hwCatalogFamilyVersion(sold.Name)
		if current, ok := oldest[family]; !ok || slices.Compare(version, current) < 0 {
			oldest[family] = version
		}
	}
	var found []hwNewCatalogModel
	for key, entry := range list {
		if entry.Mode != "chat" || key != strings.ToLower(key) || strings.ContainsAny(key, "/:@") {
			continue
		}
		base := strings.TrimSuffix(key, "-latest")
		if _, sold := hwFindSoldModel(base); sold {
			continue
		}
		if _, retired := hwRetiredModels[base]; retired {
			continue
		}
		family, version, dated := hwCatalogFamilyVersion(key)
		floor, ok := oldest[family]
		if !ok || dated || len(version) == 0 || slices.Compare(version, floor) < 0 {
			continue
		}
		inMicro, inErr := hwMicroUsdPerMillion(entry.InputCostPerToken)
		outMicro, outErr := hwMicroUsdPerMillion(entry.OutputCostPerToken)
		if inErr != nil || outErr != nil || inMicro <= 0 || outMicro <= 0 {
			continue
		}
		found = append(found, hwNewCatalogModel{Name: key, Price: model.HwModelPrice{InputMicroUsd: inMicro, OutputMicroUsd: outMicro}})
	}
	sort.Slice(found, func(i, j int) bool { return found[i].Name < found[j].Name })
	return found
}

// hwReportNewModels files one card for the new models not reported before and
// remembers them only after the board took the card. Without a configured
// board it does nothing and logs that once.
func hwReportNewModels(ctx context.Context, config HwBureauCardConfig, list map[string]hwPriceListEntry) error {
	if config.BaseURL == "" || config.CompanyID == "" {
		hwNewModelUnwiredLog.Do(func() {
			common.SysLog(common.LogText("new-model card skipped: the bureau board is not configured"))
		})
		return nil
	}
	var reported []string
	if raw := strings.TrimSpace(hwOptionValue(HwOptionReportedNewModels)); raw != "" {
		if err := common.UnmarshalJsonStr(raw, &reported); err != nil {
			return fmt.Errorf("reported new models option is not valid JSON: %w", err)
		}
	}
	var fresh []hwNewCatalogModel
	for _, candidate := range hwNewCatalogModels(list) {
		if !slices.Contains(reported, candidate.Name) && len(fresh) < hwNewModelCardMax {
			fresh = append(fresh, candidate)
		}
	}
	if len(fresh) == 0 {
		return nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Automatisch gemeld door de dagelijkse prijsronde van api.hisarweb.be: de LiteLLM-prijslijst heeft %d nieuwe model(len) van een familie die we verkopen. Prijzen in USD per miljoen tokens (invoer / uitvoer); het voorstel ligt %d %% onder de officiële prijs.\n\n", len(fresh), HwListPriceDiscountPercent)
	for _, candidate := range fresh {
		in := decimal.New(candidate.Price.InputMicroUsd, -6)
		out := decimal.New(candidate.Price.OutputMicroUsd, -6)
		fmt.Fprintf(&b, "- %s: officieel $%s / $%s, voorstel HisarWeb $%s / $%s\n", candidate.Name, in.String(), out.String(), HwOurUsd(in).String(), HwOurUsd(out).String())
	}
	b.WriteString("\nBeoordeel kwaliteit en route; een opgenomen model komt in hwSoldModels, een opgevolgd model naar hwRetiredModels (beste model voor de beste prijs). Deze kaart komt één keer per model.")
	title := fmt.Sprintf("Nieuw model in een verkochte familie: %s", fresh[0].Name)
	if len(fresh) > 1 {
		title = fmt.Sprintf("Nieuwe modellen in verkochte families: %s en %d andere", fresh[0].Name, len(fresh)-1)
	}
	if _, err := hwBureauPostIssue(ctx, config, hwBureauIssueRequest{
		Title: title, Description: b.String(), Priority: "medium", Status: "todo", AssigneeAgentID: HwBureauAgentModelverkenner,
	}); err != nil {
		return err
	}
	for _, candidate := range fresh {
		reported = append(reported, candidate.Name)
	}
	value, err := common.Marshal(reported)
	if err != nil {
		return err
	}
	return model.UpdateOption(HwOptionReportedNewModels, string(value))
}

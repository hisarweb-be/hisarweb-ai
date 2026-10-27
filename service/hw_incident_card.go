package service

// One Bureau V2 card per new outage. The minute status job calls
// hwCardNewIncident after every measurement; only the transition to a new open
// incident files a card, so a running outage never produces a card per minute.

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"gorm.io/gorm"
)

// HwOptionIncidentCardStartedAt keeps the start of the last outage that got a
// card. Outages never overlap, so a later start is a new outage; the option
// survives a restart, so a running outage is not filed again after a deploy.
const HwOptionIncidentCardStartedAt = "HwIncidentCardStartedAt"

// hwIncidentCardRetryAfter spaces the attempts after a failed card, so a board
// that is down is not called every minute.
const hwIncidentCardRetryAfter = 10 * time.Minute

var (
	hwIncidentCardMu         sync.Mutex
	hwIncidentCardRetryAt    time.Time
	hwIncidentCardUnwiredLog sync.Once
)

// hwCardNewIncident files one card for the open outage when it has not been
// filed yet. Without a configured board it does nothing and logs that once.
func hwCardNewIncident(ctx context.Context, config HwBureauCardConfig, now time.Time) error {
	if config.BaseURL == "" || config.CompanyID == "" {
		hwIncidentCardUnwiredLog.Do(func() {
			common.SysLog(common.LogText("incident card skipped: the bureau board is not configured"))
		})
		return nil
	}
	hwIncidentCardMu.Lock()
	defer hwIncidentCardMu.Unlock()
	if now.Before(hwIncidentCardRetryAt) {
		return nil
	}
	var open model.HwIncident
	err := model.DB.Where("ended_at = 0").Order("id DESC").First(&open).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	carded, _ := strconv.ParseInt(strings.TrimSpace(hwOptionValue(HwOptionIncidentCardStartedAt)), 10, 64)
	if open.StartedAt <= carded {
		return nil
	}
	failing, err := hwIncidentFailingModels(open.StartedAt, now.Unix())
	if err != nil {
		return err
	}
	if _, err := hwBureauPostIssue(ctx, config, hwIncidentCard(open, failing, HwSoldModelNames())); err != nil {
		hwIncidentCardRetryAt = now.Add(hwIncidentCardRetryAfter)
		return err
	}
	hwIncidentCardRetryAt = time.Time{}
	return model.UpdateOption(HwOptionIncidentCardStartedAt, strconv.FormatInt(open.StartedAt, 10))
}

// hwIncidentFailingModels maps every model with a bad minute since the outage
// started to its reason classes. A probe that could not measure is no evidence
// and is left out, like in the outage verdict itself.
func hwIncidentFailingModels(from int64, to int64) (map[string][]string, error) {
	var samples []model.HwStatusSample
	if err := model.DB.Where("minute >= ? AND minute <= ? AND ok = ? AND reason <> ?",
		from, to, false, model.HwStatusReasonProbeUnavailable).Find(&samples).Error; err != nil {
		return nil, err
	}
	failing := make(map[string][]string)
	for _, sample := range samples {
		reasons := failing[sample.Model]
		if sample.Reason != "" && !slices.Contains(reasons, sample.Reason) {
			reasons = append(reasons, sample.Reason)
		}
		failing[sample.Model] = reasons
	}
	return failing, nil
}

// hwIncidentCard writes the card. Every sold model down (or no model named) is
// the gateway itself, the Platformbeheerder's; a part of the catalog down is a
// model or supplier route, the Modelverkenner's.
func hwIncidentCard(open model.HwIncident, failing map[string][]string, sold []string) hwBureauIssueRequest {
	down := 0
	for _, name := range sold {
		if _, ok := failing[name]; ok {
			down++
		}
	}
	started := time.Unix(open.StartedAt, 0).UTC().Format("2006-01-02 15:04")
	card := hwBureauIssueRequest{Status: "todo"}
	if down == 0 || down >= len(sold) {
		card.Title = fmt.Sprintf("Storing api.hisarweb.be: gateway plat, alle %d modellen sinds %s UTC", len(sold), started)
		card.Priority = "critical"
		card.AssigneeAgentID = HwBureauAgentPlatformbeheerder
	} else {
		card.Title = fmt.Sprintf("Storing api.hisarweb.be: %d van %d modellen sinds %s UTC", down, len(sold), started)
		card.Priority = "high"
		card.AssigneeAgentID = HwBureauAgentModelverkenner
	}
	names := make([]string, 0, len(failing))
	for name := range failing {
		names = append(names, name)
	}
	sort.Strings(names)
	var b strings.Builder
	fmt.Fprintf(&b, "Automatisch gemeld door de statusmeting van api.hisarweb.be (incident %d, start %s UTC).\n\n", open.Id, started)
	b.WriteString("Modellen met slechte minuten (redenklasse):\n")
	for _, name := range names {
		fmt.Fprintf(&b, "- %s: %s\n", name, strings.Join(failing[name], ", "))
	}
	b.WriteString("\nDe statuspagina toont de storing al; deze kaart komt één keer per storing. Herstel het kanaal of de gateway; de storing sluit vanzelf na drie goede minuten.")
	card.Description = b.String()
	return card
}

// runHwIncidentCardSafely files the card of a new outage. A failure or panic
// is logged by class only and never stops the minute job.
func runHwIncidentCardSafely(at time.Time) {
	defer func() {
		if r := recover(); r != nil {
			common.SysError(common.LogText("incident card panicked: %v", r))
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := hwCardNewIncident(ctx, HwBureauCardConfigFromEnv(), at); err != nil {
		common.SysError(common.LogText("incident card not filed (class %s); retried after the back-off", hwPricingErrorClass(err)))
	}
}

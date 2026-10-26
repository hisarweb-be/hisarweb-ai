// HisarWeb AI customer mails: the package expires in five days, and the
// balance fell below the warning level of the last package.
//
// A daily round on the master node walks the users that still hold an active
// credit package. Every mail is claimed in hw_mail_log before it is sent, on the
// unique key (user, package, kind), so a restart, a second round or a second
// process never mails the same package twice. A send that fails keeps its claim
// on purpose: the mail server may already have accepted the message, and a
// retry could deliver it twice.
package service

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/system_setting"

	"github.com/shopspring/decimal"
)

const (
	// hwCustomerMailInterval is how often the round runs. The expiry window is
	// a full day wide, so one run a day always hits it.
	hwCustomerMailInterval = 24 * time.Hour

	// hwExpiryMailDaySeconds is the age at which a package is announced: 25
	// days after the grant, five days before the 30-day package expires.
	hwExpiryMailDaySeconds = 25 * 24 * 3600
	hwExpiryMailWindow     = 24 * 3600

	// hwLowBalancePercentDefault and hwLowBalanceMinTryDefault are the warning
	// level until the operator overrides them: ten percent of the last
	// purchased package, and no fixed floor in lira.
	hwLowBalancePercentDefault = 10
	hwLowBalanceMinTryDefault  = 0

	// hwMailTopUpPath is the page that sells a new package.
	hwMailTopUpPath = "/satin-al"
)

// hwCustomerMailSend is the one way a customer mail leaves the process. Tests
// replace it with a double so no message ever reaches a real mail server.
var hwCustomerMailSend = func(subject, receiver, content string) error {
	return common.SendEmail(subject, receiver, content)
}

// HwMailRound counts one daily round. Failed counts individual mails whose send
// returned an error; the claim of such a mail stays, so it is not retried.
type HwMailRound struct {
	Sent   int
	Failed int
}

// HwRunCustomerMails sends every customer mail that is due at now. It returns
// an error only for a round-wide failure (the user list could not be read); a
// mail that fails for one user does not stop the others. Without a usable lira
// rate only the customers whose amounts are written in lira are skipped.
func HwRunCustomerMails(now time.Time) (HwMailRound, error) {
	var round HwMailRound
	if now.IsZero() {
		return round, model.ErrHwInvalidOrder
	}
	userIDs, err := model.HwActivePackageUserIDs()
	if err != nil {
		return round, err
	}
	rate, err := hwTryPerUsd(now)
	if err != nil {
		common.SysError("customer mail round has no usable lira rate; lira customers are skipped: " + err.Error())
		rate = decimal.Zero
	}
	for _, userID := range userIDs {
		sent, failed := hwRunCustomerMailsForUserSafely(userID, now.Unix(), rate)
		round.Sent += sent
		round.Failed += failed
	}
	return round, nil
}

// hwRunCustomerMailsForUserSafely sends the mails of one user and recovers a
// panic, so one broken account cannot stop the rest of the round.
func hwRunCustomerMailsForUserSafely(userID int, now int64, rate decimal.Decimal) (sent, failed int) {
	defer func() {
		if r := recover(); r != nil {
			failed++
			common.SysError(common.LogText("customer mail round panicked for user %d: %v; continuing with the next user", userID, r))
		}
	}()
	return hwRunCustomerMailsForUser(userID, now, rate)
}

func hwRunCustomerMailsForUser(userID int, now int64, rate decimal.Decimal) (int, int) {
	var user model.User
	if err := model.DB.Select("id", "quota", "email", "username", "display_name", "setting").Where("id = ?", userID).First(&user).Error; err != nil {
		common.SysError(common.LogText("customer mail round skipped user %d: account not readable", userID))
		return 0, 1
	}
	receiver := strings.TrimSpace(user.Email)
	if receiver == "" {
		return 0, 0
	}
	lang, err := hwCustomerLanguage(user)
	if err != nil {
		common.SysError(common.LogText("customer mail round skipped user %d: language not readable", userID))
		return 0, 1
	}
	views, err := model.ListUserCreditPackages(userID)
	if err != nil {
		common.SysError(common.LogText("customer mail round skipped user %d: packages not readable", userID))
		return 0, 1
	}
	name := strings.TrimSpace(user.DisplayName)
	if name == "" {
		name = strings.TrimSpace(user.Username)
	}
	link := hwMailTopUpLink()

	currency, displayRate := hwMailCurrency(userID, time.Unix(now, 0), rate)
	if !displayRate.IsPositive() {
		common.SysError(common.LogText("customer mail round skipped user %d: no rate for the mail currency", userID))
		return 0, 1
	}

	sent, failed := 0, 0
	// The expiry mail is planned per package, so a customer who holds two
	// packages gets one mail per package, each five days before it expires.
	for _, view := range views {
		age := now - view.CreatedAt
		if age < hwExpiryMailDaySeconds || age >= hwExpiryMailDaySeconds+hwExpiryMailWindow {
			continue
		}
		if view.RemainingQuota <= 0 {
			continue
		}
		key := fmt.Sprintf("pkg:%d", view.Id)
		claimed, err := model.HwClaimMail(userID, key, model.HwMailKindExpiry, now)
		if err != nil {
			failed++
			common.SysError(common.LogText("customer mail round could not claim the expiry mail of user %d", userID))
			continue
		}
		if !claimed {
			continue
		}
		subject, content := hwMailExpiryContent(lang, name,
			hwMailFormatDate(lang, view.ExpiresAt),
			hwMailFormatMoney(lang, hwMailQuotaToMoney(view.RemainingQuota, displayRate), currency),
			hwMailFormatMoney(lang, hwMailQuotaToMoney(user.Quota, displayRate), currency),
			link)
		if err := hwCustomerMailSend(subject, receiver, content); err != nil {
			failed++
			common.SysError(common.LogText("customer mail round could not send the expiry mail of user %d; the claim stays so it is not sent twice", userID))
			continue
		}
		sent++
	}

	// The low balance mail belongs to the package the customer is spending
	// from: the one that expires first among the packages that still hold
	// balance. One mail per package is enough; a new package allows a new mail.
	spending, ok := hwSpendingPackage(views)
	if !ok {
		return sent, failed
	}
	thresholdQuota, err := hwLowBalanceThresholdQuota(userID, rate)
	if err != nil {
		failed++
		common.SysError(common.LogText("customer mail round skipped the low balance mail of user %d: threshold not computable", userID))
		return sent, failed
	}
	if user.Quota >= thresholdQuota {
		return sent, failed
	}
	key := fmt.Sprintf("pkg:%d", spending.Id)
	claimed, err := model.HwClaimMail(userID, key, model.HwMailKindLowBalance, now)
	if err != nil {
		failed++
		common.SysError(common.LogText("customer mail round could not claim the low balance mail of user %d", userID))
		return sent, failed
	}
	if !claimed {
		return sent, failed
	}
	subject, content := hwMailLowBalanceContent(lang, name,
		hwMailFormatMoney(lang, hwMailQuotaToMoney(user.Quota, displayRate), currency),
		hwMailFormatMoney(lang, hwMailQuotaToMoney(thresholdQuota, displayRate), currency),
		link)
	if err := hwCustomerMailSend(subject, receiver, content); err != nil {
		failed++
		common.SysError(common.LogText("customer mail round could not send the low balance mail of user %d; the claim stays so it is not sent twice", userID))
		return sent, failed
	}
	return sent + 1, failed
}

// hwSpendingPackage is the package the derived fill says the customer spends
// from now: the one that expires first among the packages that still hold
// balance. The low balance mail is keyed by its id, so a package warns once.
func hwSpendingPackage(views []model.HwPackageView) (model.HwPackageView, bool) {
	var spending model.HwPackageView
	found := false
	for _, view := range views {
		if view.RemainingQuota <= 0 {
			continue
		}
		if !found || view.ExpiresAt < spending.ExpiresAt || (view.ExpiresAt == spending.ExpiresAt && view.Id > spending.Id) {
			spending = view
			found = true
		}
	}
	return spending, found
}

// hwCustomerLanguage is the language of the mail: the choice on the latest
// order first, then the user setting, then Turkish, the default of the site.
func hwCustomerLanguage(user model.User) (string, error) {
	last, err := model.HwLastOrderLanguage(user.Id)
	if err != nil {
		return "", err
	}
	switch last {
	case hwLanguageEnglish:
		return hwLanguageEnglish, nil
	case hwLanguageTurkish:
		return hwLanguageTurkish, nil
	}
	return hwLanguageTag(user.GetSetting().Language), nil
}

// hwLowBalanceThresholdQuota computes the warning level in quota units: the
// configured percentage of the last purchased package, raised to the configured
// minimum in whole lira when that is larger.
func hwLowBalanceThresholdQuota(userID int, rate decimal.Decimal) (int, error) {
	percent := hwIntOptionAllowZero(model.HwLowBalancePercentOption, hwLowBalancePercentDefault)
	percent = min(max(percent, 0), 100)
	minTry := hwIntOptionAllowZero(model.HwLowBalanceMinTryOption, hwLowBalanceMinTryDefault)
	lastGranted, ok, err := model.HwLastPurchasedQuota(userID)
	if err != nil {
		return 0, err
	}
	threshold := 0
	if ok && lastGranted > 0 {
		threshold = int(math.Ceil(float64(lastGranted) * float64(percent) / 100))
	}
	if minTry > 0 && common.QuotaPerUnit > 0 {
		if !rate.IsPositive() {
			return 0, ErrHwTopUpRateMissing
		}
		minQuota := int(math.Ceil(float64(minTry) / rate.InexactFloat64() * common.QuotaPerUnit))
		threshold = max(threshold, minQuota)
	}
	return threshold, nil
}

// hwMailCurrency picks the currency the amounts of a customer's mails are
// written in: the currency of the last paid order, and the lira (with the given
// lira rate) when there is none or its rate is not available.
func hwMailCurrency(userID int, now time.Time, tryRate decimal.Decimal) (string, decimal.Decimal) {
	currency, err := model.HwLastPaidOrderCurrency(userID)
	if err != nil || currency == "" || currency == model.HwCurrencyTRY {
		return model.HwCurrencyTRY, tryRate
	}
	rate, err := hwCurrencyRate(currency, now)
	if err != nil {
		return model.HwCurrencyTRY, tryRate
	}
	return currency, rate
}

// hwMailQuotaToMoney renders quota units as the value in the customer's currency
// that they paid for them, at the daily rate of that currency.
func hwMailQuotaToMoney(quota int, rate decimal.Decimal) decimal.Decimal {
	if common.QuotaPerUnit <= 0 {
		return decimal.Zero
	}
	return decimal.NewFromInt(int64(quota)).
		Div(decimal.NewFromFloat(common.QuotaPerUnit)).
		Mul(rate)
}

// hwMailTopUpLink is the absolute address of the top-up page, built from the
// site address every mail of the site uses.
func hwMailTopUpLink() string {
	base := strings.TrimSpace(hwOption("ServerAddress"))
	if base == "" {
		base = system_setting.ServerAddress
	}
	return strings.TrimRight(base, "/") + hwMailTopUpPath
}

// hwIntOptionAllowZero is hwIntOption for a setting where zero is meaningful
// (the percentage may be switched off, the lira floor may be empty).
func hwIntOptionAllowZero(key string, fallback int) int {
	value := hwOption(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 0 {
		return fallback
	}
	return parsed
}

var hwCustomerMailOnce sync.Once

// StartHwCustomerMailJob starts the daily customer mail round. It runs once per
// process and only on the master node; the claim table makes a second process
// safe as well.
func StartHwCustomerMailJob() {
	hwCustomerMailOnce.Do(func() {
		if !common.IsMasterNode {
			return
		}
		go func() {
			common.SysLog(common.LogText("customer mail job started: interval=%s", hwCustomerMailInterval))
			runHwCustomerMailOnceSafely()
			ticker := time.NewTicker(hwCustomerMailInterval)
			defer ticker.Stop()
			for range ticker.C {
				runHwCustomerMailOnceSafely()
			}
		}()
	})
}

func runHwCustomerMailOnceSafely() {
	defer func() {
		if r := recover(); r != nil {
			common.SysError(common.LogText("customer mail job panicked: %v", r))
		}
	}()
	round, err := HwRunCustomerMails(time.Now())
	if err != nil {
		common.SysError(common.LogText("customer mail round stopped before sending: %s", err.Error()))
		return
	}
	common.SysLog(common.LogText("customer mail round finished: sent=%d failed=%d", round.Sent, round.Failed))
}

package service

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	hwMailTestRate     = "40"
	hwMailTestBase     = "https://example.test"
	hwMailTestNow      = int64(1_800_000_000)
	hwMailTestDay      = 24 * 3600
	hwMailTestValidity = 30 * 24 * 3600
)

// hwMailTestMessage is one captured customer mail.
type hwMailTestMessage struct {
	Subject  string
	Receiver string
	Content  string
}

// hwMailTestCapture replaces the process-wide sender with a double, so the
// tests never touch a mail server. It returns the list the double fills.
func hwMailTestCapture(t *testing.T) *[]hwMailTestMessage {
	t.Helper()
	var mails []hwMailTestMessage
	previous := hwCustomerMailSend
	hwCustomerMailSend = func(subject, receiver, content string) error {
		mails = append(mails, hwMailTestMessage{Subject: subject, Receiver: receiver, Content: content})
		return nil
	}
	t.Cleanup(func() { hwCustomerMailSend = previous })
	return &mails
}

// hwMailTestSetup starts a test from empty ledger tables and known options.
func hwMailTestSetup(t *testing.T) {
	t.Helper()
	require.NoError(t, model.DB.AutoMigrate(&model.HwOrder{}, &model.HwCreditPackage{}, &model.HwMailLog{}, &model.HwCreditExtension{}))
	hwMailTestClean(t)
	t.Cleanup(func() { hwMailTestClean(t) })

	keys := []string{
		"ServerAddress",
		model.HwTryPerUsdOption,
		model.HwLowBalancePercentOption,
		model.HwLowBalanceMinTryOption,
	}
	common.OptionMapRWMutex.Lock()
	if common.OptionMap == nil {
		common.OptionMap = make(map[string]string)
	}
	previous := make(map[string]string, len(keys))
	for _, key := range keys {
		previous[key] = common.OptionMap[key]
	}
	common.OptionMap["ServerAddress"] = hwMailTestBase
	common.OptionMap[model.HwTryPerUsdOption] = hwMailTestRate
	delete(common.OptionMap, model.HwLowBalancePercentOption)
	delete(common.OptionMap, model.HwLowBalanceMinTryOption)
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		defer common.OptionMapRWMutex.Unlock()
		for _, key := range keys {
			if value, ok := previous[key]; ok {
				common.OptionMap[key] = value
			} else {
				delete(common.OptionMap, key)
			}
		}
	})
}

func hwMailTestClean(t *testing.T) {
	t.Helper()
	for _, table := range []string{"hw_mail_log", "hw_orders", "hw_credit_packages"} {
		require.NoError(t, model.DB.Exec("DELETE FROM "+table).Error)
	}
}

// hwMailTestQuota is the quota of a lira amount at the test rate.
func hwMailTestQuota(try int) int {
	return common.QuotaRound(float64(try) / 40 * common.QuotaPerUnit)
}

// hwMailTestUser creates a customer with the given balance.
func hwMailTestUser(t *testing.T, quota int) model.User {
	t.Helper()
	user := hwOrderTestUser(t, 0)
	require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", user.Id).Update("quota", quota).Error)
	user.Quota = quota
	return user
}

// hwMailTestPackage gives a user one purchased package of grantQuota, created
// ageSeconds before the test's now.
func hwMailTestPackage(t *testing.T, userID int, grantQuota int, createdAt int64) model.HwCreditPackage {
	t.Helper()
	pkg := model.HwCreditPackage{
		UserId:       userID,
		Source:       model.HwSourcePurchase,
		GrantedQuota: grantQuota,
		CreatedAt:    createdAt,
		ExpiresAt:    createdAt + hwMailTestValidity,
		OrderRef:     "mail-" + common.GetRandomString(10),
	}
	require.NoError(t, model.DB.Create(&pkg).Error)
	return pkg
}

func hwMailTestSetQuota(t *testing.T, userID int, quota int) {
	t.Helper()
	require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", userID).Update("quota", quota).Error)
}

// hwMailTestAssertNoSupplierName checks the rule of the site: nothing a customer
// reads names the model supplier, in any letter case.
func hwMailTestAssertNoSupplierName(t *testing.T, mails []hwMailTestMessage) {
	t.Helper()
	require.NotEmpty(t, mails)
	for _, mail := range mails {
		text := strings.ToLower(mail.Subject + "\n" + mail.Content)
		assert.NotContains(t, text, "lumia")
		assert.NotContains(t, text, "openrouter")
	}
}

func TestHwCustomerMailExpiryOnDay25Only(t *testing.T) {
	hwMailTestSetup(t)
	mails := hwMailTestCapture(t)
	quota := hwMailTestQuota(100)
	user := hwMailTestUser(t, quota)
	now := time.Unix(hwMailTestNow, 0)

	// Day 24 after the grant: not yet.
	pkg := hwMailTestPackage(t, user.Id, quota, hwMailTestNow-24*hwMailTestDay)
	round, err := HwRunCustomerMails(now)
	require.NoError(t, err)
	assert.Equal(t, 0, round.Sent)
	assert.Empty(t, *mails)

	// Day 25: the package expires in five days.
	pkg.CreatedAt = hwMailTestNow - 25*hwMailTestDay
	pkg.ExpiresAt = pkg.CreatedAt + hwMailTestValidity
	require.NoError(t, model.DB.Model(&model.HwCreditPackage{}).Where("id = ?", pkg.Id).
		UpdateColumns(map[string]any{"created_at": pkg.CreatedAt, "expires_at": pkg.ExpiresAt}).Error)

	round, err = HwRunCustomerMails(now)
	require.NoError(t, err)
	assert.Equal(t, 1, round.Sent)
	require.Len(t, *mails, 1)
	mail := (*mails)[0]
	assert.Equal(t, user.Email, mail.Receiver)
	assert.Contains(t, mail.Subject, "süresi 5 gün içinde doluyor")
	assert.Contains(t, mail.Content, "Merhaba "+user.Username+",")
	assert.Contains(t, mail.Content, "100,00 TL")
	assert.Contains(t, mail.Content, hwMailTestBase+"/satin-al")
	hwMailTestAssertNoSupplierName(t, *mails)

	// Day 26: the window has passed, no late mail.
	*mails = nil
	pkg.CreatedAt = hwMailTestNow - 26*hwMailTestDay
	require.NoError(t, model.DB.Model(&model.HwCreditPackage{}).Where("id = ?", pkg.Id).Update("created_at", pkg.CreatedAt).Error)
	round, err = HwRunCustomerMails(time.Unix(hwMailTestNow+24*hwMailTestDay, 0))
	require.NoError(t, err)
	assert.Equal(t, 0, round.Sent)
	assert.Empty(t, *mails)
}

// A missing lira rate only skips the customers whose amounts are written in
// lira; a dollar customer still gets the expiry mail of that day.
func TestHwCustomerMailMissingTryRateSkipsOnlyLiraCustomers(t *testing.T) {
	hwMailTestSetup(t)
	mails := hwMailTestCapture(t)
	quota := hwMailTestQuota(100)
	created := hwMailTestNow - 25*hwMailTestDay
	liraUser := hwMailTestUser(t, quota)
	hwMailTestPackage(t, liraUser.Id, quota, created)
	dollarUser := hwMailTestUser(t, quota)
	hwMailTestPackage(t, dollarUser.Id, quota, created)
	require.NoError(t, model.DB.Create(&model.HwOrder{
		UserId: dollarUser.Id, MerchantRef: "mail-rate-usd", ListQuota: 1, ListValueTryMinor: 2000,
		PayableTryMinor: 1000, Currency: model.HwCurrencyUSD, DiscountKind: model.HwDiscountReturning,
		Language: "en", Status: model.HwOrderStatusPaid, CreatedAt: created,
	}).Error)
	common.OptionMapRWMutex.Lock()
	delete(common.OptionMap, model.HwTryPerUsdOption)
	common.OptionMapRWMutex.Unlock()

	round, err := HwRunCustomerMails(time.Unix(hwMailTestNow, 0))
	require.NoError(t, err)
	assert.Equal(t, 1, round.Sent)
	assert.Equal(t, 1, round.Failed)
	require.Len(t, *mails, 1)
	assert.Equal(t, dollarUser.Email, (*mails)[0].Receiver)
}

func TestHwCustomerMailExpiryNotRepeatedAfterRestart(t *testing.T) {
	hwMailTestSetup(t)
	mails := hwMailTestCapture(t)
	quota := hwMailTestQuota(100)
	user := hwMailTestUser(t, quota)
	hwMailTestPackage(t, user.Id, quota, hwMailTestNow-25*hwMailTestDay)

	round, err := HwRunCustomerMails(time.Unix(hwMailTestNow, 0))
	require.NoError(t, err)
	assert.Equal(t, 1, round.Sent)

	// Two later runs stand in for a restart and a second process on the same day.
	for range 2 {
		round, err = HwRunCustomerMails(time.Unix(hwMailTestNow+3600, 0))
		require.NoError(t, err)
		assert.Equal(t, 0, round.Sent)
	}
	assert.Len(t, *mails, 1)
}

func TestHwCustomerMailLowBalanceThreshold(t *testing.T) {
	hwMailTestSetup(t)
	mails := hwMailTestCapture(t)
	granted := hwMailTestQuota(100) // ten percent is 10 TL
	user := hwMailTestUser(t, granted)
	hwMailTestPackage(t, user.Id, granted, hwMailTestNow-hwMailTestDay)

	// Above the warning level: nothing.
	hwMailTestSetQuota(t, user.Id, hwMailTestQuota(11))
	round, err := HwRunCustomerMails(time.Unix(hwMailTestNow, 0))
	require.NoError(t, err)
	assert.Equal(t, 0, round.Sent)
	require.Empty(t, *mails)

	// Below it: one mail.
	hwMailTestSetQuota(t, user.Id, hwMailTestQuota(9))
	round, err = HwRunCustomerMails(time.Unix(hwMailTestNow+60, 0))
	require.NoError(t, err)
	assert.Equal(t, 1, round.Sent)
	require.Len(t, *mails, 1)
	assert.Contains(t, (*mails)[0].Subject, "bakiyeniz azaldı")
	assert.Contains(t, (*mails)[0].Content, "Merhaba "+user.Username+",")
	assert.Contains(t, (*mails)[0].Content, "9,00 TL")
	assert.Contains(t, (*mails)[0].Content, "10,00 TL")
	assert.Contains(t, (*mails)[0].Content, hwMailTestBase+"/satin-al")
	hwMailTestAssertNoSupplierName(t, *mails)

	// After a top-up and a second drop, this package warns only once.
	hwMailTestSetQuota(t, user.Id, granted)
	hwMailTestSetQuota(t, user.Id, hwMailTestQuota(5))
	round, err = HwRunCustomerMails(time.Unix(hwMailTestNow+120, 0))
	require.NoError(t, err)
	assert.Equal(t, 0, round.Sent)
	assert.Len(t, *mails, 1)
}

func TestHwCustomerMailLowBalanceUsesConfiguredPercentAndMinimum(t *testing.T) {
	hwMailTestSetup(t)
	mails := hwMailTestCapture(t)
	granted := hwMailTestQuota(1000)

	// Fifty percent of the last package.
	common.OptionMapRWMutex.Lock()
	common.OptionMap[model.HwLowBalancePercentOption] = "50"
	common.OptionMapRWMutex.Unlock()
	user := hwMailTestUser(t, hwMailTestQuota(499))
	hwMailTestPackage(t, user.Id, granted, hwMailTestNow-hwMailTestDay)
	round, err := HwRunCustomerMails(time.Unix(hwMailTestNow, 0))
	require.NoError(t, err)
	assert.Equal(t, 1, round.Sent)
	require.Len(t, *mails, 1)
	assert.Contains(t, (*mails)[0].Content, "500,00 TL")

	// A fixed floor in lira raises the level above the percentage.
	*mails = nil
	common.OptionMapRWMutex.Lock()
	common.OptionMap[model.HwLowBalancePercentOption] = "0"
	common.OptionMap[model.HwLowBalanceMinTryOption] = "200"
	common.OptionMapRWMutex.Unlock()
	second := hwMailTestUser(t, hwMailTestQuota(150))
	hwMailTestPackage(t, second.Id, granted, hwMailTestNow-hwMailTestDay)
	round, err = HwRunCustomerMails(time.Unix(hwMailTestNow, 0))
	require.NoError(t, err)
	assert.Equal(t, 1, round.Sent)
	require.Len(t, *mails, 1)
	assert.Contains(t, (*mails)[0].Content, "200,00 TL")

	// Above the floor, nothing.
	*mails = nil
	hwMailTestSetQuota(t, second.Id, hwMailTestQuota(250))
	round, err = HwRunCustomerMails(time.Unix(hwMailTestNow, 0))
	require.NoError(t, err)
	assert.Equal(t, 0, round.Sent)
	assert.Empty(t, *mails)
}

func TestHwCustomerMailLanguageChoice(t *testing.T) {
	hwMailTestSetup(t)
	mails := hwMailTestCapture(t)
	quota := hwMailTestQuota(100)

	// The latest order wins: English at the checkout.
	englishBuyer := hwMailTestUser(t, quota)
	hwMailTestPackage(t, englishBuyer.Id, quota, hwMailTestNow-25*hwMailTestDay)
	require.NoError(t, model.DB.Create(&model.HwOrder{
		UserId:            englishBuyer.Id,
		MerchantRef:       "mail-order-en",
		ListQuota:         quota,
		ListValueTryMinor: 10000,
		PayableTryMinor:   8500,
		Currency:          model.HwCurrencyTRY,
		DiscountKind:      model.HwDiscountReturning,
		Language:          "en",
		Status:            model.HwOrderStatusPaid,
		CreatedAt:         hwMailTestNow - 25*hwMailTestDay,
	}).Error)

	// The order says Turkish even though the setting is English: the order wins.
	settingBuyer := hwMailTestUser(t, quota)
	hwMailTestPackage(t, settingBuyer.Id, quota, hwMailTestNow-25*hwMailTestDay)
	require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", settingBuyer.Id).
		Update("setting", `{"language":"en"}`).Error)
	require.NoError(t, model.DB.Create(&model.HwOrder{
		UserId:            settingBuyer.Id,
		MerchantRef:       "mail-order-tr",
		ListQuota:         quota,
		ListValueTryMinor: 10000,
		PayableTryMinor:   8500,
		Currency:          model.HwCurrencyTRY,
		DiscountKind:      model.HwDiscountReturning,
		Language:          "tr",
		Status:            model.HwOrderStatusPaid,
		CreatedAt:         hwMailTestNow - 25*hwMailTestDay,
	}).Error)

	// Without an order the user setting decides.
	englishSettingBuyer := hwMailTestUser(t, quota)
	hwMailTestPackage(t, englishSettingBuyer.Id, quota, hwMailTestNow-25*hwMailTestDay)
	require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", englishSettingBuyer.Id).
		Update("setting", `{"language":"en"}`).Error)

	// With neither, Turkish is the default of the site.
	defaultBuyer := hwMailTestUser(t, quota)
	hwMailTestPackage(t, defaultBuyer.Id, quota, hwMailTestNow-25*hwMailTestDay)

	round, err := HwRunCustomerMails(time.Unix(hwMailTestNow, 0))
	require.NoError(t, err)
	assert.Equal(t, 4, round.Sent)
	require.Len(t, *mails, 4)

	byReceiver := make(map[string]hwMailTestMessage, len(*mails))
	for _, mail := range *mails {
		byReceiver[mail.Receiver] = mail
	}
	english := byReceiver[englishBuyer.Email]
	assert.Equal(t, "HisarWeb AI: your package expires in 5 days", english.Subject)
	assert.Contains(t, english.Content, fmt.Sprintf("Hello %s,", englishBuyer.Username))

	turkishSetting := byReceiver[settingBuyer.Email]
	assert.Contains(t, turkishSetting.Subject, "süresi 5 gün içinde doluyor")
	assert.Contains(t, turkishSetting.Content, "Merhaba "+settingBuyer.Username+",")

	englishSetting := byReceiver[englishSettingBuyer.Email]
	assert.Equal(t, "HisarWeb AI: your package expires in 5 days", englishSetting.Subject)
	assert.Contains(t, englishSetting.Content, "Hello "+englishSettingBuyer.Username+",")

	defaultMail := byReceiver[defaultBuyer.Email]
	assert.Contains(t, defaultMail.Subject, "süresi 5 gün içinde doluyor")
	assert.Contains(t, defaultMail.Content, "Merhaba "+defaultBuyer.Username+",")

	// Both expiry texts are free of supplier names.
	hwMailTestAssertNoSupplierName(t, *mails)

	// The English low balance mail is the second text a customer can read.
	*mails = nil
	hwMailTestSetQuota(t, englishBuyer.Id, hwMailTestQuota(9))
	round, err = HwRunCustomerMails(time.Unix(hwMailTestNow+60, 0))
	require.NoError(t, err)
	assert.Equal(t, 1, round.Sent)
	require.Len(t, *mails, 1)
	assert.Equal(t, "HisarWeb AI: your balance is running low", (*mails)[0].Subject)
	assert.Contains(t, (*mails)[0].Content, "Hello "+englishBuyer.Username+",")

	hwMailTestAssertNoSupplierName(t, *mails)
}

// The address confirmation and the password reset leave in the name of the site
// the operator set, in Turkish unless the customer reads English. Neither mail
// may carry the name of the upstream project or of a model supplier.
func TestHwAccountMails(t *testing.T) {
	previousName := common.SystemName
	common.SystemName = "HisarWeb AI"
	t.Cleanup(func() { common.SystemName = previousName })

	t.Run("the verification mail is Turkish with the site name", func(t *testing.T) {
		subject, content := HwEmailVerificationMail("", "428193", 15)
		assert.Equal(t, "HisarWeb AI e-posta doğrulama", subject)
		assert.Contains(t, content, "Merhaba,")
		assert.Contains(t, content, "428193")
		assert.Contains(t, content, "15 dakika")
		hwMailTestAssertNoSupplierName(t, []hwMailTestMessage{{Subject: subject, Content: content}})
		assert.NotContains(t, subject+content, "New API")
		assert.NotContains(t, strings.ToLower(subject+content), "newapi")
	})

	t.Run("an english reader gets the english text", func(t *testing.T) {
		subject, content := HwEmailVerificationMail("en-US", "428193", 15)
		assert.Equal(t, "HisarWeb AI email verification", subject)
		assert.Contains(t, content, "Hello,")
		assert.Contains(t, content, "428193")
	})

	t.Run("other languages fall back to Turkish", func(t *testing.T) {
		subject, _ := HwEmailVerificationMail("zh-CN", "428193", 15)
		assert.Equal(t, "HisarWeb AI e-posta doğrulama", subject)
		assert.Equal(t, hwLanguageTurkish, HwAccountMailLanguage("zh-CN"))
		assert.Equal(t, hwLanguageEnglish, HwAccountMailLanguage("en"))
	})

	t.Run("the reset mail carries the link and the reset text", func(t *testing.T) {
		link := "https://example.test/user/reset?email=a%40b.test&token=778899"
		subject, content := HwPasswordResetMail("tr", link, 15)
		assert.Equal(t, "HisarWeb AI şifre sıfırlama", subject)
		assert.Contains(t, content, "778899")
		assert.Contains(t, content, "15 dakika")
		englishSubject, englishContent := HwPasswordResetMail("en", link, 15)
		assert.Equal(t, "HisarWeb AI password reset", englishSubject)
		assert.Contains(t, englishContent, "reset your password")
		hwMailTestAssertNoSupplierName(t, []hwMailTestMessage{
			{Subject: subject, Content: content},
			{Subject: englishSubject, Content: englishContent},
		})
	})

	t.Run("an empty site name never leaves the mail without a brand", func(t *testing.T) {
		common.SystemName = "  "
		t.Cleanup(func() { common.SystemName = "HisarWeb AI" })
		subject, _ := HwEmailVerificationMail("tr", "1", 15)
		assert.Equal(t, "HisarWeb AI e-posta doğrulama", subject)
	})
}

func TestHwMailFormatMoneyPerCurrency(t *testing.T) {
	amount := decimal.RequireFromString("1234.5")
	for _, tc := range []struct{ lang, currency, want string }{
		{"tr", "TRY", "1.234,50 TL"},
		{"en", "TRY", "1,234.50 TL"},
		{"tr", "EUR", "1.234,50 €"},
		{"en", "EUR", "€1,234.50"},
		{"tr", "USD", "1.234,50 $"},
		{"en", "USD", "$1,234.50"},
	} {
		assert.Equal(t, tc.want, hwMailFormatMoney(tc.lang, amount, tc.currency), tc.lang+" "+tc.currency)
	}
}

// The mails write amounts in the currency of the last paid order and fall back
// to the lira without an order or without a usable rate.
func TestHwMailCurrencyFollowsTheLastPaidOrder(t *testing.T) {
	hwMailTestSetup(t)
	hwCurrencyTestOptions(t, map[string]string{
		model.HwEurPerUsdOption:     "0.8",
		model.HwEurPerUsdDateOption: time.Now().UTC().Format("2006-01-02"),
	})
	tryRate := decimal.NewFromInt(40)
	buyer := hwMailTestUser(t, 0)
	currency, rate := hwMailCurrency(buyer.Id, time.Now(), tryRate)
	assert.Equal(t, model.HwCurrencyTRY, currency)
	assert.True(t, rate.Equal(tryRate))

	require.NoError(t, model.DB.Create(&model.HwOrder{
		UserId: buyer.Id, MerchantRef: "mail-currency-eur", ListQuota: 1, ListValueTryMinor: 2000,
		PayableTryMinor: 1000, Currency: model.HwCurrencyEUR, DiscountKind: model.HwDiscountReturning,
		Language: "en", Status: model.HwOrderStatusPaid, CreatedAt: time.Now().Unix(),
	}).Error)
	currency, rate = hwMailCurrency(buyer.Id, time.Now(), tryRate)
	assert.Equal(t, model.HwCurrencyEUR, currency)
	assert.Equal(t, "0.8", rate.String())

	// An EUR rate older than seven days: the lira again.
	currency, rate = hwMailCurrency(buyer.Id, time.Now().Add(9*24*time.Hour), tryRate)
	assert.Equal(t, model.HwCurrencyTRY, currency)
	assert.True(t, rate.Equal(tryRate))
}

// The payment mail says who carries the costs and that the credit is complete;
// the refund mail names the deducted costs and what goes back.
func TestHwMailPaymentAndRefundSentences(t *testing.T) {
	for _, lang := range []string{"tr", "en"} {
		subject, content := hwMailPaymentContent(lang, "Ayşe <b>", "10,00 €", "$25.00", "https://example.test/satin-al")
		assert.NotEmpty(t, subject)
		assert.Contains(t, content, "HisarWeb")
		assert.Contains(t, content, "10,00 €")
		assert.Contains(t, content, "&lt;b&gt;", "the name is escaped")
		assert.NotContains(t, content, "—")

		_, content = hwMailRefundContent(lang, "Ayşe", "10,00 €", "0,59 €", "9,41 €", false)
		assert.Contains(t, content, "0,59 €")
		assert.Contains(t, content, "9,41 €")
		_, content = hwMailRefundContent(lang, "Ayşe", "10,00 €", "10,05 €", "", true)
		assert.Contains(t, content, "10,05 €")
		assert.NotContains(t, content, "9,41")
	}
	_, english := hwMailPaymentContent("en", "Sam", "$10.00", "$20.00", "https://example.test")
	assert.Contains(t, english, "carries the payment processing and currency conversion costs")
	assert.Contains(t, english, "full credit")
	_, turkish := hwMailRefundContent("tr", "Sam", "10,00 €", "0,59 €", "9,41 €", false)
	assert.Contains(t, turkish, "iadeden düşülür")
}

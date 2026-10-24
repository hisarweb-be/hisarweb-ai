// HisarWeb AI customer mail texts, in Turkish and English.
//
// The texts are built here and nowhere else, so the rules of the site hold for
// every mail: the sender is HisarWeb AI, the greeting is "Merhaba <naam>," or
// "Hello <name>,", no supplier of a model may be named, and the tone stays
// plain (no emoji, no dash in the middle of a sentence). The amounts arrive
// already converted to the currency of the customer: the templates do no
// pricing themselves.
package service

import (
	"fmt"
	"html"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

// The two languages the site speaks. Turkish is the default; anything that is
// not English is served in Turkish (SM-10).
const (
	hwLanguageTurkish = "tr"
	hwLanguageEnglish = "en"
)

// hwMailSiteName is the sender and signer of every customer mail.
const hwMailSiteName = "HisarWeb AI"

// hwMailZone is the zone customer dates are written in. Turkey has kept UTC+3
// without daylight saving since 2016, so a fixed zone is exact and does not
// depend on the timezone data of the server.
var hwMailZone = time.FixedZone("TRT", 3*60*60)

var hwMailMonths = map[string][12]string{
	hwLanguageTurkish: {
		"Ocak", "Şubat", "Mart", "Nisan", "Mayıs", "Haziran",
		"Temmuz", "Ağustos", "Eylül", "Ekim", "Kasım", "Aralık",
	},
	hwLanguageEnglish: {
		"January", "February", "March", "April", "May", "June",
		"July", "August", "September", "October", "November", "December",
	},
}

// hwMailFormatMoney renders an amount the way the language writes numbers: two
// decimals, a thousands separator and a decimal mark of its own, and the
// currency: "TL" after a lira amount, the symbol after a EUR or USD amount in
// Turkish and before it in English.
func hwMailFormatMoney(lang string, amount decimal.Decimal, currency string) string {
	thousands, decimalMark := ".", ","
	if lang == hwLanguageEnglish {
		thousands, decimalMark = ",", "."
	}
	text := amount.Round(2).Abs().String()
	whole, fraction, _ := strings.Cut(text, ".")
	if len(fraction) < 2 {
		fraction += strings.Repeat("0", 2-len(fraction))
	}
	var grouped strings.Builder
	if amount.IsNegative() {
		grouped.WriteByte('-')
	}
	for i := range len(whole) {
		if i > 0 && (len(whole)-i)%3 == 0 {
			grouped.WriteString(thousands)
		}
		grouped.WriteByte(whole[i])
	}
	grouped.WriteString(decimalMark)
	grouped.WriteString(fraction[:2])
	symbol := ""
	switch currency {
	case "EUR":
		symbol = "€"
	case "USD":
		symbol = "$"
	default:
		grouped.WriteString(" TL")
		return grouped.String()
	}
	if lang == hwLanguageEnglish {
		return symbol + grouped.String()
	}
	return grouped.String() + " " + symbol
}

// hwMailFormatDate writes a date as the language writes it: "5 Kasım 2026" or
// "5 November 2026". The timestamp is a Unix second count in UTC.
func hwMailFormatDate(lang string, unix int64) string {
	at := time.Unix(unix, 0).In(hwMailZone)
	months := hwMailMonths[hwLanguageTurkish]
	if lang == hwLanguageEnglish {
		months = hwMailMonths[hwLanguageEnglish]
	}
	return fmt.Sprintf("%d %s %d", at.Day(), months[at.Month()-1], at.Year())
}

// hwMailExpiryContent is the "your package expires in 5 days" mail: the day it
// expires, what is left in the package, the total balance and the way to the
// top-up page. All values are already rendered; name and link are escaped here
// because both can carry characters the HTML would misread.
func hwMailExpiryContent(lang string, name string, expiresOn string, packageLeft string, totalLeft string, link string) (subject string, content string) {
	name = html.EscapeString(name)
	link = html.EscapeString(link)
	if lang == hwLanguageEnglish {
		subject = hwMailSiteName + ": your package expires in 5 days"
		content = fmt.Sprintf(
			"<p>Hello %s,</p>"+
				"<p>The credit package in your account expires on %s.</p>"+
				"<p>Left in this package: %s<br>Your total balance: %s</p>"+
				"<p>You can still use the remaining balance before that date. To buy a new package, go to "+
				"<a href=\"%s\">%s</a></p>"+
				"<p>%s</p>",
			name, expiresOn, packageLeft, totalLeft, link, link, hwMailSiteName)
		return subject, content
	}
	subject = hwMailSiteName + ": kredi paketinizin süresi 5 gün içinde doluyor"
	content = fmt.Sprintf(
		"<p>Merhaba %s,</p>"+
			"<p>Hesabınızdaki kredi paketinin süresi %s tarihinde doluyor.</p>"+
			"<p>Bu pakette kalan: %s<br>Toplam bakiyeniz: %s</p>"+
			"<p>Kalan bakiyenizi bu tarihe kadar kullanabilirsiniz. Yeni bir paket almak için "+
			"<a href=\"%s\">%s</a> adresine gidin.</p>"+
			"<p>%s</p>",
		name, expiresOn, packageLeft, totalLeft, link, link, hwMailSiteName)
	return subject, content
}

// hwMailLowBalanceContent is the low-balance mail: the current balance, the
// threshold that was crossed and the way to the top-up page.
func hwMailLowBalanceContent(lang string, name string, balance string, threshold string, link string) (subject string, content string) {
	name = html.EscapeString(name)
	link = html.EscapeString(link)
	if lang == hwLanguageEnglish {
		subject = hwMailSiteName + ": your balance is running low"
		content = fmt.Sprintf(
			"<p>Hello %s,</p>"+
				"<p>Your balance is %s. That is below the warning level of your last package, %s.</p>"+
				"<p>To keep using your models without a break, buy a new package at "+
				"<a href=\"%s\">%s</a></p>"+
				"<p>%s</p>",
			name, balance, threshold, link, link, hwMailSiteName)
		return subject, content
	}
	subject = hwMailSiteName + ": bakiyeniz azaldı"
	content = fmt.Sprintf(
		"<p>Merhaba %s,</p>"+
			"<p>Bakiyeniz %s. Bu tutar, son paketinizin uyarı eşiğinin (%s) altında.</p>"+
			"<p>Modellerinizi kesintisiz kullanmaya devam etmek için "+
			"<a href=\"%s\">%s</a> adresinden yeni bir paket alabilirsiniz.</p>"+
			"<p>%s</p>",
		name, balance, threshold, link, link, hwMailSiteName)
	return subject, content
}

// hwMailPaymentContent is the payment confirmation: what was paid, the credit
// that was added and the sentence that HisarWeb carries the processing and
// currency conversion costs, so the full credit is added. Values arrive rendered.
func hwMailPaymentContent(lang string, name string, paid string, credit string, link string) (subject string, content string) {
	name = html.EscapeString(name)
	link = html.EscapeString(link)
	if lang == hwLanguageEnglish {
		subject = hwMailSiteName + ": payment received"
		content = fmt.Sprintf(
			"<p>Hello %s,</p>"+
				"<p>We received your payment of %s. %s of credit has been added to your account.</p>"+
				"<p>HisarWeb carries the payment processing and currency conversion costs, so you get the full credit for what you paid.</p>"+
				"<p>You can see your balance at <a href=\"%s\">%s</a></p>"+
				"<p>%s</p>",
			name, paid, credit, link, link, hwMailSiteName)
		return subject, content
	}
	subject = hwMailSiteName + ": ödemeniz alındı"
	content = fmt.Sprintf(
		"<p>Merhaba %s,</p>"+
			"<p>%s tutarındaki ödemenizi aldık. Hesabınıza %s kredi eklendi.</p>"+
			"<p>Ödeme işleme ve para birimi çevirme masraflarını HisarWeb karşılar; ödediğiniz tutarın kredisinin tamamını alırsınız.</p>"+
			"<p>Bakiyenizi <a href=\"%s\">%s</a> adresinde görebilirsiniz.</p>"+
			"<p>%s</p>",
		name, paid, credit, link, link, hwMailSiteName)
	return subject, content
}

// hwMailRefundContent is the refund mail: what was paid, the costs HisarWeb
// itself paid for the payment that are deducted, and what goes back to the card.
// A refund of nothing says that no card refund follows.
func hwMailRefundContent(lang string, name string, paid string, fee string, refunded string, nothingBack bool) (subject string, content string) {
	name = html.EscapeString(name)
	if lang == hwLanguageEnglish {
		subject = hwMailSiteName + ": your refund"
		outcome := fmt.Sprintf("%s goes back to your card.", refunded)
		if nothingBack {
			outcome = "No money goes back to your card because these costs are as high as the payment."
		}
		content = fmt.Sprintf(
			"<p>Hello %s,</p>"+
				"<p>Your payment of %s has been refunded and the credit of that purchase has been taken back.</p>"+
				"<p>The payment processing and currency conversion costs that HisarWeb itself paid for this payment, %s, are deducted from the refund. %s</p>"+
				"<p>%s</p>",
			name, paid, fee, outcome, hwMailSiteName)
		return subject, content
	}
	subject = hwMailSiteName + ": iadeniz"
	outcome := fmt.Sprintf("Kartınıza %s iade edilir.", refunded)
	if nothingBack {
		outcome = "Bu masraflar ödemenin tutarına eşit olduğu için kartınıza iade yapılmaz."
	}
	content = fmt.Sprintf(
		"<p>Merhaba %s,</p>"+
			"<p>%s tutarındaki ödemeniz iade edildi ve bu alışverişin kredisi geri alındı.</p>"+
			"<p>HisarWeb'in bu ödeme için kendisinin ödediği ödeme işleme ve para birimi çevirme masrafları olan %s, iadeden düşülür. %s</p>"+
			"<p>%s</p>",
		name, paid, fee, outcome, hwMailSiteName)
	return subject, content
}

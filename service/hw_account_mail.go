// Account mails: the address confirmation of a new account and the password
// reset link. Both are built here so the rules of the site hold for them too:
// the sender is the site name the operator set, the language is Turkish unless
// the customer reads English, and no supplier of a model is ever named. Only
// the codes and the amounts arrive from the caller; the templates do no logic
// themselves.
package service

import (
	"fmt"
	"html"
	"strings"

	"github.com/QuantumNous/new-api/common"
)

// hwAccountMailBrand is the name the mails carry: the system name of the site
// (Beheer > Instellingen), with the house name as the last resort so a mail
// never leaves with an empty brand.
func hwAccountMailBrand() string {
	if name := strings.TrimSpace(common.SystemName); name != "" {
		return name
	}
	return hwMailSiteName
}

// HwAccountMailLanguage narrows the language the request or the account states
// to the two the site speaks: English for an English tag, Turkish for anything
// else.
func HwAccountMailLanguage(stated string) string {
	return hwLanguageTag(stated)
}

// HwEmailVerificationMail is the confirmation mail of a new address: the code,
// how long it is valid, and what to do when the customer did not ask for it.
func HwEmailVerificationMail(language string, code string, minutes int) (subject string, content string) {
	brand := hwAccountMailBrand()
	escapedBrand := html.EscapeString(brand)
	escapedCode := html.EscapeString(code)
	if HwAccountMailLanguage(language) == hwLanguageEnglish {
		subject = brand + " email verification"
		content = fmt.Sprintf(
			"<p>Hello, you are verifying your email address for %s.</p>"+
				"<p>Your verification code is: <strong>%s</strong></p>"+
				"<p>The verification code is valid for %d minutes. If you did not request it, please ignore this email.</p>",
			escapedBrand, escapedCode, minutes)
		return subject, content
	}
	subject = brand + " e-posta doğrulama"
	content = fmt.Sprintf(
		"<p>Merhaba, %s hesabınız için e-posta adresinizi doğruluyorsunuz.</p>"+
			"<p>Doğrulama kodunuz: <strong>%s</strong></p>"+
			"<p>Doğrulama kodu %d dakika geçerlidir. Bu isteği siz yapmadıysanız bu e-postayı yok sayabilirsiniz.</p>",
		escapedBrand, escapedCode, minutes)
	return subject, content
}

// HwPasswordResetMail is the reset mail: the link with the one-time token, how
// long it is valid, and the same address as plain text for a mail reader that
// does not follow links.
func HwPasswordResetMail(language string, link string, minutes int) (subject string, content string) {
	brand := hwAccountMailBrand()
	escapedBrand := html.EscapeString(brand)
	escapedLink := html.EscapeString(link)
	if HwAccountMailLanguage(language) == hwLanguageEnglish {
		subject = brand + " password reset"
		content = fmt.Sprintf(
			"<p>Hello, you are resetting your password for %s.</p>"+
				"<p>Click <a href='%s'>here</a> to reset your password.</p>"+
				"<p>If the link does not work, copy this address into your browser: %s</p>"+
				"<p>The link is valid for %d minutes. If you did not request it, please ignore this email.</p>"+
				"<p>%s</p>",
			escapedBrand, escapedLink, escapedLink, minutes, escapedBrand)
		return subject, content
	}
	subject = brand + " şifre sıfırlama"
	content = fmt.Sprintf(
		"<p>Merhaba, %s hesabınızın şifresini sıfırlıyorsunuz.</p>"+
			"<p>Şifrenizi sıfırlamak için <a href='%s'>buraya</a> tıklayın.</p>"+
			"<p>Bağlantı çalışmazsa bu adresi tarayıcınıza kopyalayın: %s</p>"+
			"<p>Bağlantı %d dakika geçerlidir. Bu isteği siz yapmadıysanız bu e-postayı yok sayabilirsiniz.</p>"+
			"<p>%s</p>",
		escapedBrand, escapedLink, escapedLink, minutes, escapedBrand)
	return subject, content
}

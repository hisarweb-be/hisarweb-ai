package service

import (
	"net/url"
	"strings"

	"github.com/QuantumNous/new-api/common"
)

// HwPasswordResetLink puts the e-mail address and the one-time token in the URL
// fragment. A fragment is never sent to the server, so it stays out of proxy,
// CDN and access logs and out of the Referer header (ASVS V8.3.1). The reset
// page reads it and removes it from the address bar.
func HwPasswordResetLink(serverAddress, email, token string) string {
	return strings.TrimRight(serverAddress, "/") + "/user/reset#email=" + url.QueryEscape(email) + "&token=" + url.QueryEscape(token)
}

// HwSendResetMailAsync sends the reset mail outside the request, so the answer
// time does not depend on whether a mail was sent. A failure is logged without
// the address.
func HwSendResetMailAsync(subject, email, content string) {
	if err := common.SendEmail(subject, email, content); err != nil {
		common.SysError(common.LogText("failed to send password reset email: %s", err.Error()))
	}
}

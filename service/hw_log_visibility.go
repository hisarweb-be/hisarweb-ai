package service

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

// HwPublicErrorText prepares an upstream error text for the customer-visible
// "content" column of a log row. Supplier names are removed from the public
// column; the untouched text moves to the admin-only part of the same row, so
// operators keep the detail the customer must not read.
func HwPublicErrorText(other *model.LogOther, text string) string {
	if text == "" || !common.HwHasProviderTrace(text) {
		return text
	}
	if other != nil {
		other.SetAdmin("upstream_error", text)
	}
	return common.HwReplaceProviderWords(text)
}

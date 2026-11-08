package openai

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
)

func TestHwIsOfficialOpenAI(t *testing.T) {
	cases := []struct {
		typ  int
		base string
		want bool
	}{
		{constant.ChannelTypeOpenAI, "", true},
		{constant.ChannelTypeOpenAI, "https://api.openai.com", true},
		{constant.ChannelTypeOpenAI, "https://lumiarouter.com", false},
		{constant.ChannelTypeAzure, "https://x.openai.azure.com", true},
		{constant.ChannelTypeAnthropic, "", false},
	}
	for _, c := range cases {
		info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelType: c.typ, ChannelBaseUrl: c.base}}
		if got := hwIsOfficialOpenAI(info); got != c.want {
			t.Errorf("type %d base %q: got %v want %v", c.typ, c.base, got, c.want)
		}
	}
}

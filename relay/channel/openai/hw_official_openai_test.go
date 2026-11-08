package openai

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
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
		{constant.ChannelTypeOpenAI, "https://api.openai.com.evil.example", false},
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

func TestHwDeveloperToSystem(t *testing.T) {
	msgs := []dto.Message{{Role: "developer"}, {Role: "user"}, {Role: "developer"}}
	hwDeveloperToSystem(msgs)
	if msgs[0].Role != "system" || msgs[1].Role != "user" || msgs[2].Role != "system" {
		t.Fatalf("roles: %s %s %s", msgs[0].Role, msgs[1].Role, msgs[2].Role)
	}
}

func TestHwResponsesDeveloperToSystem(t *testing.T) {
	got := string(hwResponsesDeveloperToSystem([]byte(`[{"role":"developer","content":"a"},{"role":"user","content":"b"}]`)))
	if strings.Contains(got, "developer") || strings.Count(got, `"system"`) != 1 || !strings.Contains(got, `"user"`) {
		t.Fatalf("got %s", got)
	}
	for _, in := range []string{`"hallo"`, `[{"role":"user"}]`, `not json developer`, ``} {
		if out := string(hwResponsesDeveloperToSystem([]byte(in))); out != in {
			t.Errorf("input %q changed to %q", in, out)
		}
	}
}

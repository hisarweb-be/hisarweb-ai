package openai

import (
	"encoding/json"
	"net/url"
	"strings"

	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
)

// hwApplyUpstreamRole replaces the upstream system-to-developer step. Only
// official OpenAI and Azure know the developer role. Lumia (an OpenAI-type
// channel on another base URL) answered 422 "unknown variant developer" for
// every GPT chat request with a system prompt (8 nov 2026), so there a
// developer role, also one sent by the client, becomes system.
func hwApplyUpstreamRole(info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest, useDeveloperRole bool) {
	if !hwIsOfficialOpenAI(info) {
		hwDeveloperToSystem(request.Messages)
		return
	}
	if useDeveloperRole && len(request.Messages) > 0 && request.Messages[0].Role == "system" {
		request.Messages[0].Role = "developer"
	}
}

// hwIsOfficialOpenAI reports whether the channel talks to OpenAI itself
// (default or api.openai.com base URL) or to Azure OpenAI.
func hwIsOfficialOpenAI(info *relaycommon.RelayInfo) bool {
	if info.ChannelType == constant.ChannelTypeAzure {
		return true
	}
	if info.ChannelType != constant.ChannelTypeOpenAI {
		return false
	}
	base := strings.TrimSpace(info.ChannelBaseUrl)
	if base == "" {
		return true
	}
	u, err := url.Parse(base)
	return err == nil && strings.EqualFold(u.Hostname(), "api.openai.com")
}

// hwDeveloperToSystem rewrites a developer role to system.
func hwDeveloperToSystem(messages []dto.Message) {
	for i := range messages {
		if messages[i].Role == "developer" {
			messages[i].Role = "system"
		}
	}
}

// hwResponsesDeveloperToSystem rewrites role "developer" to "system" in the
// input array of a Responses request. Same reason as hwApplyUpstreamRole: a
// non-official upstream (Lumia) answers 422 "unknown variant developer".
// A string input, invalid JSON or an input without developer items is
// returned unchanged.
func hwResponsesDeveloperToSystem(input json.RawMessage) json.RawMessage {
	if len(input) == 0 || input[0] != '[' || !strings.Contains(string(input), "developer") {
		return input
	}
	var items []map[string]json.RawMessage
	if err := json.Unmarshal(input, &items); err != nil {
		return input
	}
	changed := false
	for _, it := range items {
		if string(it["role"]) == `"developer"` {
			it["role"] = json.RawMessage(`"system"`)
			changed = true
		}
	}
	if !changed {
		return input
	}
	out, err := json.Marshal(items)
	if err != nil {
		return input
	}
	return out
}

package openai

import (
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

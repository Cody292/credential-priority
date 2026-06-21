package provider

import (
	"strings"

	"credential-priority/internal/core"
)

// Result 是 registry 对单个凭证选择 provider 策略后的安全判定。
type Result struct {
	Credential   core.Credential
	Provider     core.Provider
	StrategyName core.StrategyName
	Freshness    core.Freshness
	ProbeStatus  core.ProbeStatus
	CanPromote   core.CanPromote
}

// Registry 根据凭证 provider/type 选择首版支持的 provider 策略。
type Registry struct {
	strategies map[core.Provider]strategy
	manual     manualStrategy
}

type strategy interface {
	evaluate(core.Credential) Result
}

type probeStrategy struct {
	provider core.Provider
	nameText core.StrategyName
}

// NewRegistry 返回包含 Codex、ChatGPT 与 manual safe no-op 的默认 registry。
func NewRegistry() Registry {
	return Registry{
		strategies: map[core.Provider]strategy{
			core.ProviderCodex:   probeStrategy{provider: core.ProviderCodex, nameText: core.StrategyCodex},
			core.ProviderChatGPT: probeStrategy{provider: core.ProviderChatGPT, nameText: core.StrategyChatGPT},
			core.ProviderManual:  manualStrategy{provider: core.ProviderManual},
		},
		manual: manualStrategy{provider: core.ProviderUnknown},
	}
}

// Evaluate 解析凭证 provider/type 并返回不会误晋升未知 provider 的策略结果。
func (r Registry) Evaluate(credential core.Credential) Result {
	selected := r.resolve(credential)
	return selected.evaluate(credential)
}

func (r Registry) resolve(credential core.Credential) strategy {
	provider := detectProvider(credential)
	if selected, ok := r.strategies[provider]; ok {
		return selected
	}
	return r.manual
}

func (s probeStrategy) evaluate(credential core.Credential) Result {
	probed := credential
	if probed.Freshness == "" {
		probed.Freshness = core.FreshnessUnknown
	}
	if probed.ProbeStatus == "" {
		probed.ProbeStatus = core.ProbeStatusUnknown
	}
	return Result{
		Credential:   probed,
		Provider:     s.provider,
		StrategyName: s.nameText,
		Freshness:    probed.Freshness,
		ProbeStatus:  probed.ProbeStatus,
		CanPromote:   core.PromotionFromProbe(probed.Freshness, probed.ProbeStatus),
	}
}

func detectProvider(credential core.Credential) core.Provider {
	providerText := normalized(string(credential.Provider))
	provider := normalizeProvider(providerText)
	if providerText == "" {
		fallback := normalizeCredentialType(string(credential.Type))
		if fallback != core.ProviderUnknown {
			return fallback
		}
	}
	return provider
}

func normalizeProvider(value string) core.Provider {
	switch normalized(value) {
	case "codex":
		return core.ProviderCodex
	case "chatgpt", "chat-gpt":
		return core.ProviderChatGPT
	case "manual":
		return core.ProviderManual
	case "gemini":
		return core.ProviderGemini
	case "openai":
		return core.ProviderOpenAI
	default:
		return core.ProviderUnknown
	}
}

func normalizeCredentialType(value string) core.Provider {
	switch normalized(value) {
	case "codex":
		return core.ProviderCodex
	case "chatgpt", "chat-gpt":
		return core.ProviderChatGPT
	case "manual":
		return core.ProviderManual
	default:
		return core.ProviderUnknown
	}
}

func normalized(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

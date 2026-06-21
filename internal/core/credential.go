package core

// Provider 标识凭证来自的上游能力域。
type Provider string

const (
	// ProviderUnknown 表示未识别或首版不支持的 provider。
	ProviderUnknown Provider = "unknown"
	// ProviderManual 表示凭证只能走人工或安全 no-op 策略。
	ProviderManual Provider = "manual"
	// ProviderCodex 表示 Codex provider。
	ProviderCodex Provider = "codex"
	// ProviderChatGPT 表示 ChatGPT provider。
	ProviderChatGPT Provider = "chatgpt"
	// ProviderGemini 表示 Gemini provider，首版仅用于识别为不支持。
	ProviderGemini Provider = "gemini"
	// ProviderOpenAI 表示 OpenAI provider，首版仅用于识别为不支持。
	ProviderOpenAI Provider = "openai"
)

// CredentialType 标识宿主 auth-file 的 type 字段。
type CredentialType string

const (
	// CredentialTypeUnknown 表示未知凭证类型。
	CredentialTypeUnknown CredentialType = "unknown"
	// CredentialTypeManual 表示人工管理的凭证类型。
	CredentialTypeManual CredentialType = "manual"
	// CredentialTypeCodex 表示 Codex 凭证类型。
	CredentialTypeCodex CredentialType = "codex"
	// CredentialTypeChatGPT 表示 ChatGPT 凭证类型。
	CredentialTypeChatGPT CredentialType = "chatgpt"
	// CredentialTypeGemini 表示 Gemini 凭证类型，首版不参与排序。
	CredentialTypeGemini CredentialType = "gemini"
	// CredentialTypeOpenAI 表示 OpenAI 凭证类型，首版不参与排序。
	CredentialTypeOpenAI CredentialType = "openai"
)

// CredentialStatus 标识宿主侧凭证状态。
type CredentialStatus string

const (
	// CredentialStatusUnknown 表示未知状态。
	CredentialStatusUnknown CredentialStatus = "unknown"
	// CredentialStatusActive 表示宿主侧 active 状态。
	CredentialStatusActive CredentialStatus = "active"
	// CredentialStatusInactive 表示宿主侧 inactive 状态。
	CredentialStatusInactive CredentialStatus = "inactive"
)

// PlanType 标识探测后可用于排序的订阅计划。
type PlanType string

const (
	// PlanTypeUnknown 表示未知计划。
	PlanTypeUnknown PlanType = "unknown"
	// PlanTypeFree 表示 free 计划。
	PlanTypeFree PlanType = "free"
	// PlanTypePlus 表示 plus 计划。
	PlanTypePlus PlanType = "plus"
	// PlanTypePro 表示 pro 计划。
	PlanTypePro PlanType = "pro"
	// PlanTypeTeam 表示 team 计划。
	PlanTypeTeam PlanType = "team"
)

// Freshness 标识凭证排序依据是否来自 fresh probe。
type Freshness string

const (
	// FreshnessUnknown 表示尚无 fresh probe 证据。
	FreshnessUnknown Freshness = "unknown"
	// FreshnessFresh 表示排序依据来自本轮 fresh probe。
	FreshnessFresh Freshness = "fresh"
	// FreshnessStale 表示仅有旧证据，不能直接参与晋升。
	FreshnessStale Freshness = "stale"
)

// ProbeStatus 标识 provider 策略对凭证探测能力的判定。
type ProbeStatus string

const (
	// ProbeStatusUnknown 表示尚未执行 provider probe。
	ProbeStatusUnknown ProbeStatus = "unknown"
	// ProbeStatusReady 表示 fresh probe 已给出可用排序证据。
	ProbeStatusReady ProbeStatus = "ready"
	// ProbeStatusUnsupported 表示当前 provider 首版不支持自动探测。
	ProbeStatusUnsupported ProbeStatus = "unsupported"
)

// CanPromote 表示凭证是否允许进入自动排序晋升候选集。
type CanPromote bool

const (
	// CannotPromote 表示凭证不得被自动晋升或重排。
	CannotPromote CanPromote = false
	// CanPromoteAfterFreshProbe 表示凭证拥有 fresh probe 证据，可进入排序候选。
	CanPromoteAfterFreshProbe CanPromote = true
)

// StrategyName 标识 registry 选中的 provider 策略名称。
type StrategyName string

const (
	// StrategyManual 表示安全 no-op/manual 策略。
	StrategyManual StrategyName = "manual"
	// StrategyCodex 表示 Codex detector/strategy。
	StrategyCodex StrategyName = "codex"
	// StrategyChatGPT 表示 ChatGPT detector/strategy。
	StrategyChatGPT StrategyName = "chatgpt"
)

// Credential 是 host auth-file 的排序领域模型快照。
type Credential struct {
	Name        string
	AuthIndex   string
	Provider    Provider
	Type        CredentialType
	Status      CredentialStatus
	Disabled    bool
	Unavailable bool
	Priority    int
	Email       string
	PlanType    PlanType
	Freshness   Freshness
	ProbeStatus ProbeStatus
}

// WithProbe 返回带有探测元数据的新凭证快照，不修改原始优先级或禁用状态。
func (c Credential) WithProbe(freshness Freshness, probeStatus ProbeStatus) Credential {
	c.Freshness = freshness
	c.ProbeStatus = probeStatus
	return c
}

// PromotionFromProbe 将 fresh probe 元数据转换为是否可参与自动晋升的明确枚举。
func PromotionFromProbe(freshness Freshness, probeStatus ProbeStatus) CanPromote {
	if freshness == FreshnessFresh && probeStatus == ProbeStatusReady {
		return CanPromoteAfterFreshProbe
	}
	return CannotPromote
}

package protocol

import "context"

// Providers is the providers.* method group (API.md §7.6).
type Providers interface {
	ListProviders(ctx context.Context) ([]Provider, error)
	ConfigureProvider(ctx context.Context, in ConfigureProviderRequest) (*Provider, error)
	TestProvider(ctx context.Context, providerID string) (*ProviderTestResult, error)
}

// Models is the models.* method group.
type Models interface {
	ListModels(ctx context.Context, providerID string) ([]Model, error)
}

// Tools is the tools.* method group.
type Tools interface {
	ListTools(ctx context.Context) ([]ToolSpec, error)
	// InvokeTool runs one tool directly, outside a run (diagnostics /
	// client-driven workflows without the LLM in the loop).
	InvokeTool(ctx context.Context, in InvokeToolRequest) (any, error)
}

// Provider is one configured LLM provider (API.md §4.9). The key is
// returned masked, never reconstructable.
type Provider struct {
	ID           string `json:"id"`
	Type         string `json:"type"` // "openai" | "anthropic" | ...
	BaseURL      string `json:"baseUrl,omitempty"`
	APIKeyMasked string `json:"apiKeyMasked"` // "" = unconfigured; e.g. "sk-…fc78"
}

// ConfigureProviderRequest — providers.configure body. Provider is the
// provider id (Provider.id), e.g. "deepseek" — a meaningful slug, named to
// match the `provider` reference field elsewhere (Model.provider,
// runs.start), not "providerId".
type ConfigureProviderRequest struct {
	Provider string `json:"provider"`
	Type     string `json:"type,omitempty"`
	BaseURL  string `json:"baseUrl,omitempty"`
	APIKey   string `json:"apiKey,omitempty"`
}

// ProviderTestResult — providers.test result.
type ProviderTestResult struct {
	OK    bool         `json:"ok"`
	Error *ProblemData `json:"error,omitempty"`
}

// Model is one entry in models.list (API.md §4.9).
type Model struct {
	ID              string             `json:"id"`
	Provider        string             `json:"provider"`
	DisplayName     string             `json:"displayName,omitempty"`
	ContextWindow   int                `json:"contextWindow,omitempty"`
	MaxOutputTokens int                `json:"maxOutputTokens,omitempty"`
	Capabilities    *ModelCapabilities `json:"capabilities,omitempty"`
	Pricing         *ModelPricing      `json:"pricing,omitempty"`
}

// ModelCapabilities — per-model feature flags (API.md §4.9).
type ModelCapabilities struct {
	Reasoning  bool `json:"reasoning,omitempty"`
	Multimodal bool `json:"multimodal,omitempty"`
	ToolUse    bool `json:"toolUse,omitempty"`
}

// ModelPricing — per-million-token pricing (API.md §4.9).
type ModelPricing struct {
	InputUsdPerMillionTokens  float64 `json:"inputUsdPerMillionTokens,omitempty"`
	OutputUsdPerMillionTokens float64 `json:"outputUsdPerMillionTokens,omitempty"`
}

// InvokeToolRequest — tools.invoke body (API.md §7.6).
type InvokeToolRequest struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
	Cwd       string         `json:"cwd,omitempty"`
}

package coreapi

import "context"

// Providers is the providers.* method group.
type Providers interface {
	ListProviders(ctx context.Context) ([]Provider, error)
	TestProvider(ctx context.Context, id string) (*ProviderTestResult, error)
}

// Models is the models.* method group.
type Models interface {
	ListModels(ctx context.Context, providerID string) ([]Model, error)
}

// Tools is the tools.* method group.
type Tools interface {
	ListTools(ctx context.Context) ([]Tool, error)
}

// Provider is one configured LLM provider entry (API.md §6.6).
type Provider struct {
	ID        string `json:"id"`
	Type      string `json:"type"`              // "openai" | "anthropic" | ...
	BaseURL   string `json:"baseUrl,omitempty"` // override for self-hosted / proxy
	HasAPIKey bool   `json:"hasApiKey"`         // true iff credentials configured
}

// ProviderTestResult is the providers.test outcome.
type ProviderTestResult struct {
	OK     bool   `json:"ok"`
	Detail string `json:"detail,omitempty"`
}

// Model is one entry in models.list (API.md §6.6).
type Model struct {
	ID            string `json:"id"`
	Provider      string `json:"provider"`                // Provider.id
	ContextWindow int    `json:"contextWindow,omitempty"`
	Description   string `json:"description,omitempty"`
}

// Tool is one entry in tools.list — the JSON-Schema parameters are
// what gets shown to the model. SafetyClass is a server-side optional
// field (front end ignores unknown keys).
type Tool struct {
	Name        string     `json:"name"`
	Description string     `json:"description,omitempty"`
	Parameters  JsonSchema `json:"parameters"`
	Origin      string     `json:"origin"`                // "server" | "client" | "mcp"
	SafetyClass string     `json:"safetyClass,omitempty"` // "safe" | "write" | "exec" | "network"
}

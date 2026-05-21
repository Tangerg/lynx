// Package config loads Lyra's runtime configuration. M1 keeps the
// shape minimal — just enough to pick a model provider and its API
// key. Later milestones extend with permission mode, sandbox profile,
// MCP server list, etc.
//
// Configuration sources (later overrides earlier):
//
//  1. Built-in defaults
//  2. ~/.lyra/config.toml (when present)
//  3. Environment variables (LYRA_*)
//  4. CLI flags (resolved by cmd/lyra)
//
// M1 implements 1 + 3 only — TOML loading lands in M2.
package config

import (
	"errors"
	"os"
)

// Provider enumerates the LLM provider Lyra talks to. M1 ships the
// two we have the smoothest lynx adapters for.
type Provider string

const (
	ProviderAnthropic Provider = "anthropic"
	ProviderOpenAI    Provider = "openai"
)

// Config is the loaded runtime configuration. Constructed by [Load];
// passed verbatim into engine + service wiring.
type Config struct {
	Provider Provider
	Model    string
	APIKey   string
}

// Load resolves the configuration from defaults + environment. The
// returned config is ready to hand to engine.New.
//
// M1 lookups (in priority order):
//
//   - LYRA_PROVIDER (default "anthropic")
//   - LYRA_MODEL    (default per provider)
//   - {PROVIDER}_API_KEY  (anthropic → ANTHROPIC_API_KEY etc.)
func Load() (Config, error) {
	provider := Provider(envOr("LYRA_PROVIDER", string(ProviderAnthropic)))

	switch provider {
	case ProviderAnthropic, ProviderOpenAI:
	default:
		return Config{}, errors.New("config: unknown LYRA_PROVIDER (want anthropic|openai)")
	}

	model := envOr("LYRA_MODEL", defaultModelFor(provider))
	apiKey := os.Getenv(apiKeyEnvFor(provider))
	if apiKey == "" {
		return Config{}, errors.New("config: " + apiKeyEnvFor(provider) + " is empty")
	}

	return Config{
		Provider: provider,
		Model:    model,
		APIKey:   apiKey,
	}, nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func defaultModelFor(p Provider) string {
	switch p {
	case ProviderAnthropic:
		return "claude-3-5-haiku-20241022"
	case ProviderOpenAI:
		return "gpt-4o-mini"
	}
	return ""
}

func apiKeyEnvFor(p Provider) string {
	switch p {
	case ProviderAnthropic:
		return "ANTHROPIC_API_KEY"
	case ProviderOpenAI:
		return "OPENAI_API_KEY"
	}
	return ""
}

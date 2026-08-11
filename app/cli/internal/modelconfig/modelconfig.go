// Package modelconfig defines provider and auxiliary-model configuration as
// consumer-owned values. Secrets remain write-only and never enter Provider.
package modelconfig

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

type RoleKind string

const (
	UtilityRole   RoleKind = "utility"
	EmbeddingRole RoleKind = "embedding"
)

func (kind RoleKind) Validate() error {
	if kind != UtilityRole && kind != EmbeddingRole {
		return fmt.Errorf("model role kind %q is invalid", kind)
	}
	return nil
}

type Role struct {
	Kind     RoleKind
	Provider string
	Model    string
}

func (role Role) Validate() error {
	if err := role.Kind.Validate(); err != nil {
		return err
	}
	provider, model := strings.TrimSpace(role.Provider), strings.TrimSpace(role.Model)
	if (provider == "") != (model == "") {
		return errors.New("model role provider and model must both be set or both be empty")
	}
	return nil
}

func (role Role) Configured() bool { return role.Provider != "" && role.Model != "" }

func (role Role) Label() string {
	if !role.Configured() {
		if role.Kind == UtilityRole {
			return "inherit the run model"
		}
		return "disabled"
	}
	return role.Provider + "/" + role.Model
}

type Roles struct {
	Utility   Role
	Embedding Role
}

func (roles Roles) Validate() error {
	if roles.Utility.Kind != UtilityRole || roles.Embedding.Kind != EmbeddingRole {
		return errors.New("model roles are assigned to the wrong slots")
	}
	if err := roles.Utility.Validate(); err != nil {
		return fmt.Errorf("utility role: %w", err)
	}
	if err := roles.Embedding.Validate(); err != nil {
		return fmt.Errorf("embedding role: %w", err)
	}
	return nil
}

type KeySource string

const (
	KeyStored KeySource = "stored"
	KeyEnv    KeySource = "env"
)

type Provider struct {
	ID                    string
	BaseURL               string
	APIKeyMasked          string
	KeySource             KeySource
	RequiresBaseURL       bool
	EmbeddingCapable      bool
	DefaultEmbeddingModel string
}

func (provider Provider) Validate() error {
	if strings.TrimSpace(provider.ID) == "" {
		return errors.New("provider id is empty")
	}
	if provider.KeySource != "" && provider.KeySource != KeyStored && provider.KeySource != KeyEnv {
		return fmt.Errorf("provider %s has invalid key source %q", provider.ID, provider.KeySource)
	}
	if provider.APIKeyMasked == "" && provider.KeySource != "" {
		return fmt.Errorf("provider %s has a key source without a configured key", provider.ID)
	}
	if provider.APIKeyMasked != "" && provider.KeySource == "" {
		return fmt.Errorf("provider %s has a configured key without its source", provider.ID)
	}
	return nil
}

func (provider Provider) Configured() bool  { return provider.APIKeyMasked != "" }
func (provider Provider) KeyEditable() bool { return provider.KeySource != KeyEnv }

type ChangeKind string

const (
	SetValue   ChangeKind = "set"
	ClearValue ChangeKind = "clear"
)

type ValueChange struct {
	Kind  ChangeKind
	Value string
}

func (change ValueChange) Validate() error {
	switch change.Kind {
	case SetValue:
		if strings.TrimSpace(change.Value) == "" {
			return errors.New("set change value is empty")
		}
	case ClearValue:
		if change.Value != "" {
			return errors.New("clear change carries a value")
		}
	default:
		return fmt.Errorf("provider change kind %q is invalid", change.Kind)
	}
	return nil
}

type UpdateProvider struct {
	Provider string
	BaseURL  *ValueChange
	APIKey   *ValueChange
}

func (update UpdateProvider) Validate() error {
	if strings.TrimSpace(update.Provider) == "" {
		return errors.New("update provider id is empty")
	}
	if update.BaseURL == nil && update.APIKey == nil {
		return errors.New("update provider has no changes")
	}
	for _, field := range []struct {
		name   string
		change *ValueChange
	}{
		{name: "base URL", change: update.BaseURL},
		{name: "API key", change: update.APIKey},
	} {
		if field.change != nil {
			if err := field.change.Validate(); err != nil {
				return fmt.Errorf("update provider %s: %w", field.name, err)
			}
		}
	}
	return nil
}

type TestResult struct {
	OK      bool
	Problem string
}

func (result TestResult) Validate() error {
	if result.OK && result.Problem != "" {
		return errors.New("successful provider test carries a problem")
	}
	if !result.OK && strings.TrimSpace(result.Problem) == "" {
		return errors.New("failed provider test has no problem")
	}
	return nil
}

type Service interface {
	Roles(context.Context) (Roles, error)
	SetRole(context.Context, Role) (Role, error)
	Providers(context.Context) ([]Provider, error)
	UpdateProvider(context.Context, UpdateProvider) (Provider, error)
	TestProvider(context.Context, string) (TestResult, error)
}

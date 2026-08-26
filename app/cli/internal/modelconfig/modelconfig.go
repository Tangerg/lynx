// Package modelconfig defines provider and auxiliary-model configuration as
// consumer-owned values. Secrets remain write-only and never enter Provider.
package modelconfig

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Tangerg/lynx/app/cli/internal/failure"
)

type RoleKind string

const (
	UtilityRole   RoleKind = "utility"
	EmbeddingRole RoleKind = "embedding"
)

func (r RoleKind) Validate() error {
	if r != UtilityRole && r != EmbeddingRole {
		return fmt.Errorf("model role kind %q is invalid", r)
	}
	return nil
}

type Role struct {
	Kind     RoleKind
	Provider string
	Model    string
}

func (r Role) Validate() error {
	if err := r.Kind.Validate(); err != nil {
		return err
	}
	provider, model := strings.TrimSpace(r.Provider), strings.TrimSpace(r.Model)
	if (provider == "") != (model == "") {
		return errors.New("model role provider and model must both be set or both be empty")
	}
	return nil
}

func (r Role) Configured() bool { return r.Provider != "" && r.Model != "" }

func (r Role) Label() string {
	if !r.Configured() {
		if r.Kind == UtilityRole {
			return "inherit the run model"
		}
		return "disabled"
	}
	return r.Provider + "/" + r.Model
}

type Roles struct {
	Utility   Role
	Embedding Role
}

func (r Roles) Validate() error {
	if r.Utility.Kind != UtilityRole || r.Embedding.Kind != EmbeddingRole {
		return errors.New("model roles are assigned to the wrong slots")
	}
	if err := r.Utility.Validate(); err != nil {
		return fmt.Errorf("utility role: %w", err)
	}
	if err := r.Embedding.Validate(); err != nil {
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

func (p Provider) Validate() error {
	if strings.TrimSpace(p.ID) == "" {
		return errors.New("provider id is empty")
	}
	if p.KeySource != "" && p.KeySource != KeyStored && p.KeySource != KeyEnv {
		return fmt.Errorf("provider %s has invalid key source %q", p.ID, p.KeySource)
	}
	if p.APIKeyMasked == "" && p.KeySource != "" {
		return fmt.Errorf("provider %s has a key source without a configured key", p.ID)
	}
	if p.APIKeyMasked != "" && p.KeySource == "" {
		return fmt.Errorf("provider %s has a configured key without its source", p.ID)
	}
	return nil
}

func (p Provider) Configured() bool  { return p.APIKeyMasked != "" }
func (p Provider) KeyEditable() bool { return p.KeySource != KeyEnv }

type ChangeKind string

const (
	SetValue   ChangeKind = "set"
	ClearValue ChangeKind = "clear"
)

type ValueChange struct {
	Kind  ChangeKind
	Value string
}

func (v ValueChange) Validate() error {
	switch v.Kind {
	case SetValue:
		if strings.TrimSpace(v.Value) == "" {
			return errors.New("set change value is empty")
		}
	case ClearValue:
		if v.Value != "" {
			return errors.New("clear change carries a value")
		}
	default:
		return fmt.Errorf("provider change kind %q is invalid", v.Kind)
	}
	return nil
}

type UpdateProvider struct {
	Provider string
	BaseURL  *ValueChange
	APIKey   *ValueChange
}

func (u UpdateProvider) Validate() error {
	if strings.TrimSpace(u.Provider) == "" {
		return errors.New("update provider id is empty")
	}
	if u.BaseURL == nil && u.APIKey == nil {
		return errors.New("update provider has no changes")
	}
	for _, field := range []struct {
		name   string
		change *ValueChange
	}{
		{name: "base URL", change: u.BaseURL},
		{name: "API key", change: u.APIKey},
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
	Problem *failure.Problem
}

func (t TestResult) Validate() error {
	if t.OK == (t.Problem != nil) {
		return errors.New("provider test result must contain exactly one success or problem state")
	}
	if t.Problem != nil {
		return t.Problem.Validate()
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

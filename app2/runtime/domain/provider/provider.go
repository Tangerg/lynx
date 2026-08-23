// Package provider owns mutable credentials independently of the static model
// catalog and provider wire adapters.
package provider

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
)

var ErrInvalid = errors.New("provider: invalid configuration")

type KeySource string

const (
	KeyNone   KeySource = ""
	KeyStored KeySource = "stored"
	KeyEnv    KeySource = "env"
)

type Provider struct {
	ID        string
	BaseURL   string
	APIKey    string
	KeySource KeySource
}

func (value Provider) Validate(requiresBaseURL bool) error {
	if strings.TrimSpace(value.ID) == "" {
		return fmt.Errorf("%w: id is required", ErrInvalid)
	}
	if requiresBaseURL && strings.TrimSpace(value.BaseURL) == "" {
		return fmt.Errorf("%w: base URL is required", ErrInvalid)
	}
	if value.BaseURL != "" {
		parsed, err := url.ParseRequestURI(value.BaseURL)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
			return fmt.Errorf("%w: base URL must be HTTP(S)", ErrInvalid)
		}
	}
	return nil
}

type TextChange struct {
	Present bool
	Clear   bool
	Value   string
}

type Patch struct {
	BaseURL TextChange
	APIKey  TextChange
}

func (value Provider) Apply(patch Patch) Provider {
	if patch.BaseURL.Present {
		value.BaseURL = strings.TrimSpace(patch.BaseURL.Value)
		if patch.BaseURL.Clear {
			value.BaseURL = ""
		}
	}
	if patch.APIKey.Present {
		value.APIKey = patch.APIKey.Value
		value.KeySource = KeyStored
		if patch.APIKey.Clear {
			value.APIKey = ""
			value.KeySource = KeyNone
		}
	}
	return value
}

// Package provider owns mutable credentials independently of the static model
// catalog and provider wire adapters.
package provider

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
)

var (
	ErrInvalid          = errors.New("provider: invalid configuration")
	ErrRevisionConflict = errors.New("provider: revision conflict")
)

// Configuration is one durable provider aggregate. Credential provenance is
// deliberately absent: stored-vs-environment is an application projection,
// not mutable provider state.
type Configuration struct {
	id       string
	baseURL  string
	apiKey   string
	revision uint64
}

func New(id string) (Configuration, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return Configuration{}, fmt.Errorf("%w: id is required", ErrInvalid)
	}
	return Configuration{id: id}, nil
}

func Rehydrate(id, baseURL, apiKey string, revision uint64) (Configuration, error) {
	value := Configuration{
		id: strings.TrimSpace(id), baseURL: strings.TrimSpace(baseURL),
		apiKey: apiKey, revision: revision,
	}
	if err := value.validateIdentity(); err != nil {
		return Configuration{}, err
	}
	return value, nil
}

func (value Configuration) validateIdentity() error {
	if value.id == "" {
		return fmt.Errorf("%w: id is required", ErrInvalid)
	}
	if value.revision == 0 {
		return fmt.Errorf("%w: revision must be positive", ErrInvalid)
	}
	return nil
}

func (value Configuration) Validate(requiresBaseURL bool) error {
	if err := value.validateIdentity(); err != nil {
		return err
	}
	if requiresBaseURL && value.baseURL == "" && value.apiKey != "" {
		return fmt.Errorf("%w: base URL is required", ErrInvalid)
	}
	if value.baseURL != "" {
		parsed, err := url.ParseRequestURI(value.baseURL)
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

// Apply mutates one aggregate generation and reports whether durable state
// actually changed. A no-op does not consume a revision.
func (value *Configuration) Apply(patch Patch) bool {
	baseURL, apiKey := value.baseURL, value.apiKey
	if patch.BaseURL.Present {
		baseURL = strings.TrimSpace(patch.BaseURL.Value)
		if patch.BaseURL.Clear {
			baseURL = ""
		}
	}
	if patch.APIKey.Present {
		apiKey = patch.APIKey.Value
		if patch.APIKey.Clear {
			apiKey = ""
		}
	}
	if baseURL == value.baseURL && apiKey == value.apiKey {
		return false
	}
	value.baseURL, value.apiKey = baseURL, apiKey
	value.revision++
	return true
}

func (value Configuration) ID() string       { return value.id }
func (value Configuration) BaseURL() string  { return value.baseURL }
func (value Configuration) APIKey() string   { return value.apiKey }
func (value Configuration) Revision() uint64 { return value.revision }

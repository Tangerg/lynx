// Package modelrole defines the provider/model assignment used by specialized
// runtime roles such as maintenance and semantic embeddings.
package modelrole

import "errors"

// ErrProviderRequired reports a configured model without its provider.
var ErrProviderRequired = errors.New("model role: provider is required when model is set")

// Role is an immutable provider/model assignment. Its zero value is unset.
type Role struct {
	providerID string
	model      string
}

// New constructs a role. An empty model always produces the unset role.
func New(providerID, model string) (Role, error) {
	if model == "" {
		return Role{}, nil
	}
	if providerID == "" {
		return Role{}, ErrProviderRequired
	}
	return Role{providerID: providerID, model: model}, nil
}

// Configured reports whether the role names a model.
func (r Role) Configured() bool {
	return r.model != ""
}

// ProviderID returns the provider assigned to the role.
func (r Role) ProviderID() string {
	return r.providerID
}

// Model returns the model assigned to the role.
func (r Role) Model() string {
	return r.model
}

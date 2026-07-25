// Package modelrole defines the provider/model assignment used by specialized
// runtime roles such as maintenance and semantic embeddings.
package modelrole

import "github.com/Tangerg/lynx/app/runtime/internal/domain/modelref"

// Role is an immutable provider/model assignment. Its zero value is unset.
type Role struct {
	selection modelref.Selection
}

// New constructs a role. It shares modelref's paired-value invariant: a role
// is either unset or names both its provider and model.
func New(providerID, model string) (Role, error) {
	selection, err := modelref.New(providerID, model)
	if err != nil {
		return Role{}, err
	}
	return Role{selection: selection}, nil
}

// Configured reports whether the role names a model.
func (r Role) Configured() bool {
	return r.selection.Configured()
}

// ProviderID returns the provider assigned to the role.
func (r Role) ProviderID() string {
	return r.selection.Provider()
}

// Model returns the model assigned to the role.
func (r Role) Model() string {
	return r.selection.Model()
}

// Selection exposes this role's already-validated provider/model assignment to
// adapters that resolve a concrete client.
func (r Role) Selection() modelref.Selection {
	return r.selection
}

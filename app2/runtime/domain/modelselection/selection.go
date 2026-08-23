// Package modelselection owns the paired provider/model identity shared by
// Sessions and internal model roles. A model id is never meaningful without
// the provider namespace that issued it.
package modelselection

import (
	"errors"
	"fmt"
	"strings"
)

var ErrInvalid = errors.New("model selection: invalid value")

// Selection is either completely empty or a complete provider/model pair.
// Its fields stay private so incomplete identities cannot escape validation.
type Selection struct {
	provider string
	model    string
}

func New(provider, model string) (Selection, error) {
	value := Selection{
		provider: strings.TrimSpace(provider),
		model:    strings.TrimSpace(model),
	}
	if (value.provider == "") != (value.model == "") {
		return Selection{}, fmt.Errorf("%w: provider and model must be set together", ErrInvalid)
	}
	return value, nil
}

func (value Selection) Provider() string { return value.provider }
func (value Selection) Model() string    { return value.model }
func (value Selection) Empty() bool      { return value.provider == "" }

type Role string

const (
	RoleUtility   Role = "utility"
	RoleEmbedding Role = "embedding"
)

func (role Role) Valid() bool {
	return role == RoleUtility || role == RoleEmbedding
}

package models

import (
	"sync/atomic"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelrole"
)

// RoleState owns one live model-role assignment. Its synchronization is kept
// inside the application boundary; consumers observe the immutable value through
// Role rather than sharing an atomic implementation detail.
type RoleState struct {
	role atomic.Pointer[modelrole.Role]
}

// NewRoleState builds a live role assignment with initial as its current value.
func NewRoleState(initial modelrole.Role) *RoleState {
	state := &RoleState{}
	state.Store(initial)
	return state
}

// Role returns the current assignment. The zero value means no specialized
// model is configured.
func (s *RoleState) Role() modelrole.Role {
	if s == nil {
		return modelrole.Role{}
	}
	role := s.role.Load()
	if role == nil {
		return modelrole.Role{}
	}
	return *role
}

// Store atomically publishes the next immutable assignment.
func (s *RoleState) Store(role modelrole.Role) {
	s.role.Store(&role)
}

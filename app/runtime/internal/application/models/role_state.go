package models

import (
	"sync/atomic"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelref"
)

// RoleState owns one live model-role assignment. Its synchronization is kept
// inside the application boundary; consumers observe the immutable value through
// Role rather than sharing an atomic implementation detail.
type RoleState struct {
	role atomic.Pointer[modelref.Selection]
}

// NewRoleState builds a live role assignment with initial as its current value.
func NewRoleState(initial modelref.Selection) *RoleState {
	state := &RoleState{}
	state.Store(initial)
	return state
}

// Role returns the current assignment. The zero value means no specialized
// model is configured.
func (s *RoleState) Role() modelref.Selection {
	if s == nil {
		return modelref.Selection{}
	}
	role := s.role.Load()
	if role == nil {
		return modelref.Selection{}
	}
	return *role
}

// Store atomically publishes the next immutable assignment.
func (s *RoleState) Store(role modelref.Selection) {
	s.role.Store(&role)
}

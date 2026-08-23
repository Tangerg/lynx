package mcpserver

import (
	"errors"
	"fmt"
	"time"
)

type AuthorizationStatus string

const (
	AuthorizationPending   AuthorizationStatus = "pending"
	AuthorizationSucceeded AuthorizationStatus = "succeeded"
	AuthorizationFailed    AuthorizationStatus = "failed"
	AuthorizationCanceled  AuthorizationStatus = "canceled"
)

type AuthorizationAttemptState struct {
	ID         string              `json:"id"`
	Server     string              `json:"server"`
	Status     AuthorizationStatus `json:"status"`
	CreatedAt  time.Time           `json:"createdAt"`
	FinishedAt *time.Time          `json:"finishedAt,omitempty"`
}

type AuthorizationAttempt struct {
	state AuthorizationAttemptState
}

func NewAuthorizationAttempt(id, server string, now time.Time) (AuthorizationAttempt, error) {
	return RehydrateAuthorizationAttempt(AuthorizationAttemptState{
		ID: id, Server: server, Status: AuthorizationPending, CreatedAt: now.UTC(),
	})
}

func RehydrateAuthorizationAttempt(state AuthorizationAttemptState) (AuthorizationAttempt, error) {
	if state.ID == "" || state.Server == "" || state.CreatedAt.IsZero() {
		return AuthorizationAttempt{}, fmt.Errorf("%w: incomplete authorization attempt", ErrInvalid)
	}
	switch state.Status {
	case AuthorizationPending:
		if state.FinishedAt != nil {
			return AuthorizationAttempt{}, fmt.Errorf("%w: pending attempt is already finished", ErrInvalid)
		}
	case AuthorizationSucceeded, AuthorizationFailed, AuthorizationCanceled:
		if state.FinishedAt == nil || state.FinishedAt.Before(state.CreatedAt) {
			return AuthorizationAttempt{}, fmt.Errorf("%w: terminal attempt has no valid finish time", ErrInvalid)
		}
	default:
		return AuthorizationAttempt{}, fmt.Errorf("%w: unknown authorization status", ErrInvalid)
	}
	return AuthorizationAttempt{state: state}, nil
}

func (value *AuthorizationAttempt) Finish(status AuthorizationStatus, now time.Time) error {
	if value.state.Status != AuthorizationPending {
		return errors.New("mcpserver: authorization attempt is already terminal")
	}
	if status != AuthorizationSucceeded && status != AuthorizationFailed && status != AuthorizationCanceled {
		return fmt.Errorf("%w: invalid terminal authorization status", ErrInvalid)
	}
	finishedAt := now.UTC()
	if finishedAt.Before(value.state.CreatedAt) {
		finishedAt = value.state.CreatedAt
	}
	value.state.Status = status
	value.state.FinishedAt = &finishedAt
	return nil
}

func (value AuthorizationAttempt) State() AuthorizationAttemptState {
	state := value.state
	if state.FinishedAt != nil {
		finishedAt := *state.FinishedAt
		state.FinishedAt = &finishedAt
	}
	return state
}

func (value AuthorizationAttempt) ID() string                  { return value.state.ID }
func (value AuthorizationAttempt) Server() string              { return value.state.Server }
func (value AuthorizationAttempt) Status() AuthorizationStatus { return value.state.Status }
func (value AuthorizationAttempt) CreatedAt() time.Time        { return value.state.CreatedAt }
func (value AuthorizationAttempt) FinishedAt() *time.Time      { return value.State().FinishedAt }
func (value AuthorizationAttempt) Terminal() bool              { return value.state.Status != AuthorizationPending }

func (value AuthorizationAttempt) Expired(cutoff time.Time) bool {
	return value.state.FinishedAt != nil && !value.state.FinishedAt.After(cutoff)
}

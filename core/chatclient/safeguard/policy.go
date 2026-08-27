package safeguard

import (
	"context"
	"errors"
	"fmt"
)

var (
	// ErrUnsafeContent identifies policy rejections. Use errors.As with
	// *UnsafeError to inspect the safe-to-disclose scope and term.
	ErrUnsafeContent           = errors.New("safeguard: unsafe content")
	ErrInvalidMiddlewareConfig = errors.New("safeguard: invalid middleware config")
	ErrInvalidSubstringConfig  = errors.New("safeguard: invalid substring matcher config")
	// ErrNilStream reports a wrapped Streamer that returned a nil sequence.
	ErrNilStream = errors.New("safeguard: nil stream sequence")
)

// Scope selects which side of a model exchange is screened.
type Scope string

const (
	ScopeInput  Scope = "input"
	ScopeOutput Scope = "output"
	ScopeBoth   Scope = "both"
)

// Valid reports whether scope selects one or both known directions.
func (s Scope) Valid() bool {
	switch s {
	case ScopeInput, ScopeOutput, ScopeBoth:
		return true
	default:
		return false
	}
}

func (s Scope) inspects(direction Scope) bool {
	return s == ScopeBoth || s == direction
}

// Match is a Matcher's decision for one text projection. Term should be empty
// when policy details must not be disclosed.
type Match struct {
	Term  string
	Found bool
}

// Matcher screens a text projection. Implementations may call remote policy
// services and must preserve context cancellation errors.
type Matcher interface {
	Match(ctx context.Context, text string) (Match, error)
}

// MatcherFunc adapts an ordinary function to Matcher.
type MatcherFunc func(ctx context.Context, text string) (Match, error)

// Match invokes m.
func (m MatcherFunc) Match(ctx context.Context, text string) (Match, error) {
	return m(ctx, text)
}

// Block describes a policy rejection delivered to MiddlewareConfig.OnBlock.
type Block struct {
	Scope Scope
	Term  string
}

// UnsafeError is a policy rejection. It unwraps to ErrUnsafeContent.
type UnsafeError struct {
	Block Block
}

func (u *UnsafeError) Error() string {
	if u == nil {
		return ErrUnsafeContent.Error()
	}
	if u.Block.Term == "" {
		return fmt.Sprintf("%s: %s blocked", ErrUnsafeContent, u.Block.Scope)
	}
	return fmt.Sprintf("%s: %s matched %q", ErrUnsafeContent, u.Block.Scope, u.Block.Term)
}

// Unwrap supports errors.Is(err, ErrUnsafeContent).
func (u *UnsafeError) Unwrap() error {
	return ErrUnsafeContent
}

package safeguard

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/scope/core/chat"
)

var (
	// ErrUnsafeContent identifies policy rejections. Use errors.As with
	// *UnsafeError to inspect the safe-to-disclose scope and term.
	ErrUnsafeContent = errors.New("safeguard: unsafe content")
	// ErrInvalidConfig reports a missing matcher or invalid scope.
	ErrInvalidConfig = errors.New("safeguard: invalid config")
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

// Block describes a policy rejection delivered to Config.OnBlock.
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

// Config controls one immutable Middleware. A zero Scope defaults to
// ScopeBoth. OnBlock is optional and runs synchronously before rejection is
// returned to the caller.
type Config struct {
	Scope   Scope
	OnBlock func(context.Context, Block)
}

// Middleware screens model inputs and outputs.
type Middleware struct {
	matcher Matcher
	config  Config
}

// New constructs middleware. A nil matcher is rejected instead of silently
// disabling a security boundary.
func New(matcher Matcher, config Config) (*Middleware, error) {
	if matcher == nil {
		return nil, fmt.Errorf("%w: nil matcher", ErrInvalidConfig)
	}
	if config.Scope == "" {
		config.Scope = ScopeBoth
	}
	if !config.Scope.Valid() {
		return nil, fmt.Errorf("%w: unknown scope %q", ErrInvalidConfig, config.Scope)
	}
	return &Middleware{matcher: matcher, config: config}, nil
}

func (m *Middleware) blocked(ctx context.Context, block Block) error {
	if m.config.OnBlock != nil {
		m.config.OnBlock(ctx, block)
	}
	return &UnsafeError{Block: block}
}

func (m *Middleware) match(ctx context.Context, scope Scope, text string) (*Block, error) {
	if text == "" {
		return nil, nil
	}
	match, err := m.matcher.Match(ctx, text)
	if err != nil {
		return nil, fmt.Errorf("safeguard: match %s: %w", scope, err)
	}
	if !match.Found {
		return nil, nil
	}
	return &Block{Scope: scope, Term: match.Term}, nil
}

func (m *Middleware) scanInput(ctx context.Context, request *chat.Request) (*Block, error) {
	if !m.config.Scope.inspects(ScopeInput) || request == nil {
		return nil, nil
	}
	for i := range request.Messages {
		message := &request.Messages[i]
		if message.Role != chat.RoleSystem && message.Role != chat.RoleUser {
			continue
		}
		block, err := m.match(ctx, ScopeInput, message.Text())
		if err != nil || block != nil {
			return block, err
		}
	}
	return nil, nil
}

func (m *Middleware) scanOutput(ctx context.Context, response *chat.Response) (*Block, error) {
	if !m.config.Scope.inspects(ScopeOutput) || response == nil {
		return nil, nil
	}
	if response.Output == nil || response.Output.Message == nil {
		return nil, nil
	}
	block, err := m.match(ctx, ScopeOutput, response.Output.Message.Text())
	if err != nil || block != nil {
		return block, err
	}
	return nil, nil
}

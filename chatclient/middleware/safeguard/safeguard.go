package safeguard

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/core/chat"
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
type Scope uint8

const (
	ScopeInput Scope = 1 << iota
	ScopeOutput
	ScopeBoth = ScopeInput | ScopeOutput
)

// Valid reports whether scope selects one or both known directions.
func (s Scope) Valid() bool {
	return s != 0 && s&^ScopeBoth == 0
}

// String returns a stable diagnostic name.
func (s Scope) String() string {
	switch s {
	case ScopeInput:
		return "input"
	case ScopeOutput:
		return "output"
	case ScopeBoth:
		return "input+output"
	default:
		return fmt.Sprintf("Scope(%d)", s)
	}
}

func (s Scope) inspects(direction Scope) bool {
	return s&direction != 0
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

// Match invokes f.
func (f MatcherFunc) Match(ctx context.Context, text string) (Match, error) {
	return f(ctx, text)
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

func (e *UnsafeError) Error() string {
	if e == nil {
		return ErrUnsafeContent.Error()
	}
	if e.Block.Term == "" {
		return fmt.Sprintf("%s: %s blocked", ErrUnsafeContent, e.Block.Scope)
	}
	return fmt.Sprintf("%s: %s matched %q", ErrUnsafeContent, e.Block.Scope, e.Block.Term)
}

// Unwrap supports errors.Is(err, ErrUnsafeContent).
func (e *UnsafeError) Unwrap() error {
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
	if config.Scope == 0 {
		config.Scope = ScopeBoth
	}
	if !config.Scope.Valid() {
		return nil, fmt.Errorf("%w: unknown scope %d", ErrInvalidConfig, config.Scope)
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
	for i := range response.Choices {
		choice := &response.Choices[i]
		if choice.Message == nil {
			continue
		}
		block, err := m.match(ctx, ScopeOutput, choice.Message.Text())
		if err != nil || block != nil {
			return block, err
		}
	}
	return nil, nil
}

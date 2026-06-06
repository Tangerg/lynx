package toolpolicy

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/Tangerg/lynx/core/model/chat"
)

// ErrToolAlreadyCalled is returned by a [OnceOnly]-wrapped tool
// when the LLM tries to invoke it twice within the same scope. The
// error is informative — the chat ToolMiddleware feeds it back to
// the LLM, which is supposed to pick a different tool next turn.
//
// Use errors.Is to detect this case in callers that want to swap a
// tool retry for a different recovery strategy.
var ErrToolAlreadyCalled = errors.New("toolpolicy: tool has already been called once in this scope")

// ErrToolLocked is returned by a [Unlocked]-wrapped tool when the
// unlock condition returns false. The accompanying message
// (formatted as "tool %q locked: %s") gives the LLM enough context
// to pick a different action.
var ErrToolLocked = errors.New("toolpolicy: tool is locked by an unlock condition")

// OnceOnly wraps tool so a second call within the same scope
// returns [ErrToolAlreadyCalled] instead of running the underlying
// tool. Each unique scope (see [Scope]) maintains its own
// already-called set.
//
// Default scope is per-[tool.NewMiddleware]-loop: callers wrap
// the action ctx with [LoopScope] before driving the tool loop;
// each tool's first call within that scope succeeds, subsequent
// calls reject. Without an explicit scope the tool effectively
// becomes "once per process lifetime".
//
// Mirrors embabel's `OneShotPerLoopTool` semantics. Returns an error
// when tool is nil — caller decides whether to surface or panic.
func OnceOnly(tool chat.Tool) (chat.Tool, error) {
	if tool == nil {
		return nil, errors.New("toolpolicy.OnceOnly: tool must not be nil")
	}
	return &onceOnlyTool{delegate: tool}, nil
}

type onceOnlyTool struct {
	delegate chat.Tool

	// processWideMu guards processWideCalled — the fallback set
	// used when no [LoopScope] is in ctx.
	processWideMu     sync.Mutex
	processWideCalled map[string]struct{}
}

func (t *onceOnlyTool) Definition() chat.ToolDefinition { return t.delegate.Definition() }
func (t *onceOnlyTool) Metadata() chat.ToolMetadata     { return t.delegate.Metadata() }

func (t *onceOnlyTool) Call(ctx context.Context, arguments string) (string, error) {
	name := t.delegate.Definition().Name

	if scope := scopeFromContext(ctx); scope != nil {
		if !scope.markCalled(name) {
			return "", fmt.Errorf("%w: %q", ErrToolAlreadyCalled, name)
		}
	} else {
		t.processWideMu.Lock()
		if t.processWideCalled == nil {
			t.processWideCalled = make(map[string]struct{})
		}
		if _, dup := t.processWideCalled[name]; dup {
			t.processWideMu.Unlock()
			return "", fmt.Errorf("%w: %q", ErrToolAlreadyCalled, name)
		}
		t.processWideCalled[name] = struct{}{}
		t.processWideMu.Unlock()
	}

	return t.delegate.Call(ctx, arguments)
}

// UnlockCondition returns true when the tool may run, false when it
// is gated. The reason text accompanies the error returned to the
// LLM when the gate is closed; an empty string defaults to a generic
// "locked" message.
type UnlockCondition func(ctx context.Context, arguments string) (allowed bool, reason string)

// Unlocked wraps tool to gate every Call behind condition. When
// condition returns false, the wrapped Call returns [ErrToolLocked]
// (wrapping the supplied reason). When true, the underlying tool
// runs normally.
//
// The condition is evaluated on every invocation — caller-provided
// state (typically blackboard) drives the gate. Use cases:
//
//   - tools that should only fire after authentication
//   - tools that should only fire once a prerequisite artifact is
//     bound on the blackboard
//   - tools that are part of a playbook step-machine
//
// Mirrors embabel's `PlaybookTool` + `UnlockCondition` semantics.
// Returns an error when tool or condition is nil — caller decides
// whether to surface or panic.
func Unlocked(tool chat.Tool, condition UnlockCondition) (chat.Tool, error) {
	if tool == nil {
		return nil, errors.New("toolpolicy.Unlocked: tool must not be nil")
	}
	if condition == nil {
		return nil, errors.New("toolpolicy.Unlocked: condition must not be nil")
	}
	return &unlockTool{delegate: tool, condition: condition}, nil
}

type unlockTool struct {
	delegate  chat.Tool
	condition UnlockCondition
}

func (t *unlockTool) Definition() chat.ToolDefinition { return t.delegate.Definition() }
func (t *unlockTool) Metadata() chat.ToolMetadata     { return t.delegate.Metadata() }

func (t *unlockTool) Call(ctx context.Context, arguments string) (string, error) {
	allowed, reason := t.condition(ctx, arguments)
	if !allowed {
		if reason == "" {
			reason = "unlock condition returned false"
		}
		return "", fmt.Errorf("%w: tool %q locked: %s",
			ErrToolLocked, t.delegate.Definition().Name, reason)
	}
	return t.delegate.Call(ctx, arguments)
}

// loopScope tracks the set of tools called within a particular
// LLM-tool-loop scope. Created by [LoopScope] and read by
// [OnceOnly].
type loopScope struct {
	mu     sync.Mutex
	called map[string]struct{}
}

// markCalled returns true on the first call for name, false on any
// subsequent call within the same scope.
func (s *loopScope) markCalled(name string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.called == nil {
		s.called = make(map[string]struct{})
	}
	if _, dup := s.called[name]; dup {
		return false
	}
	s.called[name] = struct{}{}
	return true
}

// scopeKey is the unexported context key under which [LoopScope]
// stashes a *loopScope.
type scopeKey struct{}

// LoopScope returns a child ctx carrying a fresh per-loop scope
// the [OnceOnly] decorator uses to track which tools have run.
// Action bodies wrap ctx with this before driving a chat tool loop:
//
//	ctx = toolpolicy.LoopScope(ctx)
//	text, _, err := req.Call().Text(ctx)
//
// Each LoopScope returns an isolated scope, so two tool loops
// running concurrently in the same goroutine tree don't share
// already-called state.
//
// Calls without an enclosing LoopScope fall back to a
// process-wide already-called set on the wrapping decorator
// instance — useful when the tool should genuinely fire only once
// per process.
func LoopScope(ctx context.Context) context.Context {
	return context.WithValue(ctx, scopeKey{}, &loopScope{})
}

// scopeFromContext returns the active loop scope, or nil.
func scopeFromContext(ctx context.Context) *loopScope {
	scope, _ := ctx.Value(scopeKey{}).(*loopScope)
	return scope
}

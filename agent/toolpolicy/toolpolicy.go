package toolpolicy

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/tools"
)

// ErrAlreadyCalled is returned by a [Once]-wrapped tool when a caller tries
// to invoke it twice within the same policy scope.
//
// Use errors.Is to detect this case in callers that want to choose a different
// recovery strategy.
var ErrAlreadyCalled = errors.New("toolpolicy: tool already called in this policy scope")

// ErrLocked is returned by a [Gate]-wrapped tool when its condition rejects
// the call. The wrapped error names the tool and includes the condition's
// reason so a tool loop can present actionable feedback to the model.
var ErrLocked = errors.New("toolpolicy: tool call blocked by condition")

// Once wraps tool so a second call within the same scope returns
// [ErrAlreadyCalled] instead of running the underlying tool. Each scope
// created by [WithScope] maintains its own called set.
//
// Without a scope, the allowance belongs to the returned decorator instance:
// its first call succeeds and all later calls reject for that instance's
// lifetime. This is local admission policy, not external-effect idempotency or
// cross-process coordination. Returns an error when tool is nil.
func Once(tool tools.Tool) (tools.Tool, error) {
	if tool == nil {
		return nil, errors.New("toolpolicy.Once: tool must not be nil")
	}
	return &onceTool{delegate: tool}, nil
}

type onceTool struct {
	delegate tools.Tool
	mu       sync.Mutex
	called   bool
}

var _ tools.FileMutationReporter = (*onceTool)(nil)

func (t *onceTool) Definition() chat.ToolDefinition { return t.delegate.Definition() }

func (t *onceTool) ReturnsDirect() bool {
	if d, ok := t.delegate.(interface{ ReturnsDirect() bool }); ok {
		return d.ReturnsDirect()
	}
	return false
}

// MutationPaths forwards prospective file targets without consuming the
// once-only allowance. Metadata inspection must not change policy state.
func (t *onceTool) MutationPaths(arguments string) ([]string, error) {
	if reporter, ok := t.delegate.(tools.FileMutationReporter); ok {
		return reporter.MutationPaths(arguments)
	}
	return nil, nil
}

func (t *onceTool) Call(ctx context.Context, arguments string) (string, error) {
	name := t.delegate.Definition().Name

	if scope := scopeFromContext(ctx); scope != nil {
		if !scope.markCalled(name) {
			return "", fmt.Errorf("%w: %q", ErrAlreadyCalled, name)
		}
	} else {
		t.mu.Lock()
		if t.called {
			t.mu.Unlock()
			return "", fmt.Errorf("%w: %q", ErrAlreadyCalled, name)
		}
		t.called = true
		t.mu.Unlock()
	}

	return t.delegate.Call(ctx, arguments)
}

// Condition returns true when a tool may run. When it returns false, reason is
// included in the [ErrLocked] error; an empty reason receives a useful default.
type Condition func(ctx context.Context, arguments string) (allowed bool, reason string)

// Gate wraps tool so condition decides every call. A rejected call returns
// [ErrLocked] without invoking the underlying tool; an accepted call delegates
// normally.
//
// The condition is evaluated on every invocation — caller-provided
// state (typically blackboard) drives the gate. Use cases:
//
//   - tools that should only fire after authentication
//   - tools that should only fire once a prerequisite artifact is
//     bound on the blackboard
//   - tools that are part of a playbook step-machine
//
// Returns an error when tool or condition is nil.
func Gate(tool tools.Tool, condition Condition) (tools.Tool, error) {
	if tool == nil {
		return nil, errors.New("toolpolicy.Gate: tool must not be nil")
	}
	if condition == nil {
		return nil, errors.New("toolpolicy.Gate: condition must not be nil")
	}
	return &gatedTool{delegate: tool, condition: condition}, nil
}

type gatedTool struct {
	delegate  tools.Tool
	condition Condition
}

var _ tools.FileMutationReporter = (*gatedTool)(nil)

func (t *gatedTool) Definition() chat.ToolDefinition { return t.delegate.Definition() }

func (t *gatedTool) ReturnsDirect() bool {
	if d, ok := t.delegate.(interface{ ReturnsDirect() bool }); ok {
		return d.ReturnsDirect()
	}
	return false
}

// MutationPaths forwards prospective targets without evaluating the gate
// condition. The runtime may need those targets before it invokes Call.
func (t *gatedTool) MutationPaths(arguments string) ([]string, error) {
	if reporter, ok := t.delegate.(tools.FileMutationReporter); ok {
		return reporter.MutationPaths(arguments)
	}
	return nil, nil
}

func (t *gatedTool) Call(ctx context.Context, arguments string) (string, error) {
	allowed, reason := t.condition(ctx, arguments)
	if !allowed {
		if reason == "" {
			reason = "condition returned false"
		}
		return "", fmt.Errorf("%w: %q: %s", ErrLocked, t.delegate.Definition().Name, reason)
	}
	return t.delegate.Call(ctx, arguments)
}

// loopScope tracks the set of tools called within a particular
// tool-loop scope. Created by [WithScope] and read by [Once].
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

// scopeKey is the unexported context key under which [WithScope]
// stashes a *loopScope.
type scopeKey struct{}

// WithScope returns a child context carrying a fresh scope shared by all
// [Once]-wrapped tools invoked with that context. The scope identifies tools
// by [chat.ToolDefinition.Name].
// Action bodies wrap ctx with this before driving a chat tool loop:
//
//	ctx = toolpolicy.WithScope(ctx)
//	text, _, err := request.Call().Text(ctx)
//
// Each call returns an isolated scope, so two tool loops
// running concurrently in the same goroutine tree don't share
// already-called state.
//
// Calls without an enclosing scope use the decorator-instance allowance
// documented by [Once].
func WithScope(ctx context.Context) context.Context {
	return context.WithValue(ctx, scopeKey{}, &loopScope{})
}

func scopeFromContext(ctx context.Context) *loopScope {
	scope, _ := ctx.Value(scopeKey{}).(*loopScope)
	return scope
}

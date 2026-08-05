package core

import (
	"context"
	"errors"
	"fmt"
	"math"
)

// ConfigureChildFunc supplies explicit per-child process configuration. Parallel
// child creation may call it concurrently, so an implementation owns
// synchronization for whatever mutable state it captures.
type ConfigureChildFunc func(
	ctx context.Context,
	parent ProcessView,
	child AgentDescriptor,
) (ProcessOptions, error)

// ProcessOptions is the per-process configuration bundle. Its zero value is
// unbounded and uses no optional capabilities. The runtime validates and
// snapshots its container fields before retaining them. Callers may reuse
// or mutate the Extensions slice and ChatMiddleware value after process
// construction without changing the running process; the capability objects
// stored inside those containers must themselves remain safe for their declared
// lifetime.
//
// Cross-cutting behavior belongs on an extension registered via
// [ProcessOptions.Extensions], not on a new field here.
type ProcessOptions struct {
	Blackboard Blackboard

	// ConfigureChild configures every child process spawned by this process,
	// including agent-as-tool and workflow children. The callback receives the
	// read-only parent and an inert description of the exact child definition. A
	// nil returned Blackboard keeps the selected RunChild inheritance mode;
	// other returned fields configure the child normally.
	//
	// The callback itself is inherited by descendants unless the returned
	// ProcessOptions supplies a different non-nil ConfigureChild, so one explicit
	// host policy can cover the whole delegation tree. nil preserves the
	// framework default: children inherit only their declared blackboard mode
	// and explicitly subtree-scoped event listeners.
	ConfigureChild ConfigureChildFunc

	// Budget caps cumulative execution across the complete process tree rooted
	// at this process. Children share one runtime-owned atomic admission
	// authority with the root, so concurrent siblings cannot independently
	// consume the same remaining capacity. ConfigureChild must not configure a
	// second Budget; tree-wide limits belong on the root ProcessOptions.
	Budget Budget

	// Dependencies is an optional process scope descending from
	// [runtime.Engine.Dependencies]. The runtime freezes it — and every scope
	// between it and the engine — when the process starts, then creates a fresh
	// action child for each execution. nil creates an empty process scope over
	// the engine dependencies automatically.
	//
	// Ancestry is validated at runtime so an unrelated dependency tree cannot
	// silently bypass engine composition. Intermediate layers are the host's own
	// composition and are not constrained.
	Dependencies *Dependencies

	// Extensions are plug-ins scoped to this one process. They merge with the
	// engine-scoped set at dispatch time, taking the inner position, so a
	// process middleware runs inside every engine one. Each capability
	// documents the scopes it is valid in; one that only makes sense engine-wide
	// is rejected here.
	//
	// Names must be unique within Extensions, but may deliberately collide with
	// an engine-scope Name — that collision is how a process overrides.
	Extensions []Extension

	// ChatMiddleware is composed outside the engine-level middleware for this
	// process. The process layer runs first, allowing it to bind request-scoped
	// context before shared middleware executes.
	ChatMiddleware *ChatMiddleware

	// MaxModelCalls bounds the model calls of every managed interaction this
	// process runs, including Prompt. Zero uses the engine default; zero at both
	// levels leaves each interaction bounded only by what it states itself and by
	// the tree Budget.
	MaxModelCalls int
}

// Budget limits cumulative host-defined cost, action invocations, model calls,
// and tokens across one complete process tree. ActionLimit and ModelCallLimit
// are strict admission caps. CostLimit and TokenLimit are continuation
// ceilings: the runtime cannot know a response's final usage before I/O, so an
// already-admitted call may cross either ceiling, after which no further work
// is admitted. Zero leaves a dimension unbounded; choosing units and limits is
// host policy.
type Budget struct {
	CostLimit      float64
	ActionLimit    int
	ModelCallLimit int
	TokenLimit     int64
}

// ErrInvalidBudget identifies a malformed process budget.
var ErrInvalidBudget = errors.New("budget: invalid")

// Validate checks that every configured limit is finite and non-negative.
// Zero leaves that dimension unbounded.
func (b Budget) Validate() error {
	if math.IsNaN(b.CostLimit) || math.IsInf(b.CostLimit, 0) || b.CostLimit < 0 {
		return fmt.Errorf("%w: cost limit must be finite and non-negative", ErrInvalidBudget)
	}
	if b.ActionLimit < 0 {
		return fmt.Errorf("%w: action limit must not be negative", ErrInvalidBudget)
	}
	if b.ModelCallLimit < 0 {
		return fmt.Errorf("%w: model-call limit must not be negative", ErrInvalidBudget)
	}
	if b.TokenLimit < 0 {
		return fmt.Errorf("%w: token limit must not be negative", ErrInvalidBudget)
	}
	return nil
}

package core

import (
	"context"
	"errors"
	"fmt"
	"math"
)

// ChildOptionsFunc supplies explicit per-child process configuration. It runs
// only when a parent ProcessOptions installs it, so the framework's default
// minimal child inheritance remains unchanged. Parallel child creation may call
// the same function concurrently; the implementation owns synchronization for
// captured mutable state.
type ChildOptionsFunc func(
	ctx context.Context,
	parent ProcessView,
	child *Agent,
) (ProcessOptions, error)

// ProcessOptions is the per-process configuration bundle. Pass a zero
// ProcessOptions{} when defaults suffice; the runtime normalizes unset fields
// and snapshots its container fields before retaining them. Callers may reuse
// or mutate the Session, Extensions slice, and Guardrails value after process
// construction without changing the running process; the capability objects
// stored inside those containers must themselves remain safe for their declared
// lifetime.
//
// Choosing a struct over the functional-options pattern keeps defaults
// and validation in one place and avoids polluting the
// package namespace with ~10 `With…` constructors. Direct struct-
// literal init is the intended ergonomics. Cross-cutting concerns
// (audit, verbosity, throttling, RBAC) belong on extensions registered
// via [Extensions]; ProcessOptions itself stays minimal.
type ProcessOptions struct {
	Blackboard Blackboard

	// ChildOptions configures every child process spawned by this process,
	// including agent-as-tool and workflow children. The callback receives the
	// read-only parent and exact child definition. A nil returned Blackboard
	// keeps the selected RunChild inheritance mode; other returned fields
	// configure the child normally.
	//
	// The callback itself is inherited by descendants unless the returned
	// ProcessOptions supplies a different non-nil ChildOptions, so one explicit
	// host policy can cover the whole delegation tree. nil preserves the
	// framework default: children inherit only their declared blackboard mode
	// and process event listeners.
	ChildOptions ChildOptionsFunc

	// Budget caps cumulative LLM spend (USD), action invocations, and
	// total tokens for this process. The runtime always checks a
	// Budget-derived [BudgetPolicy] implicitly each tick; additional
	// [StopPolicy] extensions can be registered via
	// Extensions (OR semantics — any policy triggers termination).
	Budget Budget

	// Dependencies is an optional process scope created from
	// [runtime.Engine.Dependencies] by calling [Dependencies.Child]. The runtime
	// freezes it when the process starts and creates a fresh action child for
	// each execution. nil creates an empty process scope over the engine
	// dependencies automatically.
	//
	// The parent relationship is validated at runtime so an unrelated dependency
	// tree cannot silently bypass engine composition.
	Dependencies *Dependencies

	// Session optionally binds this process to a multi-turn conversation. When
	// Guardrails provides BindConversation, the runtime passes [Session.ID] to
	// that host-owned context projection without serializing runtime scope into
	// chat.Request.
	//
	// Typically set via [Engine.RunInSession]; the runtime fills
	// the field and refreshes [Session.UpdatedAt] on every dispatch.
	Session *Session

	// Extensions are process-scoped plug-ins active for the lifetime of
	// this single process. They merge with engine-scoped extensions at
	// dispatch time — process extensions take inner / higher priority; for
	// example, a process ActionMiddleware sits inside every engine middleware.
	// Only capabilities documented for process scope are accepted; deploy-time
	// AgentValidator, IDGenerator, and Blackboard prototype capabilities belong
	// to engine scope. Use Blackboard above for an explicit process override.
	// Within Extensions, each
	// [Extension.Name] must be unique; the runtime returns an error
	// from Run / Start / Continue when this constraint
	// is violated. Process-scope Names may collide with engine-scope
	// Names — that's the explicit override mechanism.
	Extensions []Extension

	// Guardrails, when non-nil, overrides the engine-level guardrails
	// for this process. nil means "use the engine default". Set it to
	// inject per-process chat middleware (tool loop, history) so agent/core
	// doesn't need to import middleware implementations.
	Guardrails *ChatGuardrails
}

// Budget caps cumulative LLM spend (USD), action invocations, and total
// tokens for one process. Budget is enforced via [BudgetPolicy], which
// the runtime checks implicitly each tick — so a zero-options caller
// gets the [DefaultBudget] limits automatically. Additional policies
// (DLP, rate-limit, custom guardrails) can be registered as
// [StopPolicy] extensions; all are OR-composed.
type Budget struct {
	CostLimit   float64
	ActionLimit int
	TokenLimit  int
}

// ErrInvalidBudget identifies a malformed process budget.
var ErrInvalidBudget = errors.New("budget: invalid")

// Validate checks that every configured limit is finite and non-negative.
// Zero leaves that dimension unbounded; the all-zero Budget selects runtime
// defaults at the ProcessOptions boundary.
func (b Budget) Validate() error {
	if math.IsNaN(b.CostLimit) || math.IsInf(b.CostLimit, 0) || b.CostLimit < 0 {
		return fmt.Errorf("%w: cost limit must be finite and non-negative", ErrInvalidBudget)
	}
	if b.ActionLimit < 0 {
		return fmt.Errorf("%w: action limit must not be negative", ErrInvalidBudget)
	}
	if b.TokenLimit < 0 {
		return fmt.Errorf("%w: token limit must not be negative", ErrInvalidBudget)
	}
	return nil
}

const (
	DefaultBudgetCostLimit   = 2.0
	DefaultBudgetActionLimit = 50
	DefaultBudgetTokenLimit  = 1_000_000
)

// DefaultBudget is the baseline that applies when callers don't supply one —
// ~$2 cap, 50 actions, 1M tokens. The numbers are deliberately generous;
// production deployments tune them per-tenant.
func DefaultBudget() Budget {
	return Budget{
		CostLimit:   DefaultBudgetCostLimit,
		ActionLimit: DefaultBudgetActionLimit,
		TokenLimit:  DefaultBudgetTokenLimit,
	}
}

package turn

import (
	"context"
	"errors"
	"sync"

	"github.com/google/uuid"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/approval"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/hooks"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/todo"
)

// todoLister reads a session's current todo list — narrow consumer view of the
// todo store (the turn only reads, never writes). The turn projects the list
// to state.snapshot{todos} after a todo_write so a client renders the task
// panel. nil disables the projection.
type todoLister interface {
	List(ctx context.Context, sessionID string) ([]todo.Item, error)
}

// turnIDPrefix tags every turn id. A turn id doubles as the root run's
// wire id (runs.start returns it as runId), so it carries the run_ type
// prefix (API.md §2.2; mirrors protocol.IDPrefixRun).
const turnIDPrefix = "run_"

func newTurnID() string { return turnIDPrefix + uuid.NewString() }

type hookResolver interface {
	For(ctx context.Context, cwd string) *hooks.Bound
}

// Dependencies names the collaborators needed by the in-process [Dispatcher].
// Starter, Restorer, Steering, and Maintenance are required; every other field
// is optional and has a nil-default behavior documented on the field.
type Dependencies struct {
	// Starter dispatches the underlying agent process for a fresh turn. Required.
	Starter turnStarter

	// Restorer rebuilds a parked turn after a backend restart. Required.
	Restorer turnRestorer

	// Steering persists queued steering messages once a turn ends. Required.
	Steering steeringSink

	// Maintenance runs post-turn compaction/extraction. Required.
	Maintenance maintenanceRunner

	// Approval gates tool calls. nil auto-approves every tool, useful for tests
	// and smoke runs.
	Approval approval.Policy

	// ClientResolver resolves an explicit per-turn provider/model client. nil
	// keeps every turn on the platform default client.
	ClientResolver clientResolver

	// Todos reads the session's todo list for state.snapshot projection after a
	// todo_write. nil disables the projection.
	Todos todoLister

	// MCPAutoApprove returns the model-facing MCP tool names whose calls skip
	// the approval prompt. nil disables MCP-specific auto-approval.
	MCPAutoApprove func() map[string]struct{}

	// Hooks resolves lifecycle hooks for a turn's cwd. nil disables hooks.
	Hooks hookResolver
}

// New returns the [Dispatcher] implementation. The implementation is
// single-process — it holds in-memory state about live turns and
// fans events out to subscribers via per-turn channels.
//
// The implementation is split across files by concern:
//   - dispatcher.go     — Dispatcher interface + package entry points
//   - request.go        — Start/Rehydrate request shapes + validation
//   - event.go          — turn event model + terminal reason vocabulary
//   - inmemory.go       — in-process dispatcher construction + shared state
//   - turn_start.go     — start-turn admission into the agent engine
//   - turn_control.go   — cancel/resume interrupt control
//   - rehydrate.go      — cross-restart parked-turn resume
//   - live_registry.go  — live-turn lookup + negotiated interrupt gates
//   - event_emit.go     — stamped event delivery and backpressure semantics
//   - state.go          — per-turn state + cross-goroutine invariants
//   - turn.go           — run/drive/interrupt lifecycle
//   - terminal.go       — terminal event mapping + teardown
//   - steering.go       — mid-run steering source + final history flush
//   - event_stream.go   — event subscription + transient delta coalescing
//   - prompt_hooks.go   — pre-turn lifecycle hooks
//   - lifecycle.go      — terminal-event capture from the agent runtime
//   - observer.go       — engine tool-observer → turn.Event translation
//
// The Dispatcher interface is stable, so transport adapters don't care
// which impl they talk to.
func New(deps Dependencies) (Dispatcher, error) {
	if deps.Starter == nil {
		return nil, errors.New("turn: starter is required")
	}
	if deps.Restorer == nil {
		return nil, errors.New("turn: restorer is required")
	}
	if deps.Steering == nil {
		return nil, errors.New("turn: steering is required")
	}
	if deps.Maintenance == nil {
		return nil, errors.New("turn: maintenance is required")
	}
	return &inMemory{
		starter:        deps.Starter,
		restorer:       deps.Restorer,
		steering:       deps.Steering,
		maintenance:    deps.Maintenance,
		approval:       deps.Approval,
		resolver:       deps.ClientResolver,
		todos:          deps.Todos,
		mcpAutoApprove: deps.MCPAutoApprove,
		hooks:          deps.Hooks,
		turns:          map[string]*turnState{},
		seenSessions:   map[string]struct{}{},
	}, nil
}

// inMemory is the single-process [Dispatcher] implementation. It
// tracks live turns in a map keyed by turn id; state lives in
// process memory and does not survive restart.
type inMemory struct {
	starter     turnStarter
	restorer    turnRestorer
	steering    steeringSink
	maintenance maintenanceRunner
	approval    approval.Policy // optional — nil = auto-approve every tool
	resolver    clientResolver  // optional — nil = always use the default model
	todos       todoLister      // optional — nil = no state.snapshot{todos} projection

	// mcpAutoApprove returns the model-facing MCP tool names whose calls skip the
	// approval prompt — a per-server whitelist the runtime recomputes on every
	// MCP registry change. Consulted on the GatePrompt path only, AFTER standing
	// rules, so it never overrides a remembered deny or the read-only plan-mode
	// deny; it only spares a prompt the user would otherwise see. nil = off.
	mcpAutoApprove func() map[string]struct{}

	// hooks resolves the lifecycle-hook set for a turn's cwd. nil = no hooks.
	hooks hookResolver

	// mu guards the live-turn registry + interruptKinds + seenSessions; each
	// turn owns the synchronization of its own cross-goroutine state (see
	// turnState.mu).
	mu    sync.Mutex
	turns map[string]*turnState // turn_id → state

	// seenSessions tracks which sessions this process has already opened a turn
	// for, so the SessionStart hook fires once per session per process (not on
	// every turn). Guarded by mu.
	seenSessions map[string]struct{}

	// interruptKinds is the allowlist of HITL kinds the connected client
	// declared it can answer (ClientCapabilities.InterruptKinds). nil means
	// unconfigured → surface every kind (the permissive default for
	// in-process / CLI callers that don't negotiate). A non-nil set gates
	// strictly: a turn about to park on a kind absent here is auto-denied
	// rather than left as an unanswerable interrupt (API.md §6.2). Guarded
	// by mu.
	interruptKinds map[string]bool
}

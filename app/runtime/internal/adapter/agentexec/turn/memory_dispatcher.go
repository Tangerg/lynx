package turn

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"sync"
	"time"

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

// turnIDPrefix tags adapter-local turn handles. A TurnID is neither the stable
// domain RunID nor the agent process snapshot id, so it has its own namespace.
const turnIDPrefix = "turn_"

func newTurnID() string { return turnIDPrefix + uuid.NewString() }

type hookResolver interface {
	For(ctx context.Context, cwd string) *hooks.Bound
}

// Dependencies names the collaborators needed by the in-process [Dispatcher].
// Engine is required; every other field is optional and has a nil-default
// behavior documented on the field.
type Dependencies struct {
	// Engine drives the turn: start / restore / steer / post-turn maintenance.
	// Required.
	Engine engineDep

	// Approval gates tool calls. nil auto-approves every tool, useful for tests
	// and smoke runs.
	Approval approval.Policy

	// ClientResolver resolves an explicit per-turn provider/model client. nil
	// keeps every turn on the engine default client.
	ClientResolver clientResolver

	// Todos reads the session's todo list for state.snapshot projection after a
	// todo_write. nil disables the projection.
	Todos todoLister

	// MCPToolAutoApproved reports whether a model-facing MCP tool may skip the
	// approval prompt. nil disables MCP-specific auto-approval.
	MCPToolAutoApproved func(string) bool

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
//   - memory_dispatcher.go       — in-process dispatcher construction + shared state
//   - turn_start.go     — start-turn admission into the agent engine
//   - turn_control.go   — cancel/resume interrupt control
//   - rehydrate.go      — cross-restart parked-turn resume
//   - live_registry.go  — live-turn lookup + per-turn interrupt gates
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
// The Dispatcher interface is stable, so transport adapters do not depend on
// its concrete implementation.
func New(deps Dependencies) (Dispatcher, error) {
	if deps.Engine == nil {
		return nil, errors.New("turn: engine is required")
	}
	return &memoryDispatcher{
		engine:              deps.Engine,
		approval:            deps.Approval,
		resolver:            deps.ClientResolver,
		todos:               deps.Todos,
		mcpToolAutoApproved: deps.MCPToolAutoApproved,
		hooks:               deps.Hooks,
		turns:               map[string]*turnState{},
		seenSessions:        map[string]struct{}{},
	}, nil
}

// memoryDispatcher is the single-process [Dispatcher] implementation. It
// tracks live turns in a map keyed by turn id; state lives in
// process memory and does not survive restart.
type memoryDispatcher struct {
	engine   engineDep
	approval approval.Policy // optional — nil = auto-approve every tool
	resolver clientResolver  // optional — nil = always use the default model
	todos    todoLister      // optional — nil = no state.snapshot{todos} projection

	// mcpToolAutoApproved reports whether a model-facing MCP tool skips the
	// approval prompt. The runtime recomputes the policy on every
	// MCP registry change. Consulted on the GatePrompt path only, AFTER standing
	// rules, so it never overrides a remembered deny or the read-only plan-mode
	// deny; it only spares a prompt the user would otherwise see. nil = off.
	mcpToolAutoApproved func(string) bool

	// hooks resolves the lifecycle-hook set for a turn's cwd. nil = no hooks.
	hooks hookResolver

	// mu guards the live-turn registry + seenSessions; each turn owns the
	// synchronization of its own cross-goroutine state (see turnState.mu).
	mu        sync.Mutex
	turns     map[string]*turnState // turn_id → state
	closed    bool
	closeOnce sync.Once
	closing   []*turnState

	// seenSessions tracks which sessions this process has already opened a turn
	// for, so the SessionStart hook fires once per session per process (not on
	// every turn). Guarded by mu.
	seenSessions map[string]struct{}
}

func (s *memoryDispatcher) register(st *turnState) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return false
	}
	s.turns[st.handle.TurnID] = st
	return true
}

func (s *memoryDispatcher) isClosed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed
}

const turnCloseTimeout = 5 * time.Second

// Close cancels and joins the complete live-turn set within a bounded shutdown
// budget. The dispatcher, not the delivery run registry, is authoritative
// because parked turns remain live after their streaming segment has ended.
func (s *memoryDispatcher) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), turnCloseTimeout)
	defer cancel()
	return s.close(ctx)
}

func (s *memoryDispatcher) close(ctx context.Context) error {
	s.closeOnce.Do(func() {
		s.mu.Lock()
		s.closed = true
		s.closing = slices.Collect(maps.Values(s.turns))
		states := slices.Clone(s.closing)
		s.mu.Unlock()

		for _, st := range states {
			go func() {
				_ = s.Cancel(context.WithoutCancel(st.ctx), st.handle)
			}()
		}
	})

	for _, st := range s.closing {
		select {
		case <-st.done:
		case <-ctx.Done():
			remaining := 0
			for _, pending := range s.closing {
				select {
				case <-pending.done:
				default:
					remaining++
				}
			}
			return fmt.Errorf("%w: %d turn(s) still running", ErrCloseTimeout, remaining)
		}
	}
	return nil
}

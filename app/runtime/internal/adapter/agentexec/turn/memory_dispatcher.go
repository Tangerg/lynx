package turn

import (
	"cmp"
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
	"github.com/Tangerg/lynx/app/runtime/internal/domain/mcpserver"
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
	For(ctx context.Context, cwd string) (*hooks.Bound, error)
}

// Dependencies names the collaborators needed by the in-process [Dispatcher].
// Engine is required; every other field is optional and has a nil-default
// behavior documented on the field.
type Dependencies struct {
	// Engine starts or restores the Agent process tree. Required.
	Engine engineDep

	// Steering persists queued messages that miss the current continuation
	// round. nil reports a steering error only when such a message exists.
	Steering SteeringSink

	// Compactor and Extractor run visible turn-boundary maintenance. nil
	// disables the corresponding operation.
	Compactor Compactor
	Extractor Extractor

	// Approval gates tool calls. nil auto-approves every tool, useful for tests
	// and smoke runs.
	Approval approval.Policy

	// ClientResolver resolves an explicit per-turn provider/model client. nil
	// keeps every turn on the engine default client.
	ClientResolver clientResolver

	// Todos reads the session's todo list for state.snapshot projection after a
	// todo_write. nil disables the projection.
	Todos todoLister

	// MCPToolAutoApproved reports whether an identified MCP tool may skip the
	// approval prompt. nil disables MCP-specific auto-approval.
	MCPToolAutoApproved func(mcpserver.ToolRef) bool

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
// The Dispatcher interface is the consumer-side process-control boundary used
// by the application adapters; delivery never drives it directly.
func New(deps Dependencies) (Dispatcher, error) {
	if deps.Engine == nil {
		return nil, errors.New("turn: engine is required")
	}
	return &memoryDispatcher{
		engine:              deps.Engine,
		steering:            deps.Steering,
		compactor:           deps.Compactor,
		extractor:           deps.Extractor,
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
	engine    engineDep
	steering  SteeringSink
	compactor Compactor
	extractor Extractor
	approval  approval.Policy // optional — nil = auto-approve every tool
	resolver  clientResolver  // optional — nil = always use the default model
	todos     todoLister      // optional — nil = no state.snapshot{todos} projection

	// mcpToolAutoApproved reports whether an identified MCP tool skips the
	// approval prompt. The runtime recomputes the policy on every
	// MCP registry change. Consulted on the GatePrompt path only, AFTER standing
	// rules, so it never overrides a remembered deny or the read-only plan-mode
	// deny; it only spares a prompt the user would otherwise see. nil = off.
	mcpToolAutoApproved func(mcpserver.ToolRef) bool

	// hooks resolves the lifecycle-hook set for a turn's cwd. nil = no hooks.
	hooks hookResolver

	// mu guards the live-turn registry + seenSessions; each turn owns the
	// synchronization of its own cross-goroutine state (see turnState.mu).
	mu        sync.Mutex
	turns     map[string]*turnState // turn_id → state
	closed    bool
	closeOnce sync.Once
	closing   []*closeTarget

	// seenSessions tracks which sessions this process has already opened a turn
	// for, so the SessionStart hook fires once per session per process (not on
	// every turn). Guarded by mu.
	seenSessions map[string]struct{}
}

// closeTarget owns one shutdown cancellation result. Publishing err before
// closing cancelDone gives every Close caller a stable, race-free result even
// when an earlier Close timed out and a later call finishes the join.
type closeTarget struct {
	state      *turnState
	cancelDone chan struct{}
	err        error
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
		states := slices.Collect(maps.Values(s.turns))
		slices.SortFunc(states, func(left, right *turnState) int {
			return cmp.Compare(left.handle.TurnID, right.handle.TurnID)
		})
		s.closing = make([]*closeTarget, 0, len(states))
		for _, st := range states {
			s.closing = append(s.closing, &closeTarget{state: st, cancelDone: make(chan struct{})})
		}
		targets := slices.Clone(s.closing)
		s.mu.Unlock()

		for _, target := range targets {
			go func() {
				target.err = s.Cancel(context.WithoutCancel(target.state.ctx), target.state.handle)
				close(target.cancelDone)
			}()
		}
	})

	for _, target := range s.closing {
		select {
		case <-target.cancelDone:
		case <-ctx.Done():
			return errors.Join(closeTimeoutError(s.closing), closeCancellationErrors(s.closing))
		}
	}
	cancelErr := closeCancellationErrors(s.closing)
	for _, target := range s.closing {
		select {
		case <-target.state.done:
		case <-ctx.Done():
			return errors.Join(closeTimeoutError(s.closing), cancelErr)
		}
	}
	return cancelErr
}

func closeTimeoutError(targets []*closeTarget) error {
	remaining := 0
	for _, target := range targets {
		if !channelClosed(target.cancelDone) || !channelClosed(target.state.done) {
			remaining++
		}
	}
	return fmt.Errorf("%w: %d turn(s) still shutting down", ErrCloseTimeout, remaining)
}

func closeCancellationErrors(targets []*closeTarget) error {
	var errs []error
	for _, target := range targets {
		if !channelClosed(target.cancelDone) || target.err == nil || errors.Is(target.err, ErrTurnNotFound) {
			continue
		}
		errs = append(errs, fmt.Errorf("turn: close turn %q: %w", target.state.handle.TurnID, target.err))
	}
	return errors.Join(errs...)
}

func channelClosed(ch <-chan struct{}) bool {
	select {
	case <-ch:
		return true
	default:
		return false
	}
}

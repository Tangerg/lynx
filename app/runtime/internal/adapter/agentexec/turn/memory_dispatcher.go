package turn

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"sync"

	"github.com/google/uuid"

	"github.com/Tangerg/lynx/app/runtime/internal/component/shutdown"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/approval"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/hooks"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/mcpserver"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelref"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/todo"
	"github.com/Tangerg/lynx/chatclient"
)

// clientResolver resolves a per-turn chat client for one explicit model
// selection. It is turn's own narrow dependency on the runtime model registry;
// an unavailable provider or model is reported as an error.
type clientResolver interface {
	ResolveClient(ctx context.Context, selection modelref.Selection) (*chatclient.Client, error)
}

// todoLister reads a session's current todo list — narrow consumer view of the
// todo store (the turn only reads, never writes). The turn projects the list
// to state.snapshot{todos} after a todo_write so a client renders the task
// panel. nil disables the projection.
type todoLister interface {
	List(ctx context.Context, sessionID string) ([]todo.Item, error)
}

// ApprovalGate is the tool-call evaluator's complete approval view. Mode/rules
// management belongs to separate application and tool consumers.
type ApprovalGate interface {
	Mode(ctx context.Context) (approval.Mode, error)
	Decide(ctx context.Context, q approval.Query) (approval.Decision, bool, error)
	Remember(ctx context.Context, req approval.RememberRequest) error
}

// turnIDPrefix tags adapter-local turn handles. A TurnID is neither the stable
// domain RunID nor the agent process snapshot id, so it has its own namespace.
const turnIDPrefix = "turn_"

func newTurnID() string { return turnIDPrefix + uuid.NewString() }

type hookResolver interface {
	For(ctx context.Context, cwd string) (*hooks.Bound, error)
}

// Dependencies names the collaborators needed by the in-process dispatcher.
// Engine is required; every other field is optional and has a nil-default
// behavior documented on the field.
type Dependencies struct {
	// Engine starts or restores the Agent process tree. Required.
	Engine engineDep

	// Steering persists queued messages that miss the current continuation
	// round. nil reports a steering error only when such a message exists.
	Steering SteeringSink

	// Maintenance performs best-effort post-turn housekeeping. nil disables the
	// complete maintenance sweep.
	Maintenance BoundaryMaintenance

	// Approval gates tool calls. nil auto-approves every tool, useful for tests
	// and smoke runs.
	Approval ApprovalGate

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

// New builds the concrete in-process dispatcher. The dispatcher is
// single-process — it holds in-memory state about live turns and
// fans events out to subscribers via per-turn channels.
//
// The implementation is split across files by concern:
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
//   - observer.go       — engine tool-observer → application runs event translation
//
// Consumers define the narrow control ports they need; delivery never drives
// this adapter directly.
func New(deps Dependencies) (*memoryDispatcher, error) {
	if deps.Engine == nil {
		return nil, errors.New("turn: engine is required")
	}
	return &memoryDispatcher{
		engine:              deps.Engine,
		steering:            deps.Steering,
		maintenance:         deps.Maintenance,
		approval:            deps.Approval,
		resolver:            deps.ClientResolver,
		todos:               deps.Todos,
		mcpToolAutoApproved: deps.MCPToolAutoApproved,
		hooks:               deps.Hooks,
		turns:               map[string]*turnState{},
		seenSessions:        map[string]struct{}{},
	}, nil
}

// memoryDispatcher is the single-process turn implementation. It
// tracks live turns in a map keyed by turn id; state lives in
// process memory and does not survive restart.
type memoryDispatcher struct {
	engine      engineDep
	steering    SteeringSink
	maintenance BoundaryMaintenance // optional — nil = no turn-boundary maintenance
	approval    ApprovalGate        // optional — nil = auto-approve every tool
	resolver    clientResolver      // optional — nil = always use the default model
	todos       todoLister          // optional — nil = no state.snapshot{todos} projection

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
	mu              sync.Mutex
	turns           map[string]*turnState // turn_id → state
	closed          bool
	shutdownOnce    sync.Once
	shutdownTargets []*shutdownTarget

	// seenSessions tracks which sessions this process has already opened a turn
	// for, so the SessionStart hook fires once per session per process (not on
	// every turn). Guarded by mu.
	seenSessions map[string]struct{}
}

// shutdownTarget owns one turn's release operation. A successful attempt means
// the turn's complete ownership boundary closed, not merely that one Cancel call
// returned nil. Failed attempts remain retryable; an in-flight attempt is joined
// rather than duplicated.
type shutdownTarget struct {
	state *turnState
	step  *shutdown.Step
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

// BeginShutdown rejects future turns and starts cancellation for the complete
// live-turn set. The dispatcher, not the delivery run registry, is authoritative
// because parked turns remain live after their streaming segment has ended.
func (s *memoryDispatcher) BeginShutdown() {
	s.shutdownOnce.Do(func() {
		s.mu.Lock()
		s.closed = true
		states := slices.Collect(maps.Values(s.turns))
		slices.SortFunc(states, func(left, right *turnState) int {
			return cmp.Compare(left.handle.TurnID, right.handle.TurnID)
		})
		s.shutdownTargets = make([]*shutdownTarget, 0, len(states))
		for _, st := range states {
			state := st
			s.shutdownTargets = append(s.shutdownTargets, &shutdownTarget{
				state: state,
				step:  shutdown.New(func(ctx context.Context) error { return s.shutdownTurn(ctx, state) }),
			})
		}
		targets := slices.Clone(s.shutdownTargets)
		s.mu.Unlock()

		for _, target := range targets {
			// Broadcast the primary cancellation signal without waiting for
			// engine or storage I/O. AwaitShutdown starts and joins the bounded
			// release attempts after every component has received its signal.
			target.state.cancel()
		}
	})
}

// AwaitShutdown joins the turns cancelled by [BeginShutdown]. Its caller owns
// the deadline, so a timeout remains visible and a later await can finish the
// same shutdown rather than burying work behind a one-shot result.
func (s *memoryDispatcher) AwaitShutdown(ctx context.Context) error {
	if ctx == nil {
		return errors.New("turn: shutdown context is required")
	}
	s.BeginShutdown()
	attempts := make([]*shutdown.Attempt, 0, len(s.shutdownTargets))
	for _, target := range s.shutdownTargets {
		attempts = append(attempts, target.step.Begin(ctx))
	}
	for _, attempt := range attempts {
		if err := attempt.Wait(ctx); err != nil && ctx.Err() != nil {
			break
		}
	}
	if ctx.Err() != nil && shutdownRemaining(s.shutdownTargets) > 0 {
		return errors.Join(
			shutdownTimeoutError(s.shutdownTargets),
			shutdownAttemptErrors(s.shutdownTargets, attempts),
		)
	}
	return shutdownAttemptErrors(s.shutdownTargets, attempts)
}

// shutdownTurn keeps cancellation attached to lifecycle progress. In
// particular, a Restore that publishes its process after the first Cancel wakes
// this loop and makes the same shutdown owner retry against the now-actionable
// process.
func (s *memoryDispatcher) shutdownTurn(ctx context.Context, state *turnState) error {
	for {
		select {
		case <-state.done:
			return nil
		default:
		}
		changed := state.lifecycleChange()
		err := s.Cancel(ctx, state.handle)
		if errors.Is(err, ErrTurnNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		select {
		case <-state.done:
			return nil
		case <-changed:
		case <-ctx.Done():
			select {
			case <-state.done:
				return nil
			default:
				return ctx.Err()
			}
		}
	}
}

func shutdownTimeoutError(targets []*shutdownTarget) error {
	return fmt.Errorf("%w: %d turn(s) still shutting down", ErrShutdownTimeout, shutdownRemaining(targets))
}

func shutdownRemaining(targets []*shutdownTarget) int {
	remaining := 0
	for _, target := range targets {
		if !channelClosed(target.state.done) {
			remaining++
		}
	}
	return remaining
}

func shutdownAttemptErrors(targets []*shutdownTarget, attempts []*shutdown.Attempt) error {
	var errs []error
	for i, attempt := range attempts {
		err, complete := attempt.Result()
		if complete && err != nil && !errors.Is(err, ErrTurnNotFound) {
			errs = append(errs, fmt.Errorf(
				"turn: shut down turn %q: %w",
				targets[i].state.handle.TurnID,
				err,
			))
		}
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

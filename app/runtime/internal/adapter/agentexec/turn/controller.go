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

	apphooks "github.com/Tangerg/lynx/app/runtime/internal/application/hooks"
	"github.com/Tangerg/lynx/app/runtime/internal/component/shutdown"
	"github.com/Tangerg/lynx/app/runtime/internal/component/taskgroup"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/approval"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/mcpserver"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelref"
	"github.com/Tangerg/lynx/chatclient"
)

// chatResolver resolves a per-turn chat client for one explicit model
// selection. It is turn's own narrow dependency on the runtime model registry;
// success returns a non-nil client, while an unavailable provider or model is
// reported as an error.
type chatResolver interface {
	ResolveChat(ctx context.Context, selection modelref.Selection) (*chatclient.Client, error)
}

// ApprovalGate is the tool-call evaluator's complete approval view. Mode/rules
// management belongs to separate application and tool consumers.
type ApprovalGate interface {
	Mode(ctx context.Context, sessionID string) (approval.Mode, error)
	Decide(ctx context.Context, q approval.Query) (approval.Decision, bool, error)
	Remember(ctx context.Context, req approval.RememberRequest) error
}

// turnIDPrefix tags adapter-local turn handles. A TurnID is neither the stable
// domain RunID nor an executor process ID, so it has its own namespace.
const turnIDPrefix = "turn_"

func newTurnID() string { return turnIDPrefix + uuid.NewString() }

type hookResolver interface {
	For(ctx context.Context, cwd string) (*apphooks.Bound, error)
}

// Dependencies names the collaborators needed by the in-process controller.
// Engine and Approval are required; optional collaborators document their
// nil behavior on the field.
type Dependencies struct {
	// Engine starts or restores the Agent process tree. Required.
	Engine agentEngine

	// Steering persists queued messages that miss the current continuation
	// round. nil reports a steering error only when such a message exists.
	Steering SteeringSink

	// Maintenance performs best-effort post-turn housekeeping. nil disables the
	// complete maintenance sweep.
	Maintenance Maintenance

	// Approval gates every tool call using the owning session's effective
	// permission mode and remembered rules. Required.
	Approval ApprovalGate

	// ChatResolver resolves an explicit per-turn provider/model chat client. nil
	// supports only unset selections, which use the engine default client.
	ChatResolver chatResolver

	// ToolPresenter projects concrete tool activity and transcript results. nil
	// preserves canonical tool results and uses generic activity text.
	ToolPresenter ToolPresenter

	// ToolInterpreter interprets concrete calls for approval policy. Required.
	ToolInterpreter ToolInterpreter

	// MCPToolAutoApproved reports whether an identified MCP tool may skip the
	// approval prompt. nil disables MCP-specific auto-approval.
	MCPToolAutoApproved func(mcpserver.ToolRef) bool

	// Hooks resolves lifecycle hooks for a turn's cwd. nil disables hooks.
	Hooks hookResolver
}

// New builds the concrete in-process controller. The controller is
// single-process — it holds in-memory state about live turns and
// fans events out to subscribers via per-turn channels.
// Consumers define the narrow control ports they need; this controller exposes
// no request or presentation concerns.
func New(deps Dependencies) (*controller, error) {
	if deps.Engine == nil {
		return nil, errors.New("turn: engine is required")
	}
	if deps.Approval == nil {
		return nil, errors.New("turn: approval gate is required")
	}
	if deps.ToolInterpreter == nil {
		return nil, errors.New("turn: tool interpreter is required")
	}
	return &controller{
		engine:              deps.Engine,
		steering:            deps.Steering,
		maintenance:         deps.Maintenance,
		approval:            deps.Approval,
		chatResolver:        deps.ChatResolver,
		toolPresenter:       deps.ToolPresenter,
		toolInterpreter:     deps.ToolInterpreter,
		mcpToolAutoApproved: deps.MCPToolAutoApproved,
		hooks:               deps.Hooks,
		turns:               map[string]*turnState{},
		seenSessions:        map[string]struct{}{},
	}, nil
}

// controller is the single-process turn implementation. It
// tracks live turns in a map keyed by turn id; state lives in
// process memory and does not survive restart.
type controller struct {
	engine          agentEngine
	steering        SteeringSink
	maintenance     Maintenance // optional — nil = no Run-boundary maintenance
	approval        ApprovalGate
	chatResolver    chatResolver  // optional — nil accepts only the default model
	toolPresenter   ToolPresenter // optional — nil = generic activity and canonical results
	toolInterpreter ToolInterpreter

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
	cleanupTasks    taskgroup.Group

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

func (s *controller) register(st *turnState) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return false
	}
	s.turns[st.handle.TurnID] = st
	return true
}

func (s *controller) isClosed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed
}

// BeginShutdown rejects future turns and starts cancellation for the complete
// live-turn set. The controller is authoritative because parked Turns remain
// live after their streaming Segment has ended.
func (s *controller) BeginShutdown() {
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
		// Terminal cleanup is request-detached during normal operation. Shutdown
		// transfers that ownership to the bounded shutdown attempts below.
		s.cleanupTasks.Cancel()
	})
}

// AwaitShutdown joins the turns canceled by [BeginShutdown]. Its caller owns
// the deadline, so a timeout remains visible and a later await can finish the
// same shutdown rather than burying work behind a one-shot result.
func (s *controller) AwaitShutdown(ctx context.Context) error {
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
	return errors.Join(
		shutdownAttemptErrors(s.shutdownTargets, attempts),
		s.cleanupTasks.Wait(ctx),
	)
}

// shutdownTurn keeps cancellation attached to lifecycle progress. In
// particular, a Restore that publishes its process after the first Cancel wakes
// this loop and makes the same shutdown owner retry against the now-actionable
// process.
func (s *controller) shutdownTurn(ctx context.Context, state *turnState) error {
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

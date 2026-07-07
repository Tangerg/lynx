package turn

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/approval"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/hooks"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/todo"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel"
	corechat "github.com/Tangerg/lynx/core/model/chat"
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
// Engine is required; every other field is optional and has a nil-default
// behavior documented on the field.
type Dependencies struct {
	// Engine dispatches the underlying agent process. Required.
	Engine engineDep

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
//   - inmemory.go  — Dispatcher surface + live-turn registry (this file)
//   - turn.go      — per-turn state + the runTurn execution loop
//   - lifecycle.go — terminal-event capture from the agent runtime
//   - observer.go  — engine tool-observer → turn.Event translation
//
// The Dispatcher interface is stable, so transport adapters don't care
// which impl they talk to.
func New(deps Dependencies) (Dispatcher, error) {
	if deps.Engine == nil {
		return nil, errors.New("turn: engine is required")
	}
	return &inMemory{
		engine:         deps.Engine,
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
	engine   engineDep
	approval approval.Policy // optional — nil = auto-approve every tool
	resolver clientResolver  // optional — nil = always use the default model
	todos    todoLister      // optional — nil = no state.snapshot{todos} projection

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

// ------------------------------------------------------------------
// Dispatcher implementation
// ------------------------------------------------------------------

func (s *inMemory) StartTurn(ctx context.Context, req StartTurnRequest) (TurnHandle, error) {
	if req.SessionID == "" {
		return TurnHandle{}, errors.New("turn: SessionID is required")
	}
	if err := req.Validate(); err != nil {
		return TurnHandle{}, err
	}

	handle := TurnHandle{
		SessionID: req.SessionID,
		TurnID:    newTurnID(),
	}
	state := newTurnState(ctx, handle)
	state.model = modelOr(req.Model)
	state.cwd = req.Cwd
	// Open the turn span synchronously (before the goroutine launches and
	// before the handle is returned) so st.ctx carries it for every later
	// reader — runTurn, drive, resume, Cancel. The entry trace rode in via
	// newTurnState's WithoutCancel, so this span is its child.
	state.ctx, state.span = startTurnSpan(state.ctx, handle.SessionID, handle.TurnID, state.model)

	// Resolve this turn's lifecycle hooks (trust-filtered for the cwd). The
	// UserPromptSubmit / SessionStart hooks run BEFORE the turn launches so they
	// can inject context into the prompt or block it; a block ends the span we
	// just opened and fails the start.
	if s.hooks != nil {
		state.hooks = s.hooks.For(state.ctx, req.Cwd)
	}
	if !state.hooks.Empty() {
		msg, err := s.runPromptHooks(state.ctx, req, state)
		if err != nil {
			state.span.End()
			return TurnHandle{}, err
		}
		req.Message = msg
	}

	s.mu.Lock()
	s.turns[handle.TurnID] = state
	s.mu.Unlock()

	go s.runTurn(req, state)

	return handle, nil
}

// runPromptHooks fires SessionStart (once per session per process) + the
// UserPromptSubmit hook before a turn starts. It returns the (possibly
// context-prefixed) message, or an error wrapping [ErrPromptBlocked] when a hook
// blocked the prompt.
func (s *inMemory) runPromptHooks(ctx context.Context, req StartTurnRequest, st *turnState) (string, error) {
	var blocked bool
	var reason, inject string
	add := func(d hooks.Decision) {
		if d.Block && !blocked {
			blocked, reason = true, d.Reason
		}
		if d.InjectContext != "" {
			if inject != "" {
				inject += "\n"
			}
			inject += d.InjectContext
		}
	}
	if s.firstTurnForSession(req.SessionID) {
		add(st.hooks.Run(ctx, hooks.Input{Event: hooks.SessionStart, SessionID: req.SessionID, Cwd: req.Cwd}))
	}
	add(st.hooks.Run(ctx, hooks.Input{
		Event: hooks.UserPromptSubmit, SessionID: req.SessionID, Cwd: req.Cwd, Prompt: req.Message,
	}))
	if blocked {
		if reason == "" {
			reason = "blocked by a hook"
		}
		return "", fmt.Errorf("%w: %s", ErrPromptBlocked, reason)
	}
	if inject != "" {
		return "<hook-context>\n" + inject + "\n</hook-context>\n\n" + req.Message, nil
	}
	return req.Message, nil
}

// firstTurnForSession reports whether this is the first turn the process has
// opened for sessionID (and records it) — the SessionStart fire-once gate.
func (s *inMemory) firstTurnForSession(sessionID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.seenSessions[sessionID]; ok {
		return false
	}
	s.seenSessions[sessionID] = struct{}{}
	return true
}

// ForgetSession drops sessionID's SessionStart fire-once marker on session
// delete, so the gate set doesn't leak one entry per session over the process
// lifetime. See [Dispatcher.ForgetSession].
func (s *inMemory) ForgetSession(sessionID string) {
	s.mu.Lock()
	delete(s.seenSessions, sessionID)
	s.mu.Unlock()
}

// modelOr returns the model name for display / observability, falling
// back to "default" when the turn didn't pick one.
func modelOr(model string) string {
	if model == "" {
		return "default"
	}
	return model
}

// newTurnState builds a fresh per-turn state. Its lifetime ctx derives
// from the entry ctx via context.WithoutCancel: the caller's ctx ending
// (e.g. the StartTurn RPC returning) doesn't kill the in-flight turn —
// only [Dispatcher.Cancel] (st.cancel) does — yet the entry trace span is
// preserved, so the engine's spans chain onto the same trace. The turn
// span is layered on in StartTurn / Rehydrate. Shared by both entry
// points so they produce an identically-initialized turn.
func newTurnState(ctx context.Context, handle TurnHandle) *turnState {
	lifeCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	return &turnState{
		handle:    handle,
		events:    make(chan Event, 32),
		cancel:    cancel,
		ctx:       lifeCtx,
		startedAt: time.Now(),
	}
}

// findTurn looks up the per-turn state by id under the impl's
// mutex. Returns ErrTurnNotFound when the turn has already ended
// (runTurn deletes itself from the map on exit). Centralizes the
// "lock / lookup / unlock" sequence every public method below
// needs to perform.
func (s *inMemory) findTurn(id string) (*turnState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, ok := s.turns[id]
	if !ok {
		return nil, ErrTurnNotFound
	}
	return state, nil
}

func (s *inMemory) Events(ctx context.Context, handle TurnHandle) (iter.Seq[Event], error) {
	state, err := s.findTurn(handle.TurnID)
	if err != nil {
		return nil, err
	}
	// Single-consumer pull stream. The internal select multiplexes the
	// turn's event channel against ctx so the iterator stops promptly
	// when the caller stops listening — even while parked waiting for
	// the next event. runTurn closes state.events on turn end, which
	// terminates the range cleanly (ok == false).
	//
	// Consecutive text deltas (MessageDelta / ReasoningDelta) already buffered
	// on the channel are coalesced into one event before yielding. Under load —
	// the per-token LLM stream running ahead of the SSE consumer — this collapses
	// the 1-token-1-frame volume, cutting the hub's live-event drop rate
	// (hub.go), without touching the durable transcript (item.completed still
	// carries the full text) or adding latency: the drain is non-blocking, so a
	// trickling stream still yields each token the moment it arrives.
	return func(yield func(Event) bool) {
		var spill Event // a different-kind event pulled off mid-coalesce, yielded next
		recv := func() (Event, bool) {
			if spill != nil {
				ev := spill
				spill = nil
				return ev, true
			}
			select {
			case ev, ok := <-state.events:
				return ev, ok
			case <-ctx.Done():
				return nil, false
			}
		}
		for {
			ev, ok := recv()
			if !ok || !yield(coalesceTextDeltas(ev, state.events, &spill)) {
				return
			}
		}
	}, nil
}

// coalesceTextDeltas merges a run of same-kind text deltas (MessageDelta /
// ReasoningDelta) already buffered on ch into head, draining without blocking
// (the default branch = nothing more queued → stop). A different-kind event
// pulled off mid-drain is parked in *spill for the caller to yield next, so
// ordering is preserved. The merged event keeps head's BaseEvent — deltas are
// ephemeral (no SSE id, §5.2), so a merged delta's seq is immaterial.
func coalesceTextDeltas(head Event, ch <-chan Event, spill *Event) Event {
	switch h := head.(type) {
	case MessageDelta:
		for {
			select {
			case ev, ok := <-ch:
				if !ok {
					return h // channel closed; recv() sees the close next and stops
				}
				if d, same := ev.(MessageDelta); same {
					h.Text += d.Text
					continue
				}
				*spill = ev
			default:
			}
			return h
		}
	case ReasoningDelta:
		for {
			select {
			case ev, ok := <-ch:
				if !ok {
					return h
				}
				if d, same := ev.(ReasoningDelta); same {
					h.Text += d.Text
					continue
				}
				*spill = ev
			default:
			}
			return h
		}
	default:
		return head
	}
}

// InjectSteering queues message onto the active turn's pending
// steering buffer. The runtime flushes the queue to the chat history
// store after the turn ends — every queued message becomes a
// user-role entry in the conversation history that the next turn's
// chat history middleware loads on the next StartTurn.
//
// This is "next-turn" semantics — not true mid-stream injection.
// Steering you send while the model is mid-tool-loop affects the
// next turn, not the current one. Documented limitation; doing
// real mid-stream injection would require intercepting between
// rounds of the chat tool loop.
//
// Returns [ErrTurnNotFound] when the turn has already ended (its
// runTurn deleted itself from the map on exit). Empty messages
// are silently dropped.
func (s *inMemory) InjectSteering(_ context.Context, handle TurnHandle, message string) error {
	if message == "" {
		return nil
	}
	state, err := s.findTurn(handle.TurnID)
	if err != nil {
		return err
	}
	// Rejects with ErrTurnNotFound if the turn has closed its steering queue
	// (terminating) — same signal as a vanished turn, so SteerRun maps both to
	// run_not_found and the client retries as a fresh send.
	return state.appendSteering(message)
}

// Cancel stops a turn. The ctx cancel is the primary signal: it aborts any
// in-flight LLM stream (which reads ctx.Done()) and drives a RUNNING process's
// run loop to its own terminal via markCancelled — the single ProcessKilled
// publisher. KillProcess is reserved for a process that ISN'T looping
// (parked/suspended on a HITL interrupt, or not yet started): there's no loop
// to observe the ctx cancel, so it's terminated explicitly. Killing a Running
// process here instead would clobber its status — dropping a continuation a
// racing Resume just started (the approved tool never runs) — and publish a
// duplicate ProcessKilled alongside markCancelled.
func (s *inMemory) Cancel(_ context.Context, handle TurnHandle) error {
	state, err := s.findTurn(handle.TurnID)
	if err != nil {
		return err
	}
	state.cancel()
	// Claim the parked flag so a racing Resume can't also act on the same
	// suspended turn (whoever flips it false wins).
	proc := state.process()
	claimed := state.claimPark()
	if proc != nil && proc.Status() != core.StatusRunning {
		// Not actively looping (parked / not-yet-started): the ctx cancel
		// won't drive it to a terminal, so kill it explicitly.
		_ = proc.Cancel()
	}
	if claimed {
		// The turn was parked on an interrupt — no drive goroutine is
		// waiting on it, so emit the terminal + tear down here.
		s.finishTurn(state, TurnEndCanceled)
	}
	return nil
}

// Resume answers a turn parked on a HITL interrupt (tool approval or
// plan review). It claims the parked flag (so a racing Cancel can't
// double-act), delivers the bool decision to the agent process, and
// drives the continuation segment onto the same event channel. Returns
// [ErrTurnNotFound] when the turn isn't parked (unknown / already
// resumed / terminal).
func (s *inMemory) Resume(_ context.Context, handle TurnHandle, resolution interrupts.Resolution) error {
	state, err := s.findTurn(handle.TurnID)
	if err != nil {
		return err
	}
	if !state.claimPark() {
		// The turn exists but its park was already claimed — a concurrent Cancel
		// is finishing it. Report it distinctly from ErrTurnNotFound (turn gone /
		// restart) so the caller doesn't rehydrate and resurrect a canceled turn.
		return ErrParkClaimed
	}
	return s.resumeAndDrive(state, resolution)
}

// resumeAndDrive delivers the decision to the turn's (write-once-stable)
// parked process and launches the continuation drive. On a resume error it
// streams the terminal (ErrorEvent + TurnEnd) and returns the error;
// otherwise it starts drive and returns nil. Shared by [Resume]
// (same-process) and [Rehydrate] (cross-restart) so the resume tail —
// deliver, on-error-finish, else-drive — stays identical.
func (s *inMemory) resumeAndDrive(state *turnState, resolution interrupts.Resolution) error {
	resumed, err := state.process().Resume(state.ctx, resolution)
	if err != nil {
		s.emit(state, ErrorEvent{Message: err.Error(), Code: "ENGINE_ERROR"})
		s.finishTurn(state, TurnEndErrored)
		return err
	}
	go s.drive(state, resumed)
	return nil
}

// SetInterruptKinds records the HITL kinds the connected client can
// answer (from ClientCapabilities.InterruptKinds, negotiated at
// runtime.initialize). Passing an empty slice gates every kind; never
// calling it leaves the permissive default (surface all). Single-tenant:
// one client's negotiation applies process-wide.
func (s *inMemory) SetInterruptKinds(kinds []string) {
	set := make(map[string]bool, len(kinds))
	for _, k := range kinds {
		set[k] = true
	}
	s.mu.Lock()
	s.interruptKinds = set
	s.mu.Unlock()
}

// canSurface reports whether a turn may park on an interrupt of kind —
// true when no allowlist is configured (permissive default) or kind is in
// it. A false result means the client can't answer this kind, so the turn
// auto-denies instead of deadlocking.
func (s *inMemory) canSurface(kind string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.interruptKinds == nil {
		return true
	}
	return s.interruptKinds[kind]
}

// ProcessID returns the agent-process id backing a live turn — the
// snapshot key the runtime persists so a restart can rebuild the process
// via [Rehydrate]. Returns [ErrTurnNotFound] when the turn isn't live.
func (s *inMemory) ProcessID(_ context.Context, handle TurnHandle) (string, error) {
	state, err := s.findTurn(handle.TurnID)
	if err != nil {
		return "", err
	}
	proc := state.process()
	if proc == nil {
		return "", errors.New("turn: turn has not dispatched a process yet")
	}
	return proc.ID(), nil
}

// Rehydrate rebuilds a turn from a persisted process snapshot and resumes
// it — the cross-restart counterpart to [Resume]. It registers a fresh
// turn (new handle), restores + re-parks the agent process via
// [kernel.RestoreTurn] with a fresh observer + lifecycle listener, then
// delivers the decision and drives the continuation onto the new turn's
// event channel. The caller subscribes via [Events] on the returned handle.
func (s *inMemory) Rehydrate(ctx context.Context, req RehydrateRequest) (TurnHandle, error) {
	if req.ProcessID == "" {
		return TurnHandle{}, errors.New("turn: ProcessID is required")
	}
	handle := TurnHandle{SessionID: req.SessionID, TurnID: newTurnID()}
	state := newTurnState(ctx, handle)
	// Re-resolve the parked run's per-run client from the persisted
	// provider+model so the continuation runs against the SAME model (mirrors
	// the StartTurn path). No selection / no resolver / a provider since removed
	// → nil client = platform default, and the span records "default".
	var client *corechat.Client
	if req.Provider != "" && req.Model != "" && s.resolver != nil {
		c, err := s.resolver.ResolveClient(state.ctx, req.Provider, req.Model)
		if err != nil {
			state.cancel()
			return TurnHandle{}, err
		}
		client = c
		state.model = req.Model
	} else {
		state.model = "default"
	}
	state.ctx, state.span = startTurnSpan(state.ctx, handle.SessionID, handle.TurnID, state.model)
	observer := &turnObserver{svc: s, st: state}
	state.lifecycle = &turnLifecycle{}

	proc, err := s.engine.RestoreTurn(state.ctx, req.ProcessID, kernel.RestoreTurnRequest{
		SessionID:     req.SessionID,
		Observer:      observer,
		EventListener: state.lifecycle.listener(handle.TurnID),
		ChatClient:    client,
	})
	if err != nil {
		state.cancel()
		return TurnHandle{}, err
	}
	state.lifecycle.setRoot(proc.ID())
	state.setProc(proc)

	s.mu.Lock()
	s.turns[handle.TurnID] = state
	s.mu.Unlock()

	// The restored process is re-parked (RestoreTurn re-ticked it). Deliver the
	// decision and drive the continuation. On a resume error resumeAndDrive has
	// already torn the turn down (finishTurn), so there is no live turn for the
	// caller to subscribe to — return the error rather than a handle to a dead
	// turn (ResumeRun maps it to run_not_found instead of leaking ErrTurnNotFound
	// when its openSegment then can't find the turn). A nil error means the
	// continuation is driving and the caller subscribes via [Events].
	if err := s.resumeAndDrive(state, interrupts.Resolution{Approved: req.Approved}); err != nil {
		return TurnHandle{}, err
	}
	return handle, nil
}

// emit stamps the event with the next sequence number and timestamp
// and pushes it onto the turn's channel. Type-specific stamping
// lives on each concrete event (via the unexported [Event.stamp]
// method) so this dispatcher stays open-closed — adding a new
// event variant means writing the struct + one stamp method,
// nothing here.
//
// Sends block when the consumer falls behind: the durable history
// (items.list) is built from this stream, so backpressure — the turn
// slowing to the consumer's persistence speed — is correct where
// dropping would silently corrupt persisted items (a lost
// MessageDelta truncates the item text; a lost TurnEnd misreports the
// outcome as canceled). The turn-lifetime ctx is the escape hatch: a
// canceled turn stops blocking producers even when no consumer is
// left to drain.
func (s *inMemory) emit(st *turnState, ev Event) {
	stamped := ev.stamp(BaseEvent{
		SessionID: st.handle.SessionID,
		TurnID:    st.handle.TurnID,
		Seq:       st.seq.Add(1),
		Timestamp: time.Now(),
	})
	// Prefer delivery: when the buffer has room the event lands regardless of
	// whether the turn ctx was already canceled. This is what makes a canceled
	// turn's TERMINAL event (TurnEnd / the ErrorEvent before it) reach a
	// consumer still draining the stream — Cancel cancels st.ctx *before* the
	// finishTurn / drive path emits the terminal, so a bare select would race
	// the terminal into the ctx.Done() escape and drop it (a lost TurnEnd
	// misreports the outcome as canceled, or as no end at all). A keeping-up
	// consumer has drained the buffer by terminal time, so the fast path lands
	// it; only a backed-up buffer falls through to the escape below.
	select {
	case st.events <- stamped:
		return
	default:
	}
	// Buffer full: block until the consumer drains, or bail when the turn ctx
	// is canceled so a producer never wedges on an abandoned channel.
	select {
	case st.events <- stamped:
	case <-st.ctx.Done():
	}
}

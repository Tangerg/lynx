package chat

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"

	"github.com/Tangerg/lynx/lyra/internal/engine"
)

// New returns the M1 [Service] implementation. The implementation
// is single-process — it holds in-memory state about live turns and
// fans events out to subscribers via per-turn channels.
//
// Future milestones extend this: session-store backing, multi-client
// event fan-out, plan-mode pause/resume, etc. The Service interface
// is stable, so transport adapters (M8+) don't care which impl they
// talk to.
func New(eng *engine.Engine) Service {
	if eng == nil {
		panic("chat: engine is required")
	}
	return &impl{engine: eng, turns: map[string]*turnState{}}
}

// turnState holds the per-turn bookkeeping the implementation needs:
// the event channel subscribers read from, the cancel func that fires
// when [Service.Cancel] is called, and a monotone sequence number
// stamped onto every emitted event.
type turnState struct {
	handle TurnHandle
	events chan Event
	cancel context.CancelFunc
	seq    atomic.Uint64
}

type impl struct {
	engine *engine.Engine

	mu    sync.Mutex
	turns map[string]*turnState // turn_id → state
}

// ------------------------------------------------------------------
// Service implementation
// ------------------------------------------------------------------

func (s *impl) StartTurn(ctx context.Context, req StartTurnRequest) (TurnHandle, error) {
	if req.SessionID == "" {
		return TurnHandle{}, errors.New("chat: SessionID is required")
	}
	if req.Message == "" {
		return TurnHandle{}, errors.New("chat: Message must not be empty")
	}

	handle := TurnHandle{
		SessionID: req.SessionID,
		TurnID:    uuid.NewString(),
	}

	// Cancellation is per-turn — derive from a background ctx so the
	// caller's ctx ending (e.g. the StartTurn RPC returning) doesn't
	// kill the in-flight turn.
	turnCtx, cancel := context.WithCancel(context.Background())

	state := &turnState{
		handle: handle,
		events: make(chan Event, 32),
		cancel: cancel,
	}

	s.mu.Lock()
	s.turns[handle.TurnID] = state
	s.mu.Unlock()

	go s.runTurn(turnCtx, state, req)

	return handle, nil
}

func (s *impl) Events(_ context.Context, handle TurnHandle) (<-chan Event, error) {
	s.mu.Lock()
	state, ok := s.turns[handle.TurnID]
	s.mu.Unlock()
	if !ok {
		return nil, ErrTurnNotFound
	}
	return state.events, nil
}

func (s *impl) InjectSteering(_ context.Context, _ TurnHandle, _ string) error {
	// M1 leaves steering as a stub — surface stable so transport
	// adapters can call it; impl arrives with M3+ when multi-turn
	// + session persistence land.
	return errors.New("chat: steering not implemented in M1")
}

func (s *impl) Cancel(_ context.Context, handle TurnHandle) error {
	s.mu.Lock()
	state, ok := s.turns[handle.TurnID]
	s.mu.Unlock()
	if !ok {
		return ErrTurnNotFound
	}
	state.cancel()
	return nil
}

// ------------------------------------------------------------------
// Turn execution
// ------------------------------------------------------------------

// runTurn drives one turn from start to finish, emitting events as
// it goes. It always closes the event channel and clears the turn
// from the in-memory map so subsequent [Events] / [Cancel] return
// ErrTurnNotFound.
func (s *impl) runTurn(ctx context.Context, st *turnState, req StartTurnRequest) {
	defer func() {
		close(st.events)
		s.mu.Lock()
		delete(s.turns, st.handle.TurnID)
		s.mu.Unlock()
	}()

	startedAt := time.Now()
	s.emit(st, TurnStart{
		BaseEvent: st.baseEvent(),
		Model:     "default", // M1 — engine exposes model name in M2+
	})

	reply, runErr := s.engine.RunChat(ctx, req.Message)
	if runErr != nil {
		// Honour cancellation differently from genuine errors so
		// transport adapters can render the right state.
		if errors.Is(ctx.Err(), context.Canceled) {
			s.emit(st, TurnEnd{
				BaseEvent: st.baseEvent(),
				Reason:    TurnEndCancelled,
				Duration:  time.Since(startedAt),
			})
			return
		}
		s.emit(st, ErrorEvent{
			BaseEvent: st.baseEvent(),
			Message:   runErr.Error(),
			Code:      "ENGINE_ERROR",
		})
		s.emit(st, TurnEnd{
			BaseEvent: st.baseEvent(),
			Reason:    TurnEndErrored,
			Duration:  time.Since(startedAt),
		})
		return
	}

	// M1 emits the full reply as one MessageDelta. M2 will replace
	// this with real chunk streaming via lynx event bus.
	s.emit(st, MessageDelta{
		BaseEvent: st.baseEvent(),
		Text:      reply,
	})

	s.emit(st, TurnEnd{
		BaseEvent: st.baseEvent(),
		Reason:    TurnEndCompleted,
		Duration:  time.Since(startedAt),
		// TokenUsage / CostUSD wired up in M5 when invocation history
		// per-turn aggregation lands.
	})
}

// emit drops one event on the turn's channel. The send is non-blocking
// for cancellation safety — if the receiver has fallen behind we drop
// the event rather than block the turn forever.
func (s *impl) emit(st *turnState, ev Event) {
	switch e := ev.(type) {
	case TurnStart:
		ev = withSeq(st, e)
	case MessageDelta:
		ev = withSeq(st, e)
	case ToolCallStart:
		ev = withSeq(st, e)
	case ToolCallEnd:
		ev = withSeq(st, e)
	case TurnEnd:
		ev = withSeq(st, e)
	case ErrorEvent:
		ev = withSeq(st, e)
	default:
		panic(fmt.Sprintf("chat: unknown event type %T", ev))
	}

	select {
	case st.events <- ev:
	default:
		// Drop — subscriber is too slow. Future enhancement: buffered
		// outbox with metric counter so we can spot slow clients.
	}
}

// withSeq is the type-aware seq stamping helper. Each concrete event
// is a value type; we rewrite the BaseEvent's Seq before sending.
func withSeq(st *turnState, ev Event) Event {
	seq := st.seq.Add(1)
	now := time.Now()

	switch e := ev.(type) {
	case TurnStart:
		e.BaseEvent.Seq = seq
		e.BaseEvent.Timestamp = now
		return e
	case MessageDelta:
		e.BaseEvent.Seq = seq
		e.BaseEvent.Timestamp = now
		return e
	case ToolCallStart:
		e.BaseEvent.Seq = seq
		e.BaseEvent.Timestamp = now
		return e
	case ToolCallEnd:
		e.BaseEvent.Seq = seq
		e.BaseEvent.Timestamp = now
		return e
	case TurnEnd:
		e.BaseEvent.Seq = seq
		e.BaseEvent.Timestamp = now
		return e
	case ErrorEvent:
		e.BaseEvent.Seq = seq
		e.BaseEvent.Timestamp = now
		return e
	}
	return ev
}

func (st *turnState) baseEvent() BaseEvent {
	return BaseEvent{
		SessionID: st.handle.SessionID,
		TurnID:    st.handle.TurnID,
	}
}

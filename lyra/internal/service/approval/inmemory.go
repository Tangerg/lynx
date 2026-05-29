package approval

import (
	"context"
	"sync"
	"sync/atomic"
)

// New returns the single-process in-memory [Service]. Initial
// stance comes from mode — pass [ModeYolo] for environments where
// every tool call auto-passes (CI, smoke tests); production
// callers typically wire [ModeBalanced] or [ModeSafe].
//
// The implementation keeps pending requests in a map keyed by
// request id and uses one buffered channel per request so
// [Console.Decide] never blocks regardless of producer
// scheduling. All methods are safe for concurrent use.
func New(mode Mode) Service {
	s := &inMemory{pending: map[string]*pendingRequest{}}
	s.mode.Store(int32(mode))
	return s
}

// pendingRequest pairs the public [Request] with the buffered
// channel [Console.Decide] pushes onto. The channel is buffered
// cap-1 so a Decide call never blocks even if the Request
// goroutine has already given up on ctx.
type pendingRequest struct {
	req      Request
	decision chan Decision
}

// inMemory is the in-process Service implementation. Mode is held
// in an atomic.Int32 so [Console.GetMode] / [Console.SetMode] never
// contend with the pending-map mutex.
type inMemory struct {
	mode atomic.Int32

	mu      sync.Mutex
	pending map[string]*pendingRequest
}

// ListPending returns a copy of the pending requests in arbitrary
// (map-iteration) order. Caller mutation is safe — the slice and
// its elements are detached from the internal state.
func (s *inMemory) ListPending(_ context.Context) ([]Request, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Request, 0, len(s.pending))
	for _, p := range s.pending {
		out = append(out, p.req)
	}
	return out, nil
}

// Decide resolves the pending request at requestID by pushing
// decision onto its channel. Unknown id → [ErrRequestNotFound].
// The pending entry is removed by the Request goroutine when it
// observes the decision, so a second Decide on the same id also
// returns ErrRequestNotFound.
func (s *inMemory) Decide(_ context.Context, requestID string, decision Decision) error {
	s.mu.Lock()
	p, ok := s.pending[requestID]
	s.mu.Unlock()
	if !ok {
		return ErrRequestNotFound
	}
	// Non-blocking send: channel is buffered cap-1 and only one
	// Decide can succeed per request (subsequent senders fall through
	// the default and find the entry gone after the receiver clears it).
	select {
	case p.decision <- decision:
		return nil
	default:
		return ErrRequestNotFound
	}
}

func (s *inMemory) SetMode(_ context.Context, mode Mode) error {
	s.mode.Store(int32(mode))
	return nil
}

func (s *inMemory) GetMode(_ context.Context) (Mode, error) {
	return Mode(s.mode.Load()), nil
}

// Register declares req as pending and returns the channel its
// decision arrives on plus a cleanup func the caller must invoke
// to drop the entry. The split — register, then emit the user
// event, then wait on the channel — exists so producers can
// avoid the race where [Decide] is called before the pending
// entry is observable to [ListPending].
//
// Empty req.ID returns a closed channel sending [DecisionDeny] so
// callers can treat the error path the same as a normal Deny.
func (s *inMemory) Register(req Request) (<-chan Decision, func()) {
	if req.ID == "" {
		// Closed channel returns the zero value (DecisionApprove)
		// immediately — not desirable. We return a buffered channel
		// pre-loaded with Deny so the caller's select picks it up.
		ch := make(chan Decision, 1)
		ch <- DecisionDeny
		return ch, func() {}
	}
	pending := &pendingRequest{
		req:      req,
		decision: make(chan Decision, 1),
	}
	s.mu.Lock()
	s.pending[req.ID] = pending
	s.mu.Unlock()
	cleanup := func() {
		s.mu.Lock()
		delete(s.pending, req.ID)
		s.mu.Unlock()
	}
	return pending.decision, cleanup
}

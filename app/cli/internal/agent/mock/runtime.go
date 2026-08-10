package mock

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
)

var errCanceled = errors.New("mock: run canceled")

const (
	defaultPageSize = 20
	maximumPageSize = 100
)

// FaultKind identifies one transport fault injected into the next run
// subscription. Faults alter delivery only; the durable session log remains
// intact so clients can prove their replay and recovery behavior.
type FaultKind string

const (
	FaultDisconnect FaultKind = "disconnect"
	FaultDuplicate  FaultKind = "duplicate"
	FaultGap        FaultKind = "gap"
	FaultConflict   FaultKind = "conflict"
)

// SubscriptionFault is consumed by one FollowRun call. After is the one-based
// delivery position at which the fault occurs; values below one mean the first
// event.
type SubscriptionFault struct {
	Kind  FaultKind
	After int
}

// Runtime is a complete in-memory runtime adapter. Runs outlive individual
// subscriptions, event logs are replayable, and every operation is safe for
// concurrent use so the mock exercises the same lifecycle as a remote adapter.
type Runtime struct {
	Instant bool
	Script  func(prompt string) Script
	Faults  []SubscriptionFault

	mu       sync.Mutex
	sessions map[string]*sessionState
	runs     map[string]*runState
	starts   map[string]*startAttempt
	canceled map[string]struct{}
	rules    []agent.ApprovalRule
	fault    int
	next     uint64
	now      func() time.Time
}

type sessionState struct {
	meta     agent.Session
	events   []agent.Envelope
	active   string
	starting *startAttempt
	changed  chan struct{}
}

type startAttempt struct {
	input    agent.StartRun
	ready    chan struct{}
	run      agent.Run
	err      error
	finished bool
}

type runState struct {
	id           string
	sessionID    string
	startedAfter agent.Cursor
	status       agent.RunStatus
	script       Script
	interaction  agent.Interaction
	start        agent.StartRun
	answers      map[string]agent.Answer
	resuming     *resumeAttempt
	cancel       chan struct{}
	cancelOnce   sync.Once
	finishOnce   sync.Once
}

type resumeAttempt struct {
	interruptID string
	answer      agent.Answer
	ready       chan struct{}
	err         error
}

func New() *Runtime {
	r := &Runtime{
		sessions: make(map[string]*sessionState),
		runs:     make(map[string]*runState),
		starts:   make(map[string]*startAttempt),
		canceled: make(map[string]struct{}),
		now:      time.Now,
	}
	for _, session := range demoSessions() {
		r.sessions[session.ID] = &sessionState{meta: session, changed: make(chan struct{})}
	}
	r.seedHistory()
	return r
}

var _ agent.Runtime = (*Runtime)(nil)

func (r *Runtime) ListModels(ctx context.Context) ([]agent.Model, error) {
	if err := context.Cause(ctx); err != nil {
		return nil, err
	}
	return []agent.Model{
		{ID: "mock-balanced", DisplayName: "Mock Balanced", Description: "Synthetic balanced coding model", Default: true, Efforts: []string{"low", "medium", "high"}, Context: 200_000},
		{ID: "mock-fast", DisplayName: "Mock Fast", Description: "Synthetic low-latency model", Efforts: []string{"low", "medium"}, Context: 128_000},
		{ID: "mock-deep", DisplayName: "Mock Deep", Description: "Synthetic deep-reasoning model", Efforts: []string{"medium", "high", "max"}, Context: 400_000},
	}, nil
}

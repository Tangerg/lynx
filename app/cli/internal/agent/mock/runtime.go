package mock

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
)

var errCanceled = errors.New("mock: run canceled")

const (
	defaultPageSize = 20
	maxPageSize     = 100
)

type FaultKind string

const (
	FaultDisconnect FaultKind = "disconnect"
	FaultDuplicate  FaultKind = "duplicate"
	FaultConflict   FaultKind = "conflict"
)

type SubscriptionFault struct {
	Kind  FaultKind
	After int
}

// Runtime is an in-memory implementation of the CLI's consumer port. It models
// stable Runs, per-resume Segments, opaque event IDs, authoritative cold reads,
// and complete interrupt sets independently from any delivery transport.
type Runtime struct {
	Instant bool
	Script  func(prompt string) Script
	Faults  []SubscriptionFault

	mu           sync.Mutex
	sessions     map[string]*sessionState
	runs         map[string]*runState
	rules        []storedRule
	approvalMode agent.ApprovalMode
	fault        int
	next         uint64
	now          func() time.Time
}

type sessionState struct {
	meta         agent.Session
	items        []durableItem
	plan         []agent.PlanItem
	planRevision uint64
	planAtRun    map[string][]agent.PlanItem
	runs         []string
	active       string
}

type durableItem struct {
	runID string
	block agent.Block
}

type storedRule struct {
	view      agent.ApprovalRule
	sessionID string
}

type runState struct {
	id           string
	sessionID    string
	provider     string
	model        string
	limits       agent.RunLimits
	status       agent.RunStatus
	active       string
	segments     map[string]*segmentState
	script       Script
	interactions []agent.Interaction
	answers      map[string]agent.Answer
	cancel       chan struct{}
	cancelOnce   sync.Once
	finishOnce   sync.Once
	usage        agent.Usage
	outcome      agent.Outcome
}

type segmentState struct {
	id      string
	events  []agent.RunEvent
	changed chan struct{}
	closed  bool
}

func New() *Runtime {
	runtime := &Runtime{
		sessions:     make(map[string]*sessionState),
		runs:         make(map[string]*runState),
		approvalMode: agent.ApprovalModeBalanced,
		now:          time.Now,
	}
	for _, session := range demoSessions() {
		runtime.sessions[session.ID] = &sessionState{meta: session}
	}
	runtime.seedHistory()
	return runtime
}

var _ agent.Runtime = (*Runtime)(nil)

func (r *Runtime) ListModels(ctx context.Context) ([]agent.Model, error) {
	if err := context.Cause(ctx); err != nil {
		return nil, err
	}
	models := []agent.Model{
		{ID: "balanced", Provider: "mock", DisplayName: "Mock Balanced", ContextWindow: 200_000, MaxOutputTokens: 32_000, Capabilities: agent.ModelCapabilities{Reasoning: true, ReasoningLevels: []string{"low", "medium", "high"}, Multimodal: true, ToolUse: true}},
		{ID: "fast", Provider: "mock", DisplayName: "Mock Fast", ContextWindow: 128_000, MaxOutputTokens: 16_000, Capabilities: agent.ModelCapabilities{ToolUse: true}},
		{ID: "deep", Provider: "synthetic", DisplayName: "Synthetic Deep", ContextWindow: 400_000, MaxOutputTokens: 64_000, Capabilities: agent.ModelCapabilities{Reasoning: true, ReasoningLevels: []string{"medium", "high", "max"}, ToolUse: true}},
	}
	return models, nil
}

func (r *Runtime) GetApprovalMode(ctx context.Context) (agent.ApprovalMode, error) {
	if err := context.Cause(ctx); err != nil {
		return "", err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.approvalMode, nil
}

func (r *Runtime) SetApprovalMode(ctx context.Context, mode agent.ApprovalMode) (agent.ApprovalMode, error) {
	if err := context.Cause(ctx); err != nil {
		return "", err
	}
	if err := mode.Validate(); err != nil {
		return "", fmt.Errorf("mock: %w", err)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.approvalMode = mode
	return mode, nil
}

package approvals

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/Tangerg/lynx/app/runtime/internal/application/invalidation"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/approval"
)

var (
	ErrModeStoreUnavailable = errors.New("approvals: session mode store unavailable")
	ErrRuleStoreUnavailable = errors.New("approvals: rule store unavailable")
)

// New returns a [RuntimePolicy]: a validated runtime default mode, a durable
// session-mode store, and a persistent rule store. Pass [ModeYolo] for
// environments where every tool call auto-passes (CI, smoke tests). store may
// be nil for mode-only environments: Decide never matches and persistence
// operations return [ErrRuleStoreUnavailable]. modeStore may be nil when Plan
// mode is unavailable; ordinary calls still use the runtime default. ModePlan
// is rejected as a default because only a session may enter it.
func NewRuntimePolicy(
	mode approval.Mode,
	store RuleStore,
	modeStore ModeStore,
	invalidations invalidation.Publish,
) (*RuntimePolicy, error) {
	if !mode.ValidDefault() {
		return nil, fmt.Errorf("%w: %d", approval.ErrInvalidMode, mode)
	}
	p := &RuntimePolicy{store: store, modeStore: modeStore, invalidations: invalidations}
	p.mode.Store(int32(mode))
	return p, nil
}

// ModeStore persists explicit per-session permission state. Missing means use
// the runtime default. Implementations must return found=false for a missing
// session row and validate ownership at their persistence boundary.
type ModeStore interface {
	LookupMode(ctx context.Context, sessionID string) (state approval.SessionMode, found bool, err error)
	PutMode(ctx context.Context, sessionID string, state approval.SessionMode) error
}

// RuntimePolicy combines two policy facts consumed together at the tool-call
// boundary: session-effective permission mode and remembered approval rules.
// The default mode is atomic; Plan-mode transitions are serialized because
// they are rare state changes whose read/replace pair must be one process fact.
type RuntimePolicy struct {
	mode          atomic.Int32
	modeMu        sync.Mutex
	modeStore     ModeStore
	store         RuleStore
	invalidations invalidation.Publish
}

// DefaultMode returns the runtime fallback used by sessions without an explicit
// mode row.
func (p *RuntimePolicy) DefaultMode(_ context.Context) (approval.Mode, error) {
	mode := approval.Mode(p.mode.Load())
	if !mode.ValidDefault() {
		return 0, fmt.Errorf("%w: stored value %d", approval.ErrInvalidMode, mode)
	}
	return mode, nil
}

// SetDefaultMode changes the runtime fallback. Plan mode is session-only and is
// therefore rejected here.
func (p *RuntimePolicy) SetDefaultMode(_ context.Context, mode approval.Mode) error {
	if !mode.ValidDefault() {
		return fmt.Errorf("%w: %d", approval.ErrInvalidMode, mode)
	}
	p.mode.Store(int32(mode))
	p.invalidations.Notify(invalidation.Notice{Resource: invalidation.Approvals})
	return nil
}

// Mode returns the effective mode for sessionID. An empty id reads the runtime
// default; a session with no explicit row also inherits that default.
func (p *RuntimePolicy) Mode(ctx context.Context, sessionID string) (approval.Mode, error) {
	fallback, err := p.DefaultMode(ctx)
	if err != nil {
		return 0, err
	}
	if sessionID == "" || p.modeStore == nil {
		return fallback, nil
	}
	state, found, err := p.modeStore.LookupMode(ctx, sessionID)
	if err != nil {
		return 0, err
	}
	if !found {
		return fallback, nil
	}
	if err := state.Validate(); err != nil {
		return 0, err
	}
	return state.Mode, nil
}

// EnterPlanMode narrows one session to read-only and records the permission mode
// it must regain on exit. It returns changed=false when already active.
func (p *RuntimePolicy) EnterPlanMode(ctx context.Context, sessionID string) (changed bool, err error) {
	if sessionID == "" {
		return false, fmt.Errorf("%w: session id is required", approval.ErrInvalidSessionMode)
	}
	if p.modeStore == nil {
		return false, ErrModeStoreUnavailable
	}
	p.modeMu.Lock()
	defer p.modeMu.Unlock()

	mode, err := p.Mode(ctx, sessionID)
	if err != nil {
		return false, err
	}
	if mode == approval.ModePlan {
		return false, nil
	}
	state := approval.SessionMode{Mode: approval.ModePlan, RestoreMode: mode}
	if err := p.modeStore.PutMode(ctx, sessionID, state); err != nil {
		return false, err
	}
	return true, nil
}

// ExitPlanMode restores the exact mode captured by EnterPlanMode. It returns
// changed=false when the session is not in Plan mode.
func (p *RuntimePolicy) ExitPlanMode(ctx context.Context, sessionID string) (restored approval.Mode, changed bool, err error) {
	if sessionID == "" {
		return 0, false, fmt.Errorf("%w: session id is required", approval.ErrInvalidSessionMode)
	}
	if p.modeStore == nil {
		return 0, false, ErrModeStoreUnavailable
	}
	p.modeMu.Lock()
	defer p.modeMu.Unlock()

	state, found, err := p.modeStore.LookupMode(ctx, sessionID)
	if err != nil {
		return 0, false, err
	}
	if !found || state.Mode != approval.ModePlan {
		mode, modeErr := p.Mode(ctx, sessionID)
		return mode, false, modeErr
	}
	if err := state.Validate(); err != nil {
		return 0, false, err
	}
	restored = state.RestoreMode
	if err := p.modeStore.PutMode(ctx, sessionID, approval.SessionMode{Mode: restored}); err != nil {
		return 0, false, err
	}
	return restored, true, nil
}

func (p *RuntimePolicy) Decide(ctx context.Context, q approval.Query) (approval.Decision, bool, error) {
	if p.store == nil {
		return approval.Decide(nil, q)
	}
	candidates, err := p.store.Visible(ctx, q.SessionID, q.ProjectDir)
	if err != nil {
		return "", false, err
	}
	d, ok, err := approval.Decide(candidates, q)
	if err != nil {
		return "", false, err
	}
	return d, ok, nil
}

func (p *RuntimePolicy) Remember(ctx context.Context, req approval.RememberRequest) error {
	rule, err := req.Rule()
	if err != nil {
		return err
	}
	if p.store == nil {
		return ErrRuleStoreUnavailable
	}
	if err := p.store.Put(ctx, rule); err != nil {
		return err
	}
	p.invalidations.Notify(invalidation.Notice{Resource: invalidation.Approvals})
	return nil
}

func (p *RuntimePolicy) Rules(ctx context.Context, sessionID, projectDir string) ([]approval.Rule, error) {
	if p.store == nil {
		return nil, nil
	}
	rules, err := p.store.Visible(ctx, sessionID, projectDir)
	if err != nil {
		return nil, err
	}
	for index, rule := range rules {
		if err := rule.Validate(); err != nil {
			return nil, fmt.Errorf("approval: visible rule %d: %w", index, err)
		}
	}
	return rules, nil
}

func (p *RuntimePolicy) Forget(ctx context.Context, id string) error {
	if id == "" {
		return fmt.Errorf("%w: id is required", approval.ErrInvalidRule)
	}
	if p.store == nil {
		return ErrRuleStoreUnavailable
	}
	if err := p.store.Delete(ctx, id); err != nil {
		return err
	}
	p.invalidations.Notify(invalidation.Notice{Resource: invalidation.Approvals})
	return nil
}

package approval

import (
	"context"
	"fmt"
	"sync/atomic"
)

// New returns a [RuntimePolicy]: a validated mutable mode
// (lock-free atomic) plus a persistent rule store. Pass [ModeYolo] for
// environments where every tool call auto-passes (CI, smoke tests). store may
// be nil for mode-only environments: Decide never matches and persistence
// operations return [ErrRuleStoreUnavailable]. An unknown initial mode is
// rejected.
func New(mode Mode, store RuleStore) (*RuntimePolicy, error) {
	if !mode.Valid() {
		return nil, fmt.Errorf("%w: %d", ErrInvalidMode, mode)
	}
	p := &RuntimePolicy{store: store}
	p.mode.Store(int32(mode))
	return p, nil
}

// RuntimePolicy is the approval stance: an atomic mode + a rule store. The mode is
// read once per gated call (write-rare); rules go through the injected store.
type RuntimePolicy struct {
	mode  atomic.Int32
	store RuleStore
}

func (p *RuntimePolicy) Mode(_ context.Context) (Mode, error) {
	mode := Mode(p.mode.Load())
	if !mode.Valid() {
		return 0, fmt.Errorf("%w: stored value %d", ErrInvalidMode, mode)
	}
	return mode, nil
}

func (p *RuntimePolicy) SetMode(_ context.Context, mode Mode) error {
	if !mode.Valid() {
		return fmt.Errorf("%w: %d", ErrInvalidMode, mode)
	}
	p.mode.Store(int32(mode))
	return nil
}

func (p *RuntimePolicy) Decide(ctx context.Context, q Query) (Decision, bool, error) {
	if p.store == nil {
		return ruleSet(nil).decide(q)
	}
	candidates, err := p.store.Visible(ctx, q.SessionID, q.ProjectDir)
	if err != nil {
		return "", false, err
	}
	d, ok, err := ruleSet(candidates).decide(q)
	if err != nil {
		return "", false, err
	}
	return d, ok, nil
}

func (p *RuntimePolicy) Remember(ctx context.Context, req RememberRequest) error {
	rule, err := req.rule()
	if err != nil {
		return err
	}
	if p.store == nil {
		return ErrRuleStoreUnavailable
	}
	return p.store.Put(ctx, rule)
}

func (p *RuntimePolicy) Rules(ctx context.Context, sessionID, projectDir string) ([]Rule, error) {
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
		return fmt.Errorf("%w: id is required", ErrInvalidRule)
	}
	if p.store == nil {
		return ErrRuleStoreUnavailable
	}
	return p.store.Delete(ctx, id)
}

package approval

import (
	"context"
	"sync/atomic"
)

// New returns the runtime approval [Service]: a mutable mode (lock-free
// atomic) plus a persistent rule store. Pass [ModeYolo] for environments where
// every tool call auto-passes (CI, smoke tests). store may be nil — then no
// rules are remembered (Decide never matches, Remember is a no-op), which is
// the right shape for tests that exercise only mode gating.
func New(mode Mode, store RuleStore) Service {
	p := &policy{store: store}
	p.mode.Store(int32(mode))
	return p
}

// policy is the approval stance: an atomic mode + a rule store. The mode is
// read once per gated call (write-rare); rules go through the injected store.
type policy struct {
	mode  atomic.Int32
	store RuleStore
}

var _ Service = (*policy)(nil)

func (p *policy) Mode(_ context.Context) (Mode, error) {
	return Mode(p.mode.Load()), nil
}

func (p *policy) SetMode(_ context.Context, mode Mode) error {
	p.mode.Store(int32(mode))
	return nil
}

func (p *policy) Decide(ctx context.Context, q Query) (Decision, bool, error) {
	if p.store == nil {
		return "", false, nil
	}
	candidates, err := p.store.Visible(ctx, q.SessionID, q.ProjectDir)
	if err != nil {
		return "", false, err
	}
	d, ok := ruleSet(candidates).decide(q)
	return d, ok, nil
}

func (p *policy) Remember(ctx context.Context, req RememberRequest) error {
	if p.store == nil {
		return nil
	}
	rule, ok := req.rule()
	if !ok {
		return nil // can't key this scope (e.g. project rule with no cwd) — drop it
	}
	return p.store.Put(ctx, rule)
}

func (p *policy) Rules(ctx context.Context, sessionID, projectDir string) ([]Rule, error) {
	if p.store == nil {
		return nil, nil
	}
	return p.store.Visible(ctx, sessionID, projectDir)
}

func (p *policy) Forget(ctx context.Context, id string) error {
	if p.store == nil {
		return nil
	}
	return p.store.Delete(ctx, id)
}

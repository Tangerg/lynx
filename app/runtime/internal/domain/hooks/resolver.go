package hooks

import (
	"context"
	"sync"
)

// Resolver binds the hooks.json cascade to a working directory: it loads the
// global + project hooks for a cwd, drops the project hooks when the project
// isn't trusted (a cloned repo's hooks must not auto-run), and hands back a
// [Bound] the turn fires events on. Results are cached per cwd for the process
// (config is read once; a change needs a restart — same as AGENTS.md today).
type Resolver struct {
	home string
	// trusted reports whether a project root's project-scope hooks may run.
	// nil ⇒ project hooks are never trusted (global-only).
	trusted func(projectRoot string) bool
	runner  *Runner

	mu    sync.Mutex
	cache map[string][]Hook // cwd → discovered hooks (all scopes, untrust-filtered)
}

// NewResolver builds a Resolver. home is the user's home dir (for the global
// ~/.lyra/hooks.json); trusted gates project hooks (nil = global-only); onError
// is forwarded to the [Runner] for broken-hook reporting (may be nil).
func NewResolver(home string, trusted func(projectRoot string) bool, onError func(ctx context.Context, source string, err error)) *Resolver {
	return &Resolver{
		home:    home,
		trusted: trusted,
		runner:  NewRunner(onError),
		cache:   map[string][]Hook{},
	}
}

// For returns the Bound hooks for cwd — global hooks plus the project's hooks
// when the project is trusted. The loaded config is cached per cwd, but the
// TRUST check is re-evaluated every call, so a trust toggle (e.g. via the GUI)
// takes effect on the next turn with no cache invalidation. A nil Resolver
// returns a nil Bound (Run is a no-op), so callers needn't nil-check it.
func (r *Resolver) For(ctx context.Context, cwd string) *Bound {
	if r == nil || cwd == "" {
		return nil
	}
	all := r.load(ctx, cwd)
	projectTrusted := r.trusted != nil && r.trusted(ProjectRoot(cwd))
	kept := all[:0:0]
	for _, h := range all {
		if h.Scope == ScopeProject && !projectTrusted {
			continue // untrusted project: skip its hooks
		}
		kept = append(kept, h)
	}
	return &Bound{hooks: kept, runner: r.runner}
}

// load returns cwd's discovered hooks (all scopes, NOT trust-filtered), cached
// per cwd for the process — the file-I/O part of resolution. Trust filtering is
// applied fresh per call by For/Inspect.
func (r *Resolver) load(ctx context.Context, cwd string) []Hook {
	r.mu.Lock()
	if all, ok := r.cache[cwd]; ok {
		r.mu.Unlock()
		return all
	}
	r.mu.Unlock()

	all, err := Load(ctx, cwd, r.home, nil)
	if err != nil {
		return nil
	}
	r.mu.Lock()
	r.cache[cwd] = all
	r.mu.Unlock()
	return all
}

// Inspection is the read-only view of a cwd's hooks for a management surface
// (workspace.hooks.list): every discovered hook (trusted or not), the project
// root that gates the project-scope ones, and whether it's currently trusted.
type Inspection struct {
	ProjectRoot    string
	ProjectTrusted bool
	Hooks          []Hook
}

// Inspect returns the full discovered hook set for cwd plus the project's trust
// status — for a GUI to review hooks and decide whether to trust the project.
// Unlike For, it does NOT trust-filter (the GUI shows untrusted project hooks so
// the user can review them before trusting). Nil Resolver → empty Inspection.
func (r *Resolver) Inspect(ctx context.Context, cwd string) Inspection {
	if r == nil || cwd == "" {
		return Inspection{}
	}
	root := ProjectRoot(cwd)
	return Inspection{
		ProjectRoot:    root,
		ProjectTrusted: r.trusted != nil && r.trusted(root),
		Hooks:          r.load(ctx, cwd),
	}
}

// Bound is the resolved hook set for one cwd, ready to fire events.
type Bound struct {
	hooks  []Hook
	runner *Runner
}

// Run fires the bound hooks for in's event and returns the combined Decision.
// Nil-safe: a nil Bound (no resolver / empty config) returns the zero Decision,
// so every seam can call st.hooks.Run(...) unguarded.
func (b *Bound) Run(ctx context.Context, in Input) Decision {
	if b == nil || len(b.hooks) == 0 {
		return Decision{}
	}
	return b.runner.Run(ctx, b.hooks, in)
}

// Empty reports whether the Bound has no hooks (lets a seam skip building an
// Input payload when nothing would fire). Nil-safe.
func (b *Bound) Empty() bool { return b == nil || len(b.hooks) == 0 }

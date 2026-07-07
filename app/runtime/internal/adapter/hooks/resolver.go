package hooks

import (
	"context"
	"sync"

	domainhooks "github.com/Tangerg/lynx/app/runtime/internal/domain/hooks"
)

// Resolver binds the hooks.json cascade to a working directory: it loads the
// global + project hooks, drops project hooks when the project is not trusted,
// and returns a bound hook set for the turn to fire.
type Resolver struct {
	home string
	// trusted reports whether a project root's project-scope hooks may run.
	// nil means project hooks are never trusted.
	trusted func(context.Context, string) bool
	runner  *domainhooks.Runner

	mu    sync.Mutex
	cache map[string][]domainhooks.Hook
}

// NewResolver builds a Resolver. home is the user's home dir for the global
// hooks.json; trusted gates project hooks; onError reports broken commands.
func NewResolver(home string, trusted func(context.Context, string) bool, onError func(ctx context.Context, source string, err error)) *Resolver {
	return &Resolver{
		home:    home,
		trusted: trusted,
		runner:  domainhooks.NewRunner(Shell{}, onError),
		cache:   map[string][]domainhooks.Hook{},
	}
}

// For returns global hooks plus the project's hooks when the project is
// trusted. File discovery is cached per cwd; trust is rechecked every call.
func (r *Resolver) For(ctx context.Context, cwd string) *domainhooks.Bound {
	if r == nil || cwd == "" {
		return nil
	}
	all := r.load(ctx, cwd)
	projectTrusted := r.trusted != nil && r.trusted(ctx, ProjectRoot(cwd))
	kept := all[:0:0]
	for _, h := range all {
		if h.Scope == domainhooks.ScopeProject && !projectTrusted {
			continue
		}
		kept = append(kept, h)
	}
	return domainhooks.NewBound(kept, r.runner)
}

// Inspect returns every discovered hook plus the project trust state. It does
// not trust-filter, so management UIs can review hooks before granting trust.
func (r *Resolver) Inspect(ctx context.Context, cwd string) domainhooks.Inspection {
	if r == nil || cwd == "" {
		return domainhooks.Inspection{}
	}
	root := ProjectRoot(cwd)
	return domainhooks.Inspection{
		ProjectRoot:    root,
		ProjectTrusted: r.trusted != nil && r.trusted(ctx, root),
		Hooks:          r.load(ctx, cwd),
	}
}

func (r *Resolver) load(ctx context.Context, cwd string) []domainhooks.Hook {
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

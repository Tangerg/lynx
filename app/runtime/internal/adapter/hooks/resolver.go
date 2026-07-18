package hooks

import (
	"context"

	domainhooks "github.com/Tangerg/lynx/app/runtime/internal/domain/hooks"
)

// Resolver binds the hooks.json cascade to a working directory: it loads the
// global + project hooks, drops project hooks when the project is not trusted,
// and returns a bound hook set for the turn to fire.
type Resolver struct {
	home string
	// trusted reports whether a project root's project-scope hooks may run.
	// nil means project hooks are never trusted.
	trusted func(context.Context, string) (bool, error)
	runner  *domainhooks.Runner
}

// NewResolver builds a Resolver. home is the user's home dir for the global
// hooks.json; trusted gates project hooks; onError reports broken commands.
func NewResolver(home string, trusted func(context.Context, string) (bool, error), onError func(ctx context.Context, source string, err error)) *Resolver {
	return &Resolver{
		home:    home,
		trusted: trusted,
		runner:  domainhooks.NewRunner(Shell{}, onError),
	}
}

// For returns the current global hooks plus the project's hooks when the
// project is trusted. Discovery and trust are read on every call so edits and
// revocations take effect on the next turn without a process restart.
func (r *Resolver) For(ctx context.Context, cwd string) (*domainhooks.Bound, error) {
	if r == nil || cwd == "" {
		return nil, nil
	}
	root := ProjectRoot(cwd)
	projectTrusted := false
	var err error
	if r.trusted != nil {
		projectTrusted, err = r.trusted(ctx, root)
		if err != nil {
			return nil, err
		}
	}
	all, err := load(ctx, cwd, r.home, projectTrusted)
	if err != nil {
		return nil, err
	}
	return domainhooks.NewBound(all, r.runner), nil
}

// Inspect returns every discovered hook plus the project trust state. It does
// not trust-filter, so management UIs can review hooks before granting trust.
func (r *Resolver) Inspect(ctx context.Context, cwd string) (domainhooks.Inspection, error) {
	if r == nil || cwd == "" {
		return domainhooks.Inspection{}, nil
	}
	root := ProjectRoot(cwd)
	all, err := Load(ctx, cwd, r.home)
	if err != nil {
		return domainhooks.Inspection{}, err
	}
	projectTrusted := false
	if r.trusted != nil {
		projectTrusted, err = r.trusted(ctx, root)
		if err != nil {
			return domainhooks.Inspection{}, err
		}
	}
	return domainhooks.Inspection{
		ProjectRoot:    root,
		ProjectTrusted: projectTrusted,
		Hooks:          all,
	}, nil
}

// Package workspace is the application coordinator for the project-scoped
// developer-surface use cases: long-term memory (LYRA.md), skill + recipe
// discovery, and lifecycle-hook inspection/trust. It is a thin use-case layer
// over domain stores and filesystem-backed discovery ports; the delivery
// layer drives it per workspace request (cwd-scoped).
package workspace

import (
	"context"
	"errors"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/hooks"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/knowledge"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/recipes"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/skills"
)

// ErrMemoryUnavailable reports that this runtime was built without a knowledge store.
var ErrMemoryUnavailable = errors.New("workspace: memory unavailable")

// SkillCatalog enumerates the skills visible from a working directory (project
// over global). The composition root supplies promptsource-backed discovery.
type SkillCatalog interface {
	ListSkills(ctx context.Context, workdir string) ([]skills.Info, error)
}

// SkillCurator manages the global self-authored skill library: listing every
// skill with its lifecycle and moving one between active and archived (never
// deleting). The composition root supplies the file-backed authoring store; nil
// disables the management surface.
type SkillCurator interface {
	List(ctx context.Context) ([]skills.Entry, error)
	Archive(ctx context.Context, name string) error
	Restore(ctx context.Context, name string) error
}

// HookInspector resolves the lifecycle hooks discovered for a cwd plus the
// project's trust status.
type HookInspector interface {
	Inspect(ctx context.Context, cwd string) (hooks.Inspection, error)
}

// HookTrustStore mutates project hook trust (the workspace.hooks.setTrust
// surface). nil leaves trust read-only (CLI / file only).
type HookTrustStore interface {
	Trust(ctx context.Context, projectRoot string) error
	Untrust(ctx context.Context, projectRoot string) error
}

// RecipeLister discovers the prompt recipes visible from a working directory —
// a project's .lyra/recipes layered over the global directory. The composition
// root supplies the filesystem-backed implementation (the promptsource adapter);
// the port keeps the coordinator free of file I/O.
type RecipeLister interface {
	List(ctx context.Context, cwd string) ([]recipes.Recipe, error)
}

// Coordinator owns the workspace developer-surface use cases. Any of its
// dependencies may be nil, disabling the corresponding capability.
type Coordinator struct {
	memory  knowledge.Store
	skills  SkillCatalog
	curator SkillCurator
	hooks   HookInspector
	trust   HookTrustStore
	recipes RecipeLister
}

// Config bundles the Coordinator's dependencies.
type Config struct {
	Memory  knowledge.Store
	Skills  SkillCatalog
	Curator SkillCurator
	Hooks   HookInspector
	Trust   HookTrustStore
	// Recipes discovers the prompt recipes visible from a working directory. The
	// composition root supplies the filesystem-backed implementation; nil disables
	// recipe discovery (listRecipes returns empty).
	Recipes RecipeLister
}

// New returns a workspace Coordinator over cfg.
func New(cfg Config) *Coordinator {
	return &Coordinator{
		memory:  cfg.Memory,
		skills:  cfg.Skills,
		curator: cfg.Curator,
		hooks:   cfg.Hooks,
		trust:   cfg.Trust,
		recipes: cfg.Recipes,
	}
}

// HasMemory reports whether this runtime is backed by a long-term knowledge store.
func (c *Coordinator) HasMemory() bool { return c.memory != nil }

// ListMemoryEntries enumerates LYRA.md entries across scopes.
func (c *Coordinator) ListMemoryEntries(ctx context.Context, cwd string) ([]knowledge.Entry, error) {
	if c.memory == nil {
		return nil, ErrMemoryUnavailable
	}
	return c.memory.List(ctx, cwd)
}

// Memory returns the LYRA.md content for one scope.
func (c *Coordinator) Memory(ctx context.Context, scope knowledge.Scope, cwd string) (string, error) {
	if c.memory == nil {
		return "", ErrMemoryUnavailable
	}
	return c.memory.Get(ctx, scope, cwd)
}

// UpdateMemory overwrites the LYRA.md content for one scope.
func (c *Coordinator) UpdateMemory(ctx context.Context, scope knowledge.Scope, cwd string, content string) error {
	if c.memory == nil {
		return ErrMemoryUnavailable
	}
	return c.memory.Update(ctx, scope, cwd, content)
}

// ListSkills enumerates the skills visible from cwd (project over global) for
// workspace.listSkills.
func (c *Coordinator) ListSkills(ctx context.Context, cwd string) ([]skills.Info, error) {
	if c.skills == nil {
		return nil, nil
	}
	return c.skills.ListSkills(ctx, cwd)
}

// ListManagedSkills returns the global self-authored skill library — active and
// archived skills, each tagged with its lifecycle (workspace.skills.list). Empty
// when no authoring store is wired.
func (c *Coordinator) ListManagedSkills(ctx context.Context) ([]skills.Entry, error) {
	if c.curator == nil {
		return nil, nil
	}
	return c.curator.List(ctx)
}

// ArchiveSkill removes a skill from active use without deleting it
// (workspace.skills.archive). No-op when no authoring store is wired.
func (c *Coordinator) ArchiveSkill(ctx context.Context, name string) error {
	if c.curator == nil {
		return nil
	}
	return c.curator.Archive(ctx, name)
}

// RestoreSkill returns an archived skill to active use
// (workspace.skills.restore). No-op when no authoring store is wired.
func (c *Coordinator) RestoreSkill(ctx context.Context, name string) error {
	if c.curator == nil {
		return nil
	}
	return c.curator.Restore(ctx, name)
}

// ListRecipes enumerates the prompt recipes visible from cwd — project recipes
// (<cwd>/.lyra/recipes) layered over the global directory, project winning on a
// name collision (workspace.recipes.list).
func (c *Coordinator) ListRecipes(ctx context.Context, cwd string) ([]recipes.Recipe, error) {
	if c.recipes == nil {
		return nil, nil
	}
	return c.recipes.List(ctx, cwd)
}

// InspectHooks returns the lifecycle hooks discovered for cwd plus the project's
// trust status (workspace.hooks.list). Empty when hooks are unconfigured.
func (c *Coordinator) InspectHooks(ctx context.Context, cwd string) (hooks.Inspection, error) {
	if c.hooks == nil {
		return hooks.Inspection{}, nil
	}
	return c.hooks.Inspect(ctx, cwd)
}

// SetProjectHookTrust trusts (or revokes) a project's hooks (workspace.hooks.
// setTrust). No-op when no trust store is wired.
func (c *Coordinator) SetProjectHookTrust(ctx context.Context, projectRoot string, trusted bool) error {
	if c.trust == nil {
		return nil
	}
	if trusted {
		return c.trust.Trust(ctx, projectRoot)
	}
	return c.trust.Untrust(ctx, projectRoot)
}

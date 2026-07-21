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

// ErrSkillDraftsUnavailable reports that this runtime was built without a skill
// authoring store, so the offline draft-review surface is not negotiated.
// Delivery maps it to capability_not_negotiated.
var ErrSkillDraftsUnavailable = errors.New("workspace: skill drafts unavailable")

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

// SkillDrafts is the offline review surface for agent-mined skill proposals:
// enumerate the pending drafts, promote one into the active library, or discard
// (reject) it. The method set matches the file-backed authoring store the
// composition root supplies; nil disables the surface (the review methods report
// [ErrSkillDraftsUnavailable] rather than silently no-op, so the client can
// negotiate the capability off). Distinct from [SkillCurator] — reviewing
// proposals is a different capability from curating the active library.
type SkillDrafts interface {
	ListDrafts(ctx context.Context) ([]skills.DraftInfo, error)
	Promote(ctx context.Context, handle skills.DraftHandle) error
	DiscardDraft(ctx context.Context, handle skills.DraftHandle) error
}

// HookInspector resolves the lifecycle hooks discovered for a cwd plus the
// project's trust status.
type HookInspector interface {
	Inspect(ctx context.Context, cwd string) (hooks.Inspection, error)
}

// HookTrustStore mutates project hook trust (the hooks.setTrust
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
	drafts  SkillDrafts
	hooks   HookInspector
	trust   HookTrustStore
	recipes RecipeLister
}

// Config bundles the Coordinator's dependencies.
type Config struct {
	Memory  knowledge.Store
	Skills  SkillCatalog
	Curator SkillCurator
	Drafts  SkillDrafts
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
		drafts:  cfg.Drafts,
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
// skills.discovered.list.
func (c *Coordinator) ListSkills(ctx context.Context, cwd string) ([]skills.Info, error) {
	if c.skills == nil {
		return nil, nil
	}
	return c.skills.ListSkills(ctx, cwd)
}

// ListManagedSkills returns the global self-authored skill library — active and
// archived skills, each tagged with its lifecycle (skills.library.list). Empty
// when no authoring store is wired.
func (c *Coordinator) ListManagedSkills(ctx context.Context) ([]skills.Entry, error) {
	if c.curator == nil {
		return nil, nil
	}
	return c.curator.List(ctx)
}

// ArchiveSkill removes a skill from active use without deleting it
// (skills.library.archive). No-op when no authoring store is wired.
func (c *Coordinator) ArchiveSkill(ctx context.Context, name string) error {
	if c.curator == nil {
		return nil
	}
	return c.curator.Archive(ctx, name)
}

// RestoreSkill returns an archived skill to active use
// (skills.library.restore). No-op when no authoring store is wired.
func (c *Coordinator) RestoreSkill(ctx context.Context, name string) error {
	if c.curator == nil {
		return nil
	}
	return c.curator.Restore(ctx, name)
}

// ListSkillDrafts enumerates the agent-mined skill proposals awaiting offline
// review (skills.drafts.list). Reports [ErrSkillDraftsUnavailable] when no
// authoring store is wired.
func (c *Coordinator) ListSkillDrafts(ctx context.Context) ([]skills.DraftInfo, error) {
	if c.drafts == nil {
		return nil, ErrSkillDraftsUnavailable
	}
	return c.drafts.ListDrafts(ctx)
}

// PromoteSkillDraft publishes a reviewed draft into the active skill library
// (skills.drafts.promote). Reports [ErrSkillDraftsUnavailable] when no authoring
// store is wired.
func (c *Coordinator) PromoteSkillDraft(ctx context.Context, handle skills.DraftHandle) error {
	if c.drafts == nil {
		return ErrSkillDraftsUnavailable
	}
	return c.drafts.Promote(ctx, handle)
}

// RejectSkillDraft discards a reviewed draft (skills.drafts.reject). Reports
// [ErrSkillDraftsUnavailable] when no authoring store is wired.
func (c *Coordinator) RejectSkillDraft(ctx context.Context, handle skills.DraftHandle) error {
	if c.drafts == nil {
		return ErrSkillDraftsUnavailable
	}
	return c.drafts.DiscardDraft(ctx, handle)
}

// ListRecipes enumerates the prompt recipes visible from cwd — project recipes
// (<cwd>/.lyra/recipes) layered over the global directory, project winning on a
// name collision (recipes.list).
func (c *Coordinator) ListRecipes(ctx context.Context, cwd string) ([]recipes.Recipe, error) {
	if c.recipes == nil {
		return nil, nil
	}
	return c.recipes.List(ctx, cwd)
}

// InspectHooks returns the lifecycle hooks discovered for cwd plus the project's
// trust status (hooks.list). Empty when hooks are unconfigured.
func (c *Coordinator) InspectHooks(ctx context.Context, cwd string) (hooks.Inspection, error) {
	if c.hooks == nil {
		return hooks.Inspection{}, nil
	}
	return c.hooks.Inspect(ctx, cwd)
}

// SetProjectHookTrust trusts (or revokes) a project's hooks (hooks.
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

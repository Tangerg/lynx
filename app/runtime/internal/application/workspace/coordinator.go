// Package workspace contains focused project-scoped application use cases:
// workspace identity, file and VCS browsing, long-term memory (LYRA.md), skill
// and recipe discovery, lifecycle-hook inspection/trust, and Git-state
// subscriptions. Each use case takes only the port it consumes; delivery drives
// the relevant one per cwd-scoped request.
package workspace

import (
	"context"
	"errors"
	"io"

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

// ErrFileWatchUnavailable reports that this runtime has no workspace-change
// observer. A subscription without requested watches remains useful for
// application-published events; callers requesting Git-state watches receive
// this explicit capability failure instead of a silent, inert subscription.
var ErrFileWatchUnavailable = errors.New("workspace: file watch unavailable")

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

// GitStateWatcher observes the small set of Git metadata directories that
// signal a changed repository state. The adapter owns filesystem notification,
// debounce, repository layout, and goroutine lifetime; the application owns
// resolving requested workspace roots and exposes only a neutral resync
// callback. Closing the returned subscription stops all callbacks before it
// returns.
type GitStateWatcher interface {
	WatchGitState(roots []string, notify func()) (io.Closer, error)
}

// Context resolves the process-facing workspace identity shared by independent
// workspace use cases. It owns no feature adapter: each capability below takes
// this small context plus only the port it actually needs.
type Context struct {
	defaultCwd string
	home       string
	paths      Paths
}

// NewContext constructs the shared workspace identity resolver.
func NewContext(defaultCwd, home string, paths Paths) *Context {
	return &Context{defaultCwd: defaultCwd, home: home, paths: paths}
}

// Files owns root-scoped file browser operations.
type Files struct {
	context *Context
	files   FileBrowser
}

func NewFiles(context *Context, files FileBrowser) *Files {
	return &Files{context: context, files: files}
}

// VCS owns root-scoped Git status and diff operations.
type VCS struct {
	context *Context
	git     GitReader
}

func NewVCS(context *Context, git GitReader) *VCS { return &VCS{context: context, git: git} }

// Discovery owns project, recipe, and instruction-document discovery.
type Discovery struct {
	context   *Context
	projects  ProjectCatalog
	agentDocs AgentDocFinder
	recipes   RecipeLister
}

func NewDiscovery(context *Context, projects ProjectCatalog, agentDocs AgentDocFinder, recipes RecipeLister) *Discovery {
	return &Discovery{context: context, projects: projects, agentDocs: agentDocs, recipes: recipes}
}

// Knowledge owns the human-authored LYRA.md cascade use cases.
type Knowledge struct {
	context *Context
	memory  knowledge.Store
}

func NewKnowledge(context *Context, memory knowledge.Store) *Knowledge {
	return &Knowledge{context: context, memory: memory}
}

// Skills owns skill discovery, library curation, and proposal review.
type Skills struct {
	context       *Context
	skills        SkillCatalog
	curator       SkillCurator
	drafts        SkillDrafts
	skillsChanged func(struct{})
}

func NewSkills(context *Context, skills SkillCatalog, curator SkillCurator, drafts SkillDrafts, skillsChanged func(struct{})) *Skills {
	return &Skills{context: context, skills: skills, curator: curator, drafts: drafts, skillsChanged: skillsChanged}
}

// Hooks owns lifecycle-hook inspection and trust decisions.
type Hooks struct {
	context *Context
	hooks   HookInspector
	trust   HookTrustStore
}

// HookInspection is the workspace use case's resolved hook view. Active is
// business policy (global hooks always run; project hooks require trust), not
// a presentation decision for Delivery to reconstruct.
type HookInspection struct {
	ProjectRoot    string
	ProjectTrusted bool
	Hooks          []ResolvedHook
}

type ResolvedHook struct {
	Hook   hooks.Hook
	Active bool
}

func NewHooks(context *Context, hooks HookInspector, trust HookTrustStore) *Hooks {
	return &Hooks{context: context, hooks: hooks, trust: trust}
}

// GitWatch owns Git-state subscription setup over the technical watch adapter.
type GitWatch struct {
	context *Context
	watcher GitStateWatcher
}

func NewGitWatch(context *Context, watcher GitStateWatcher) *GitWatch {
	return &GitWatch{context: context, watcher: watcher}
}

// HasMemory reports whether this runtime is backed by a long-term knowledge store.
func (c *Knowledge) HasMemory() bool { return c != nil && c.memory != nil }

// HasFileWatch reports whether Git-state workspace subscriptions are wired.
func (c *GitWatch) HasFileWatch() bool { return c != nil && c.watcher != nil }

// WatchGitState resolves each requested working directory to its canonical
// workspace root, removes duplicate roots, then delegates technical watching to
// the configured adapter. It deliberately carries no delivery/protocol event
// type: any observed change means only "resync the workspace view".
func (c *GitWatch) WatchGitState(cwds []string, notify func()) (io.Closer, error) {
	if c.watcher == nil {
		return nil, ErrFileWatchUnavailable
	}
	seen := make(map[string]struct{}, len(cwds))
	roots := make([]string, 0, len(cwds))
	for _, cwd := range cwds {
		root, err := c.context.root(cwd)
		if err != nil {
			return nil, err
		}
		if _, duplicate := seen[root]; duplicate {
			continue
		}
		seen[root] = struct{}{}
		roots = append(roots, root)
	}
	return c.watcher.WatchGitState(roots, notify)
}

// ListMemoryEntries enumerates LYRA.md entries across scopes.
func (c *Knowledge) ListMemoryEntries(ctx context.Context, cwd string) ([]knowledge.Entry, error) {
	if c.memory == nil {
		return nil, ErrMemoryUnavailable
	}
	root, err := c.context.root(cwd)
	if err != nil {
		return nil, err
	}
	return c.memory.List(ctx, root)
}

// Memory returns the LYRA.md content for one scope.
func (c *Knowledge) Memory(ctx context.Context, scope knowledge.Scope, cwd string) (string, error) {
	if c.memory == nil {
		return "", ErrMemoryUnavailable
	}
	if scope == knowledge.ScopeUser {
		return c.memory.Get(ctx, scope, "")
	}
	root, err := c.context.root(cwd)
	if err != nil {
		return "", err
	}
	return c.memory.Get(ctx, scope, root)
}

// UpdateMemory overwrites the LYRA.md content for one scope.
func (c *Knowledge) UpdateMemory(ctx context.Context, scope knowledge.Scope, cwd string, content string) error {
	if c.memory == nil {
		return ErrMemoryUnavailable
	}
	if scope == knowledge.ScopeUser {
		return c.memory.Update(ctx, scope, "", content)
	}
	root, err := c.context.root(cwd)
	if err != nil {
		return err
	}
	return c.memory.Update(ctx, scope, root, content)
}

// ListSkills enumerates the skills visible from cwd (project over global) for
// skills.discovered.list.
func (c *Skills) ListSkills(ctx context.Context, cwd string) ([]skills.Info, error) {
	root, err := c.context.root(cwd)
	if err != nil {
		return nil, err
	}
	if c.skills == nil {
		return nil, nil
	}
	return c.skills.ListSkills(ctx, root)
}

// ListManagedSkills returns the global self-authored skill library — active and
// archived skills, each tagged with its lifecycle (skills.library.list). Empty
// when no authoring store is wired.
func (c *Skills) ListManagedSkills(ctx context.Context) ([]skills.Entry, error) {
	if c.curator == nil {
		return nil, nil
	}
	return c.curator.List(ctx)
}

// ArchiveSkill removes a skill from active use without deleting it
// (skills.library.archive). No-op when no authoring store is wired.
func (c *Skills) ArchiveSkill(ctx context.Context, name string) error {
	if c.curator == nil {
		return nil
	}
	if err := c.curator.Archive(ctx, name); err != nil {
		return err
	}
	c.notifySkillsChanged()
	return nil
}

// RestoreSkill returns an archived skill to active use
// (skills.library.restore). No-op when no authoring store is wired.
func (c *Skills) RestoreSkill(ctx context.Context, name string) error {
	if c.curator == nil {
		return nil
	}
	if err := c.curator.Restore(ctx, name); err != nil {
		return err
	}
	c.notifySkillsChanged()
	return nil
}

// ListSkillDrafts enumerates the agent-mined skill proposals awaiting offline
// review (skills.drafts.list). Reports [ErrSkillDraftsUnavailable] when no
// authoring store is wired.
func (c *Skills) ListSkillDrafts(ctx context.Context) ([]skills.DraftInfo, error) {
	if c.drafts == nil {
		return nil, ErrSkillDraftsUnavailable
	}
	return c.drafts.ListDrafts(ctx)
}

// PromoteSkillDraft publishes a reviewed draft into the active skill library
// (skills.drafts.promote). Reports [ErrSkillDraftsUnavailable] when no authoring
// store is wired.
func (c *Skills) PromoteSkillDraft(ctx context.Context, handle skills.DraftHandle) error {
	if c.drafts == nil {
		return ErrSkillDraftsUnavailable
	}
	if err := c.drafts.Promote(ctx, handle); err != nil {
		return err
	}
	c.notifySkillsChanged()
	return nil
}

func (c *Skills) notifySkillsChanged() {
	if c.skillsChanged != nil {
		c.skillsChanged(struct{}{})
	}
}

// RejectSkillDraft discards a reviewed draft (skills.drafts.reject). Reports
// [ErrSkillDraftsUnavailable] when no authoring store is wired.
func (c *Skills) RejectSkillDraft(ctx context.Context, handle skills.DraftHandle) error {
	if c.drafts == nil {
		return ErrSkillDraftsUnavailable
	}
	return c.drafts.DiscardDraft(ctx, handle)
}

// ListRecipes enumerates the prompt recipes visible from cwd — project recipes
// (<cwd>/.lyra/recipes) layered over the global directory, project winning on a
// name collision (recipes.list).
func (c *Discovery) ListRecipes(ctx context.Context, cwd string) ([]recipes.Recipe, error) {
	root, err := c.context.root(cwd)
	if err != nil {
		return nil, err
	}
	if c.recipes == nil {
		return nil, nil
	}
	return c.recipes.List(ctx, root)
}

// InspectHooks returns the lifecycle hooks discovered for cwd plus the project's
// trust status (hooks.list). Empty when hooks are unconfigured.
func (c *Hooks) InspectHooks(ctx context.Context, cwd string) (HookInspection, error) {
	root, err := c.context.root(cwd)
	if err != nil {
		return HookInspection{}, err
	}
	if c.hooks == nil {
		return HookInspection{}, nil
	}
	inspection, err := c.hooks.Inspect(ctx, root)
	if err != nil {
		return HookInspection{}, err
	}
	resolved := HookInspection{
		ProjectRoot: inspection.ProjectRoot, ProjectTrusted: inspection.ProjectTrusted,
		Hooks: make([]ResolvedHook, 0, len(inspection.Hooks)),
	}
	for _, hook := range inspection.Hooks {
		resolved.Hooks = append(resolved.Hooks, ResolvedHook{
			Hook: hook, Active: hook.Scope == hooks.ScopeGlobal || inspection.ProjectTrusted,
		})
	}
	return resolved, nil
}

// SetProjectHookTrust trusts (or revokes) a project's hooks (hooks.
// setTrust). No-op when no trust store is wired.
func (c *Hooks) SetProjectHookTrust(ctx context.Context, projectRoot string, trusted bool) error {
	root, err := c.context.root(projectRoot)
	if err != nil {
		return err
	}
	if c.trust == nil {
		return nil
	}
	if trusted {
		return c.trust.Trust(ctx, root)
	}
	return c.trust.Untrust(ctx, root)
}

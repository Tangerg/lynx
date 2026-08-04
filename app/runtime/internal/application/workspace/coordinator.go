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
	"github.com/Tangerg/lynx/app/runtime/internal/domain/skills"
)

// ErrMemoryUnavailable reports that this runtime was built without a knowledge store.
var ErrMemoryUnavailable = errors.New("workspace: memory unavailable")

// ErrSkillProposalsUnavailable reports that this runtime was built without a
// Skill proposal store, so authoring and review are not negotiated.
// Delivery maps it to capability_not_negotiated.
var ErrSkillProposalsUnavailable = errors.New("workspace: skill proposals unavailable")

// ErrSkillLibraryUnavailable reports that this runtime was built without the
// skill-library curator. Library mutations must fail explicitly rather than
// acknowledge a change that could not have happened.
var ErrSkillLibraryUnavailable = errors.New("workspace: skill library unavailable")

// ErrFileWatchUnavailable reports that this runtime has no workspace-change
// observer. A subscription without requested watches remains useful for
// application-published events; callers requesting Git-state watches receive
// this explicit capability failure instead of a silent, inert subscription.
var ErrFileWatchUnavailable = errors.New("workspace: file watch unavailable")

// SkillCatalog enumerates the skills visible from a working directory (project
// over user). The composition root supplies promptsource-backed discovery.
type SkillCatalog interface {
	ListSkills(ctx context.Context, workdir string) ([]SkillInfo, error)
}

// SkillCurator manages the user self-authored Skill library: listing every
// skill with its lifecycle and moving one between active and archived (never
// deleting). The composition root supplies the file-backed authoring store; nil
// disables the management surface.
type SkillCurator interface {
	List(ctx context.Context) ([]skills.Entry, error)
	Archive(ctx context.Context, name string) error
	Restore(ctx context.Context, name string) error
}

// SkillProposals stores immutable proposals in either the project or user Skill
// library. projectRoot is already resolved by the application boundary.
type SkillProposals interface {
	SubmitProposal(ctx context.Context, projectRoot string, proposal skills.Proposal) (skills.ProposalRef, error)
	ListProposals(ctx context.Context, projectRoot string) ([]skills.ProposalInfo, error)
	ApproveProposal(ctx context.Context, projectRoot string, ref skills.ProposalRef) error
	RejectProposal(ctx context.Context, projectRoot string, ref skills.ProposalRef) error
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

// KnowledgeStore is the workspace knowledge use case's complete persistence
// need. Prompt assembly receives its own read-only view from agentexec.
type KnowledgeStore interface {
	Get(ctx context.Context, scope knowledge.Scope, dir string) (string, error)
	Update(ctx context.Context, scope knowledge.Scope, dir string, content string) error
	List(ctx context.Context, dir string) ([]knowledge.Entry, error)
}

// RecipeLister discovers the prompt recipes visible from a working directory —
// a project's .lyra/recipes layered over the global directory. The composition
// root supplies the filesystem-backed implementation (the promptsource adapter);
// the port keeps the coordinator free of file I/O.
type RecipeLister interface {
	List(ctx context.Context, cwd string) ([]Recipe, error)
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
	defaultWorkspacePath string
	userHome             string
	paths                Paths
}

// NewContext constructs the shared workspace identity resolver.
func NewContext(defaultWorkspacePath, userHome string, paths Paths) *Context {
	return &Context{
		defaultWorkspacePath: defaultWorkspacePath,
		userHome:             userHome,
		paths:                paths,
	}
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
	context    *Context
	workspaces Catalog
	agentDocs  AgentDocFinder
	recipes    RecipeLister
}

func NewDiscovery(context *Context, workspaces Catalog, agentDocs AgentDocFinder, recipes RecipeLister) *Discovery {
	return &Discovery{context: context, workspaces: workspaces, agentDocs: agentDocs, recipes: recipes}
}

// Knowledge owns the human-authored LYRA.md cascade use cases.
type Knowledge struct {
	context *Context
	memory  KnowledgeStore
}

func NewKnowledge(context *Context, memory KnowledgeStore) *Knowledge {
	return &Knowledge{context: context, memory: memory}
}

// Skills owns skill discovery, library curation, and proposal review.
type Skills struct {
	context       *Context
	skills        SkillCatalog
	curator       SkillCurator
	proposals     SkillProposals
	skillsChanged func(struct{})
}

func NewSkills(context *Context, skills SkillCatalog, curator SkillCurator, proposals SkillProposals, skillsChanged func(struct{})) *Skills {
	return &Skills{context: context, skills: skills, curator: curator, proposals: proposals, skillsChanged: skillsChanged}
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
	if err := scope.Validate(); err != nil {
		return "", err
	}
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
	if err := scope.Validate(); err != nil {
		return err
	}
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

// ListSkills enumerates the Skills visible from cwd (project over user) for
// skills.discovered.list.
func (c *Skills) ListSkills(ctx context.Context, cwd string) ([]SkillInfo, error) {
	root, err := c.context.root(cwd)
	if err != nil {
		return nil, err
	}
	if c.skills == nil {
		return nil, nil
	}
	return c.skills.ListSkills(ctx, root)
}

// ListManagedSkills returns the user self-authored Skill library — active and
// archived skills, each tagged with its lifecycle (skills.library.list).
// Reports [ErrSkillLibraryUnavailable] when no curator is wired.
func (c *Skills) ListManagedSkills(ctx context.Context) ([]skills.Entry, error) {
	if c.curator == nil {
		return nil, ErrSkillLibraryUnavailable
	}
	return c.curator.List(ctx)
}

// ArchiveSkill removes a skill from active use without deleting it
// (skills.library.archive). Reports [ErrSkillLibraryUnavailable] when no
// curator is wired.
func (c *Skills) ArchiveSkill(ctx context.Context, name string) error {
	if c.curator == nil {
		return ErrSkillLibraryUnavailable
	}
	if err := c.curator.Archive(ctx, name); err != nil {
		return err
	}
	c.notifySkillsChanged()
	return nil
}

// RestoreSkill returns an archived skill to active use
// (skills.library.restore). Reports [ErrSkillLibraryUnavailable] when no
// curator is wired.
func (c *Skills) RestoreSkill(ctx context.Context, name string) error {
	if c.curator == nil {
		return ErrSkillLibraryUnavailable
	}
	if err := c.curator.Restore(ctx, name); err != nil {
		return err
	}
	c.notifySkillsChanged()
	return nil
}

// SubmitSkillProposal submits immutable content to the requested project or
// user review queue. It never activates the Skill.
func (c *Skills) SubmitSkillProposal(ctx context.Context, cwd string, proposal skills.Proposal) (skills.ProposalRef, error) {
	if c.proposals == nil {
		return skills.ProposalRef{}, ErrSkillProposalsUnavailable
	}
	root, err := c.context.root(cwd)
	if err != nil {
		return skills.ProposalRef{}, err
	}
	ref, err := c.proposals.SubmitProposal(ctx, root, proposal)
	if err != nil {
		return skills.ProposalRef{}, err
	}
	c.notifySkillsChanged()
	return ref, nil
}

func (c *Skills) ListSkillProposals(ctx context.Context, cwd string) ([]skills.ProposalInfo, error) {
	if c.proposals == nil {
		return nil, ErrSkillProposalsUnavailable
	}
	root, err := c.context.root(cwd)
	if err != nil {
		return nil, err
	}
	return c.proposals.ListProposals(ctx, root)
}

func (c *Skills) ApproveSkillProposal(ctx context.Context, cwd string, ref skills.ProposalRef) error {
	if c.proposals == nil {
		return ErrSkillProposalsUnavailable
	}
	root, err := c.context.root(cwd)
	if err != nil {
		return err
	}
	if err := c.proposals.ApproveProposal(ctx, root, ref); err != nil {
		return err
	}
	c.notifySkillsChanged()
	return nil
}

func (c *Skills) RejectSkillProposal(ctx context.Context, cwd string, ref skills.ProposalRef) error {
	if c.proposals == nil {
		return ErrSkillProposalsUnavailable
	}
	root, err := c.context.root(cwd)
	if err != nil {
		return err
	}
	if err := c.proposals.RejectProposal(ctx, root, ref); err != nil {
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

// ListRecipes enumerates the prompt recipes visible from cwd — project recipes
// (<cwd>/.lyra/recipes) layered over the global directory, project winning on a
// name collision (recipes.list).
func (c *Discovery) ListRecipes(ctx context.Context, cwd string) ([]Recipe, error) {
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

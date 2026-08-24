package workspace

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
)

// RecipeLister discovers the recipes visible from a working directory.
type RecipeLister interface {
	List(ctx context.Context, cwd string) ([]Recipe, error)
}

// Discovery owns workspace, recipe, and instruction-document discovery.
type Discovery struct {
	scope      *Scope
	workspaces Catalog
	agentDocs  AgentDocFinder
	recipes    RecipeLister
}

func NewDiscovery(scope *Scope, workspaces Catalog, agentDocs AgentDocFinder, recipes RecipeLister) *Discovery {
	return &Discovery{scope: scope, workspaces: workspaces, agentDocs: agentDocs, recipes: recipes}
}

// Recipes enumerates project recipes layered over the global directory.
func (d *Discovery) Recipes(ctx context.Context, cwd string) ([]Recipe, error) {
	root, err := d.scope.root(cwd)
	if err != nil {
		return nil, err
	}
	if d.recipes == nil {
		return nil, nil
	}
	return d.recipes.List(ctx, root)
}

// Resolved is the current filesystem identity of one workspace ref.
type Resolved struct {
	Path        string
	ProjectRoot string
	Missing     bool
}

// Summary is a distinct workspace identity derived from user-facing sessions.
type Summary struct {
	Name         string
	Path         string
	ProjectRoot  string
	Missing      bool
	SessionCount int
	LastActiveAt time.Time
}

// Catalog supplies the user-facing sessions and their current workspace
// identities. The session coordinator is the production implementation.
type Catalog interface {
	List(ctx context.Context) ([]session.Session, error)
	InspectWorkspace(cwd string) (Resolved, error)
}

// Resolve returns the canonical live workspace identity for path, using the
// host-provided default when path is empty.
func (d *Discovery) Resolve(path string) (Resolved, error) {
	if d.workspaces == nil {
		return Resolved{}, errors.New("workspace: workspace catalog is not configured")
	}
	if path == "" {
		path = d.scope.defaultWorkspacePath
	}
	identity, err := d.workspaces.InspectWorkspace(path)
	if err != nil {
		return Resolved{}, err
	}
	return Resolved{
		Path: identity.Path, ProjectRoot: identity.ProjectRoot, Missing: identity.Missing,
	}, nil
}

// Workspaces returns each non-empty session workspace once, newest-active first.
func (d *Discovery) Workspaces(ctx context.Context) ([]Summary, error) {
	if d.workspaces == nil {
		return nil, errors.New("workspace: workspace catalog is not configured")
	}
	sessions, err := d.workspaces.List(ctx)
	if err != nil {
		return nil, err
	}
	workspaces := workspacesFromSessions(sessions)
	for index := range workspaces {
		identity, err := d.workspaces.InspectWorkspace(workspaces[index].Path)
		if err != nil {
			return nil, err
		}
		workspaces[index].Path = identity.Path
		workspaces[index].ProjectRoot = identity.ProjectRoot
		workspaces[index].Missing = identity.Missing
	}
	return workspaces, nil
}

func workspacesFromSessions(sessions []session.Session) []Summary {
	byPath := map[string]*Summary{}
	for _, sessionValue := range sessions {
		path := sessionValue.Workspace().Path()
		workspace := byPath[path]
		if workspace == nil {
			workspace = &Summary{Path: path, Name: filepath.Base(path)}
			byPath[path] = workspace
		}
		workspace.SessionCount++
		if workspace.LastActiveAt.IsZero() || sessionValue.UpdatedAt().After(workspace.LastActiveAt) {
			workspace.LastActiveAt = sessionValue.UpdatedAt()
		}
	}
	workspaces := make([]Summary, 0, len(byPath))
	for _, workspace := range byPath {
		workspaces = append(workspaces, *workspace)
	}
	slices.SortFunc(workspaces, func(a, b Summary) int { return b.LastActiveAt.Compare(a.LastActiveAt) })
	return workspaces
}

// AgentDoc is one discovered instruction document with its cascade scope.
type AgentDoc struct {
	Path  string
	Scope AgentDocScope
}

// AgentDocFinder discovers the workspace instruction-document cascade.
type AgentDocFinder interface {
	Find(ctx context.Context, cwd, home string) ([]AgentDocFile, error)
}

// AgentDocs returns the instruction-document cascade for one workspace.
func (d *Discovery) AgentDocs(ctx context.Context, cwd string) ([]AgentDoc, error) {
	root, err := d.scope.root(cwd)
	if err != nil {
		return nil, err
	}
	if d.agentDocs == nil {
		return nil, errors.New("workspace: agent document finder is not configured")
	}
	files, err := d.agentDocs.Find(ctx, root, d.scope.userHome)
	if err != nil {
		return nil, err
	}
	docs := make([]AgentDoc, 0, len(files))
	for _, file := range files {
		switch file.Scope {
		case AgentDocScopeHome, AgentDocScopeProjectRoot, AgentDocScopeCWD:
			docs = append(docs, AgentDoc{Path: file.Path, Scope: file.Scope})
		default:
			return nil, fmt.Errorf("workspace: unsupported agent document scope %q", file.Scope)
		}
	}
	return docs, nil
}

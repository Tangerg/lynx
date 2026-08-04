package workspace

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
)

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
	InspectWorkspace(cwd string) (session.WorkspaceIdentity, error)
}

// ResolveWorkspace returns the canonical live identity for path, using the
// host-provided default when path is empty.
func (c *Discovery) ResolveWorkspace(path string) (Resolved, error) {
	if c.workspaces == nil {
		return Resolved{}, errors.New("workspace: workspace catalog is not configured")
	}
	if path == "" {
		path = c.context.defaultWorkspacePath
	}
	identity, err := c.workspaces.InspectWorkspace(path)
	if err != nil {
		return Resolved{}, err
	}
	return Resolved{
		Path: identity.Cwd, ProjectRoot: identity.ProjectRoot, Missing: identity.Missing,
	}, nil

}

// ListWorkspaces returns each non-empty session workspace once, newest-active first.
func (c *Discovery) ListWorkspaces(ctx context.Context) ([]Summary, error) {
	if c.workspaces == nil {
		return nil, errors.New("workspace: workspace catalog is not configured")
	}
	sessions, err := c.workspaces.List(ctx)
	if err != nil {
		return nil, err
	}
	workspaces := workspacesFromSessions(sessions)
	for index := range workspaces {
		identity, err := c.workspaces.InspectWorkspace(workspaces[index].Path)
		if err != nil {
			return nil, err
		}
		workspaces[index].Path = identity.Cwd
		workspaces[index].ProjectRoot = identity.ProjectRoot
		workspaces[index].Missing = identity.Missing
	}
	return workspaces, nil
}

func workspacesFromSessions(sessions []session.Session) []Summary {
	byPath := map[string]*Summary{}
	for _, session := range sessions {
		if session.Cwd == "" {
			continue
		}
		workspace := byPath[session.Cwd]
		if workspace == nil {
			workspace = &Summary{Path: session.Cwd, Name: filepath.Base(session.Cwd)}
			byPath[session.Cwd] = workspace
		}
		workspace.SessionCount++
		if workspace.LastActiveAt.IsZero() || session.UpdatedAt.After(workspace.LastActiveAt) {
			workspace.LastActiveAt = session.UpdatedAt
		}
	}
	workspaces := make([]Summary, 0, len(byPath))
	for _, workspace := range byPath {
		workspaces = append(workspaces, *workspace)
	}
	slices.SortFunc(workspaces, func(a, b Summary) int { return b.LastActiveAt.Compare(a.LastActiveAt) })
	return workspaces
}

// AgentDocScope identifies where an instruction document participates in the
// cascade.
type AgentDocScope string

const (
	AgentDocScopeHome        AgentDocScope = "home"
	AgentDocScopeCwd         AgentDocScope = "cwd"
	AgentDocScopeProjectRoot AgentDocScope = "projectRoot"
)

// AgentDoc is one discovered instruction document with its cascade scope.
type AgentDoc struct {
	Path  string
	Scope AgentDocScope
}

// AgentDocFinder discovers the workspace instruction-document cascade.
type AgentDocFinder interface {
	DiscoverAgentDocs(ctx context.Context, cwd, home string) ([]AgentDocFile, error)
}

// ListAgentDocs returns the instruction-document cascade for one workspace.
func (c *Discovery) ListAgentDocs(ctx context.Context, cwd string) ([]AgentDoc, error) {
	root, err := c.context.root(cwd)
	if err != nil {
		return nil, err
	}
	if c.agentDocs == nil {
		return nil, errors.New("workspace: agent document finder is not configured")
	}
	files, err := c.agentDocs.DiscoverAgentDocs(ctx, root, c.context.userHome)
	if err != nil {
		return nil, err
	}
	docs := make([]AgentDoc, 0, len(files))
	for _, file := range files {
		docs = append(docs, AgentDoc{Path: file.Path, Scope: agentDocScope(file.Path, root, c.context.userHome)})
	}
	return docs, nil
}

func agentDocScope(path, cwd, home string) AgentDocScope {
	dir := filepath.Dir(path)
	switch {
	case home != "" && dir == home:
		return AgentDocScopeHome
	case cwd != "" && (dir == cwd || strings.HasPrefix(path, cwd+string(filepath.Separator))):
		return AgentDocScopeCwd
	default:
		return AgentDocScopeProjectRoot
	}
}

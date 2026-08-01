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

// ResolvedWorkspace is the current filesystem identity of one workspace ref.
type ResolvedWorkspace struct {
	Path        string
	ProjectRoot string
	Missing     bool
}

// WorkspaceSummary is a distinct workspace identity derived from user-facing sessions.
type WorkspaceSummary struct {
	Name         string
	Path         string
	ProjectRoot  string
	Missing      bool
	SessionCount int
	LastActiveAt time.Time
}

// WorkspaceCatalog supplies the user-facing sessions and their current workspace
// identities. The session coordinator is the production implementation.
type WorkspaceCatalog interface {
	List(ctx context.Context) ([]session.Session, error)
	InspectWorkspace(cwd string) (session.WorkspaceIdentity, error)
}

// ResolveWorkspace returns the canonical live identity for path, using the
// host-provided default when path is empty.
func (c *Discovery) ResolveWorkspace(path string) (ResolvedWorkspace, error) {
	if c.workspaces == nil {
		return ResolvedWorkspace{}, errors.New("workspace: workspace catalog is not configured")
	}
	if path == "" {
		path = c.context.defaultCwd
	}
	identity, err := c.workspaces.InspectWorkspace(path)
	if err != nil {
		return ResolvedWorkspace{}, err
	}
	return ResolvedWorkspace{
		Path: identity.Cwd, ProjectRoot: identity.ProjectRoot, Missing: identity.Missing,
	}, nil

}

// ListWorkspaces returns each non-empty session workspace once, newest-active first.
func (c *Discovery) ListWorkspaces(ctx context.Context) ([]WorkspaceSummary, error) {
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

func workspacesFromSessions(sessions []session.Session) []WorkspaceSummary {
	byPath := map[string]*WorkspaceSummary{}
	for _, session := range sessions {
		if session.Cwd == "" {
			continue
		}
		workspace := byPath[session.Cwd]
		if workspace == nil {
			workspace = &WorkspaceSummary{Path: session.Cwd, Name: filepath.Base(session.Cwd)}
			byPath[session.Cwd] = workspace
		}
		workspace.SessionCount++
		if workspace.LastActiveAt.IsZero() || session.UpdatedAt.After(workspace.LastActiveAt) {
			workspace.LastActiveAt = session.UpdatedAt
		}
	}
	workspaces := make([]WorkspaceSummary, 0, len(byPath))
	for _, workspace := range byPath {
		workspaces = append(workspaces, *workspace)
	}
	slices.SortFunc(workspaces, func(a, b WorkspaceSummary) int { return b.LastActiveAt.Compare(a.LastActiveAt) })
	return workspaces
}

// AgentDocScope identifies where an instruction document participates in the
// cascade, without leaking a raw delivery enum through the application layer.
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
	files, err := c.agentDocs.DiscoverAgentDocs(ctx, root, c.context.home)
	if err != nil {
		return nil, err
	}
	docs := make([]AgentDoc, 0, len(files))
	for _, file := range files {
		docs = append(docs, AgentDoc{Path: file.Path, Scope: agentDocScope(file.Path, root, c.context.home)})
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

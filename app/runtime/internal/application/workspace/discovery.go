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

// Project is a distinct workspace identity derived from user-facing sessions.
type Project struct {
	Name         string
	Cwd          string
	ProjectRoot  string
	CwdMissing   bool
	SessionCount int
	LastActiveAt time.Time
}

// ProjectCatalog supplies the user-facing sessions and their current workspace
// identities. The session coordinator is the production implementation.
type ProjectCatalog interface {
	List(ctx context.Context) ([]session.Session, error)
	InspectWorkspace(cwd string) (session.WorkspaceIdentity, error)
}

// ListProjects returns each non-empty session cwd once, newest-active first.
func (c *Discovery) ListProjects(ctx context.Context) ([]Project, error) {
	if c.projects == nil {
		return nil, errors.New("workspace: project catalog is not configured")
	}
	sessions, err := c.projects.List(ctx)
	if err != nil {
		return nil, err
	}
	projects := projectsFromSessions(sessions)
	for index := range projects {
		identity, err := c.projects.InspectWorkspace(projects[index].Cwd)
		if err != nil {
			return nil, err
		}
		projects[index].Cwd = identity.Cwd
		projects[index].ProjectRoot = identity.ProjectRoot
		projects[index].CwdMissing = identity.Missing
	}
	return projects, nil
}

func projectsFromSessions(sessions []session.Session) []Project {
	byCwd := map[string]*Project{}
	for _, session := range sessions {
		if session.Cwd == "" {
			continue
		}
		project := byCwd[session.Cwd]
		if project == nil {
			project = &Project{Cwd: session.Cwd, Name: filepath.Base(session.Cwd)}
			byCwd[session.Cwd] = project
		}
		project.SessionCount++
		if project.LastActiveAt.IsZero() || session.UpdatedAt.After(project.LastActiveAt) {
			project.LastActiveAt = session.UpdatedAt
		}
	}
	projects := make([]Project, 0, len(byCwd))
	for _, project := range byCwd {
		projects = append(projects, *project)
	}
	slices.SortFunc(projects, func(a, b Project) int { return b.LastActiveAt.Compare(a.LastActiveAt) })
	return projects
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

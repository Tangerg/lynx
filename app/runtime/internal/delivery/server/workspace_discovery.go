package server

import (
	"context"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/agentdoc"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
)

// workspace.* discovery reads (API.md §7.5): the distinct-cwd Project view,
// skills, and the AGENTS.md cascade — derived from sessions / the engine /
// AGENTS.md discovery rather than the filesystem or git.

// WorkspaceListProjects derives the Project view from sessions: one
// entry per distinct Session.cwd (API.md §0.2 / §7.5), newest-active
// first. projectRoot / branch are best-effort decorations left empty
// until the engine grows a git probe.
func (s *Server) WorkspaceListProjects(ctx context.Context, _ protocol.PageQuery) (*protocol.Page[protocol.Project], error) {
	sessions, err := s.rt.ListSessions(ctx)
	if err != nil {
		return nil, err
	}
	return protocol.NewPage(projectsFromSessions(sessions)), nil
}

// projectsFromSessions collapses sessions into the distinct-cwd Project
// view: one entry per non-empty Session.cwd, session-counted, newest-
// active first. Pure (no I/O) so it's unit-testable on its own.
func projectsFromSessions(sessions []session.Session) []protocol.Project {
	byCwd := map[string]*protocol.Project{}
	for _, s := range sessions {
		if s.Cwd == "" {
			continue // no cwd ⇒ no project identity
		}
		p := byCwd[s.Cwd]
		if p == nil {
			p = &protocol.Project{Cwd: s.Cwd, Name: filepath.Base(s.Cwd)}
			byCwd[s.Cwd] = p
		}
		p.SessionCount++
		if p.LastActiveAt == nil || s.UpdatedAt.After(*p.LastActiveAt) {
			t := s.UpdatedAt
			p.LastActiveAt = &t
		}
	}
	out := make([]protocol.Project, 0, len(byCwd))
	for _, p := range byCwd {
		out = append(out, *p)
	}
	slices.SortFunc(out, func(a, b protocol.Project) int {
		return b.LastActiveAt.Compare(*a.LastActiveAt) // most-recently-active first
	})
	return out
}

// WorkspaceListSkills enumerates the skills visible from cwd — project skills
// (<cwd>/.lyra/skills) layered over the global directory, project winning on a
// name collision (the same set + precedence the engine gives the model). Each
// entry's Source records its scope ("project" | "global"). cwd defaults to the
// serve directory (API.md §7.5).
func (s *Server) WorkspaceListSkills(ctx context.Context, in protocol.WorkspaceListQuery) (*protocol.Page[protocol.Skill], error) {
	root, err := s.workspaceRoot(in.Cwd)
	if err != nil {
		return nil, err
	}
	found, err := s.rt.ListSkills(ctx, root)
	if err != nil {
		return nil, err
	}
	out := make([]protocol.Skill, 0, len(found))
	for _, sk := range found {
		out = append(out, protocol.Skill{Name: sk.Name, Description: sk.Description, Source: protocol.SkillSource(sk.Scope)})
	}
	return protocol.NewPage(out), nil
}

// WorkspaceListRecipes enumerates the prompt recipes visible from cwd — project
// recipes (<cwd>/.lyra/recipes) layered over the global directory, project
// winning on a name collision. Each entry carries its full Body so the client
// can expand ($ARGUMENTS / $1..$9) and send it. cwd defaults to the serve
// directory (workspace.recipes.list, API.md §7.5).
func (s *Server) WorkspaceListRecipes(ctx context.Context, in protocol.WorkspaceListQuery) (*protocol.Page[protocol.Recipe], error) {
	root, err := s.workspaceRoot(in.Cwd)
	if err != nil {
		return nil, err
	}
	found, err := s.rt.ListRecipes(ctx, root)
	if err != nil {
		return nil, err
	}
	out := make([]protocol.Recipe, 0, len(found))
	for _, r := range found {
		out = append(out, protocol.Recipe{
			Name:         r.Name,
			Description:  r.Description,
			ArgumentHint: r.ArgumentHint,
			Body:         r.Body,
			Scope:        protocol.RecipeScope(r.Scope),
			Source:       r.Source,
		})
	}
	return protocol.NewPage(out), nil
}

// WorkspaceListAgentDocs lists the AGENTS.md files discovered from cwd
// (or the serve cwd) up to home — the same cascade the engine injects
// into the system prompt (API.md §7.5).
func (s *Server) WorkspaceListAgentDocs(ctx context.Context, q protocol.WorkspaceListQuery) (*protocol.Page[protocol.AgentDoc], error) {
	root, err := s.workspaceRoot(q.Cwd)
	if err != nil {
		return nil, err
	}
	home := s.serverInfo.Home
	files, err := agentdoc.Discover(ctx, root, home)
	if err != nil {
		return nil, err
	}
	out := make([]protocol.AgentDoc, 0, len(files))
	for _, f := range files {
		out = append(out, protocol.AgentDoc{Path: f.Path, Scope: protocol.AgentDocScope(agentDocScope(f.Path, root, home))})
	}
	return protocol.NewPage(out), nil
}

// agentDocScope classifies a discovered AGENTS.md by where it sits in the
// cwd→home cascade: the home dir → "home", anything under cwd → "cwd",
// else an ancestor in between → "projectRoot" (API.md §4.10 scope).
func agentDocScope(path, cwd, home string) string {
	dir := filepath.Dir(path)
	switch {
	case home != "" && dir == home:
		return "home"
	case cwd != "" && (dir == cwd || strings.HasPrefix(path, cwd+string(filepath.Separator))):
		return "cwd"
	default:
		return "projectRoot"
	}
}

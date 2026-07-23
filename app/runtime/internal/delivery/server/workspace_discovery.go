package server

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
)

// WorkspaceListProjects projects the application-owned distinct-workspace view
// derived from user-facing sessions.
func (s *Server) WorkspaceListProjects(ctx context.Context, _ protocol.PageQuery) (*protocol.Page[protocol.Project], error) {
	projects, err := s.workspaceDiscovery.ListProjects(ctx)
	if err != nil {
		return nil, wireWorkspaceError(err)
	}
	out := make([]protocol.Project, 0, len(projects))
	for _, project := range projects {
		lastActiveAt := project.LastActiveAt
		out = append(out, protocol.Project{
			Name: project.Name, Cwd: project.Cwd, ProjectRoot: project.ProjectRoot, CwdMissing: project.CwdMissing,
			SessionCount: project.SessionCount, LastActiveAt: &lastActiveAt,
		})
	}
	return protocol.NewPage(out), nil
}

// WorkspaceListSkills maps application skill discovery to the protocol shape.
func (s *Server) WorkspaceListSkills(ctx context.Context, in protocol.WorkspaceListQuery) (*protocol.Page[protocol.Skill], error) {
	found, err := s.workspaceSkills.ListSkills(ctx, in.Cwd)
	if err != nil {
		return nil, wireWorkspaceError(err)
	}
	out := make([]protocol.Skill, 0, len(found))
	for _, skill := range found {
		out = append(out, protocol.Skill{Name: skill.Name, Description: skill.Description, Source: protocol.SkillSource(skill.Scope)})
	}
	return protocol.NewPage(out), nil
}

// WorkspaceListRecipes maps application recipe discovery to the protocol shape.
func (s *Server) WorkspaceListRecipes(ctx context.Context, in protocol.WorkspaceListQuery) (*protocol.Page[protocol.Recipe], error) {
	found, err := s.workspaceDiscovery.ListRecipes(ctx, in.Cwd)
	if err != nil {
		return nil, wireWorkspaceError(err)
	}
	out := make([]protocol.Recipe, 0, len(found))
	for _, recipe := range found {
		out = append(out, protocol.Recipe{
			Name: recipe.Name, Description: recipe.Description, ArgumentHint: recipe.ArgumentHint,
			Body: recipe.Body, Scope: protocol.RecipeScope(recipe.Scope), Source: recipe.Source,
		})
	}
	return protocol.NewPage(out), nil
}

// WorkspaceListAgentDocs maps the application-owned instruction-document
// cascade onto the protocol shape.
func (s *Server) WorkspaceListAgentDocs(ctx context.Context, in protocol.WorkspaceListQuery) (*protocol.Page[protocol.AgentDoc], error) {
	docs, err := s.workspaceDiscovery.ListAgentDocs(ctx, in.Cwd)
	if err != nil {
		return nil, wireWorkspaceError(err)
	}
	out := make([]protocol.AgentDoc, 0, len(docs))
	for _, doc := range docs {
		out = append(out, protocol.AgentDoc{Path: doc.Path, Scope: protocol.AgentDocScope(string(doc.Scope))})
	}
	return protocol.NewPage(out), nil
}

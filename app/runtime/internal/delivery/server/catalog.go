package server

import (
	"context"
	"fmt"

	workspaceapp "github.com/Tangerg/lynx/app/runtime/internal/application/workspace"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
)

// ResolveWorkspace projects the application's current filesystem identity onto
// the canonical wire resource.
func (s *Server) ResolveWorkspace(_ context.Context, in protocol.ResolveWorkspaceRequest) (*protocol.WorkspaceInfo, error) {
	path := ""
	if in.Ref != nil {
		path = in.Ref.Path
	}
	resolved, err := s.workspaceDiscovery.ResolveWorkspace(path)
	if err != nil {
		return nil, wireWorkspaceError(err)
	}
	out := workspaceInfoToWire(resolved.Path, resolved.ProjectRoot, resolved.Missing)
	return &out, nil
}

// ListWorkspaces projects the application-owned distinct-workspace view
// derived from user-facing sessions.
func (s *Server) ListWorkspaces(ctx context.Context, _ protocol.PageQuery) (*protocol.Page[protocol.WorkspaceSummary], error) {
	workspaces, err := s.workspaceDiscovery.ListWorkspaces(ctx)
	if err != nil {
		return nil, wireWorkspaceError(err)
	}
	out := make([]protocol.WorkspaceSummary, 0, len(workspaces))
	for _, workspace := range workspaces {
		lastActiveAt := workspace.LastActiveAt
		out = append(out, protocol.WorkspaceSummary{
			Workspace: workspaceInfoToWire(workspace.Path, workspace.ProjectRoot, workspace.Missing),
			Name:      workspace.Name, SessionCount: workspace.SessionCount, LastActiveAt: &lastActiveAt,
		})
	}
	return protocol.NewPage(out), nil
}

func workspaceInfoToWire(path, projectRoot string, missing bool) protocol.WorkspaceInfo {
	availability := protocol.WorkspaceAvailable
	if missing {
		availability = protocol.WorkspaceMissing
	}
	return protocol.WorkspaceInfo{
		Ref: protocol.WorkspaceRef{Path: path}, ProjectRoot: projectRoot, Availability: availability,
	}
}

// ListDiscoveredSkills maps application skill discovery to the protocol shape.
func (s *Server) ListDiscoveredSkills(ctx context.Context, in protocol.WorkspaceListQuery) (*protocol.Page[protocol.Skill], error) {
	found, err := s.workspaceSkills.ListSkills(ctx, in.Workspace.Path)
	if err != nil {
		return nil, wireWorkspaceError(err)
	}
	out := make([]protocol.Skill, 0, len(found))
	for _, skill := range found {
		source, ok := skillSourceWire(skill.Scope)
		if !ok {
			return nil, fmt.Errorf("skills.discovered.list: unsupported skill scope %q", skill.Scope)
		}
		out = append(out, protocol.Skill{Name: skill.Name, Description: skill.Description, Source: source})
	}
	return protocol.NewPage(out), nil
}

// ListRecipes maps application recipe discovery to the protocol shape.
func (s *Server) ListRecipes(ctx context.Context, in protocol.WorkspaceListQuery) (*protocol.Page[protocol.Recipe], error) {
	found, err := s.workspaceDiscovery.ListRecipes(ctx, in.Workspace.Path)
	if err != nil {
		return nil, wireWorkspaceError(err)
	}
	out := make([]protocol.Recipe, 0, len(found))
	for _, recipe := range found {
		scope, ok := recipeScopeWire(recipe.Scope)
		if !ok {
			return nil, fmt.Errorf("recipes.list: unsupported recipe scope %q", recipe.Scope)
		}
		out = append(out, protocol.Recipe{
			Name: recipe.Name, Description: recipe.Description, ArgumentHint: recipe.ArgumentHint,
			Body: recipe.Body, Scope: scope, Source: recipe.Source,
		})
	}
	return protocol.NewPage(out), nil
}

// ListAgentDocs maps the application-owned instruction-document
// cascade onto the protocol shape.
func (s *Server) ListAgentDocs(ctx context.Context, in protocol.WorkspaceListQuery) (*protocol.Page[protocol.AgentDoc], error) {
	docs, err := s.workspaceDiscovery.ListAgentDocs(ctx, in.Workspace.Path)
	if err != nil {
		return nil, wireWorkspaceError(err)
	}
	out := make([]protocol.AgentDoc, 0, len(docs))
	for _, doc := range docs {
		scope, ok := agentDocScopeWire(doc.Scope)
		if !ok {
			return nil, fmt.Errorf("agentDocs.list: unsupported document scope %q", doc.Scope)
		}
		out = append(out, protocol.AgentDoc{Path: doc.Path, Scope: scope})
	}
	return protocol.NewPage(out), nil
}

func skillSourceWire(scope workspaceapp.SkillScope) (protocol.SkillSource, bool) {
	switch scope {
	case workspaceapp.SkillScopeProject:
		return protocol.SkillSourceProject, true
	case workspaceapp.SkillScopeGlobal:
		return protocol.SkillSourceGlobal, true
	default:
		return "", false
	}
}

func recipeScopeWire(scope workspaceapp.RecipeScope) (protocol.RecipeScope, bool) {
	switch scope {
	case workspaceapp.RecipeScopeProject:
		return protocol.RecipeScopeProject, true
	case workspaceapp.RecipeScopeGlobal:
		return protocol.RecipeScopeGlobal, true
	default:
		return "", false
	}
}

func agentDocScopeWire(scope workspaceapp.AgentDocScope) (protocol.AgentDocScope, bool) {
	switch scope {
	case workspaceapp.AgentDocScopeCwd:
		return protocol.AgentDocScopeCwd, true
	case workspaceapp.AgentDocScopeProjectRoot:
		return protocol.AgentDocScopeProjectRoot, true
	case workspaceapp.AgentDocScopeHome:
		return protocol.AgentDocScopeHome, true
	default:
		return "", false
	}
}

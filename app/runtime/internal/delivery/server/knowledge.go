package server

import (
	"context"
	"fmt"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/knowledge"
)

// ListKnowledge enumerates LYRA.md entries across scopes (API.md §7.7).
// The entire knowledge.* group is capability-gated, so an unwired store is a
// capability error rather than a synthetic empty collection.
func (s *Server) ListKnowledge(ctx context.Context, in protocol.WorkspaceQuery) (*protocol.Page[protocol.KnowledgeEntry], error) {
	entries, err := s.workspaceKnowledge.Entries(ctx, in.Workspace.Path)
	if err != nil {
		return nil, wireWorkspaceError(err)
	}
	out := make([]protocol.KnowledgeEntry, 0, len(entries))
	for _, e := range entries {
		scope, err := presentKnowledgeScope(e.Scope)
		if err != nil {
			return nil, err
		}
		out = append(out, protocol.KnowledgeEntry{
			Scope:     scope,
			Content:   e.Content,
			UpdatedAt: e.UpdatedAt,
		})
	}
	return protocol.NewPage(out), nil
}

// GetKnowledge returns one scope's LYRA.md content. Dispatch has already
// validated the scope (KnowledgeScope.Valid).
func (s *Server) GetKnowledge(ctx context.Context, in protocol.GetKnowledgeRequest) (*protocol.KnowledgeEntry, error) {
	scope, cwd, err := s.knowledgeTargetFromWire(in.Scope, workspaceRefPath(in.Workspace))
	if err != nil {
		return nil, err
	}
	content, err := s.workspaceKnowledge.Read(ctx, scope, cwd)
	if err != nil {
		return nil, wireWorkspaceError(err)
	}
	return &protocol.KnowledgeEntry{Scope: in.Scope, Content: content}, nil
}

func (s *Server) UpdateKnowledge(ctx context.Context, in protocol.UpdateKnowledgeRequest) error {
	scope, cwd, err := s.knowledgeTargetFromWire(in.Scope, workspaceRefPath(in.Workspace))
	if err != nil {
		return err
	}
	return wireWorkspaceError(s.workspaceKnowledge.Update(ctx, scope, cwd, in.Content))
}

// presentKnowledgeScope / knowledgeScopeFromWire bridge the protocol and Domain closed
// vocabularies. The wire's cwd + projectRoot both
// fold into the project scope (addressed by the request's cwd);
// home maps to the user scope.
func presentKnowledgeScope(scope knowledge.Scope) (protocol.KnowledgeScope, error) {
	switch scope {
	case knowledge.ScopeProject:
		return protocol.KnowledgeScopeCWD, nil
	case knowledge.ScopeUser:
		return protocol.KnowledgeScopeHome, nil
	default:
		return "", fmt.Errorf("knowledge: unsupported knowledge scope %q", scope)
	}
}

func knowledgeScopeFromWire(scope protocol.KnowledgeScope) (knowledge.Scope, error) {
	switch scope {
	case protocol.KnowledgeScopeCWD, protocol.KnowledgeScopeProjectRoot:
		return knowledge.ScopeProject, nil
	case protocol.KnowledgeScopeHome:
		return knowledge.ScopeUser, nil
	default:
		return "", fmt.Errorf("%w: unknown knowledge scope %q", protocol.ErrInvalidParams, scope)
	}
}

func (s *Server) knowledgeTargetFromWire(scope protocol.KnowledgeScope, cwd string) (knowledge.Scope, string, error) {
	target, err := knowledgeScopeFromWire(scope)
	if err != nil {
		return "", "", err
	}
	if target == knowledge.ScopeUser {
		return target, "", nil
	}
	return target, cwd, nil
}

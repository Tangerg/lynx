package server

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/knowledge"
	"github.com/Tangerg/lynx/app/runtime/protocol"
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
		wire := presentKnowledgeEntry(e)
		wire.Scope = scope
		out = append(out, wire)
	}
	return protocol.NewPage(out), nil
}

// GetKnowledge returns one scope's LYRA.md content. Dispatch has already
// validated the scope (KnowledgeScope.Valid).
func (s *Server) GetKnowledge(ctx context.Context, in protocol.GetKnowledgeRequest) (*protocol.KnowledgeEntry, error) {
	scope, err := knowledgeScopeFromWire(in.Scope)
	if err != nil {
		return nil, err
	}
	entry, err := s.workspaceKnowledge.Read(ctx, scope, workspaceRefPath(in.Workspace))
	if err != nil {
		return nil, wireWorkspaceError(err)
	}
	wire := presentKnowledgeEntry(entry)
	return &wire, nil
}

func (s *Server) UpdateKnowledge(ctx context.Context, in protocol.UpdateKnowledgeRequest) (*protocol.KnowledgeEntry, error) {
	scope, err := knowledgeScopeFromWire(in.Scope)
	if err != nil {
		return nil, err
	}
	entry, err := s.workspaceKnowledge.Update(
		ctx, scope, workspaceRefPath(in.Workspace), in.ExpectedRevision, in.Content,
	)
	if err != nil {
		if errors.Is(err, knowledge.ErrRevisionConflict) {
			return nil, fmt.Errorf("%w: the knowledge document changed after it was read", protocol.ErrRevisionConflict)
		}
		if errors.Is(err, knowledge.ErrRevisionRequired) {
			return nil, fmt.Errorf("%w: %w", protocol.ErrInvalidParams, err)
		}
		return nil, wireWorkspaceError(err)
	}
	wire := presentKnowledgeEntry(entry)
	return &wire, nil
}

func presentKnowledgeEntry(entry knowledge.Entry) protocol.KnowledgeEntry {
	return protocol.KnowledgeEntry{
		Scope: protocol.KnowledgeScope(entry.Scope), Content: entry.Content,
		Revision: entry.Revision, UpdatedAt: entry.UpdatedAt,
	}
}

// presentKnowledgeScope / knowledgeScopeFromWire bridge the protocol and Domain
// closed vocabularies without collapsing distinct cascade locations.
func presentKnowledgeScope(scope knowledge.Scope) (protocol.KnowledgeScope, error) {
	switch scope {
	case knowledge.ScopeCWD:
		return protocol.KnowledgeScopeCWD, nil
	case knowledge.ScopeProjectRoot:
		return protocol.KnowledgeScopeProjectRoot, nil
	case knowledge.ScopeHome:
		return protocol.KnowledgeScopeHome, nil
	default:
		return "", fmt.Errorf("knowledge: unsupported knowledge scope %q", scope)
	}
}

func knowledgeScopeFromWire(scope protocol.KnowledgeScope) (knowledge.Scope, error) {
	switch scope {
	case protocol.KnowledgeScopeCWD:
		return knowledge.ScopeCWD, nil
	case protocol.KnowledgeScopeProjectRoot:
		return knowledge.ScopeProjectRoot, nil
	case protocol.KnowledgeScopeHome:
		return knowledge.ScopeHome, nil
	default:
		return "", fmt.Errorf("%w: unknown knowledge scope %q", protocol.ErrInvalidParams, scope)
	}
}

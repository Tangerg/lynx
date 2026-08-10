package server

import (
	"context"
	"fmt"

	workspaceapp "github.com/Tangerg/lynx/app/runtime/internal/application/workspace"
	"github.com/Tangerg/lynx/app/runtime/protocol"
)

// ListAgentDocs maps the application-owned instruction-document cascade onto
// the protocol shape.
func (s *Server) ListAgentDocs(ctx context.Context, in protocol.WorkspaceQuery) (*protocol.Page[protocol.AgentDoc], error) {
	docs, err := s.workspaceDiscovery.AgentDocs(ctx, in.Workspace.Path)
	if err != nil {
		return nil, wireWorkspaceError(err)
	}
	out := make([]protocol.AgentDoc, 0, len(docs))
	for _, doc := range docs {
		scope, ok := presentAgentDocScope(doc.Scope)
		if !ok {
			return nil, fmt.Errorf("agentDocs.list: unsupported document scope %q", doc.Scope)
		}
		out = append(out, protocol.AgentDoc{Path: doc.Path, Scope: scope})
	}
	return protocol.NewPage(out), nil
}

func presentAgentDocScope(scope workspaceapp.AgentDocScope) (protocol.AgentDocScope, bool) {
	switch scope {
	case workspaceapp.AgentDocScopeCWD:
		return protocol.AgentDocScopeCWD, true
	case workspaceapp.AgentDocScopeProjectRoot:
		return protocol.AgentDocScopeProjectRoot, true
	case workspaceapp.AgentDocScopeHome:
		return protocol.AgentDocScopeHome, true
	default:
		return "", false
	}
}

package server

import (
	"context"

	"github.com/Tangerg/lynx/lyra/rpc/protocol"
)

// workspace.* (API.md §7.5) — no git / ripgrep probes wired into the
// engine yet. List endpoints return an EMPTY slice so frontend panels
// render an empty state instead of an error; the specific-resource
// reads (diff / fileHead / grep / mcp.reconnect) stay notImpl until the
// engine grows the corresponding probe.

func (i *Server) WorkspaceListFileChanges(_ context.Context, _ protocol.WorkspaceQuery) ([]protocol.FileChange, error) {
	return []protocol.FileChange{}, nil
}

func (i *Server) WorkspaceGetDiff(_ context.Context, _ protocol.GetDiffRequest) ([]protocol.DiffRow, error) {
	return nil, notImpl("workspace.getDiff")
}

func (i *Server) WorkspaceGetFileHead(_ context.Context, _ protocol.GetFileHeadRequest) (*protocol.FileHead, error) {
	return nil, notImpl("workspace.getFileHead")
}

func (i *Server) WorkspaceGrep(_ context.Context, _ protocol.GrepRequest) (*protocol.GrepResult, error) {
	return nil, notImpl("workspace.grep")
}

func (i *Server) WorkspaceListProjects(_ context.Context) ([]protocol.Project, error) {
	return []protocol.Project{}, nil
}

func (i *Server) WorkspaceListSkills(_ context.Context, _ protocol.WorkspaceQuery) ([]protocol.Skill, error) {
	return []protocol.Skill{}, nil
}

func (i *Server) WorkspaceListAgentDocs(_ context.Context, _ protocol.WorkspaceQuery) ([]protocol.AgentDoc, error) {
	return []protocol.AgentDoc{}, nil
}

func (i *Server) WorkspaceMCPListServers(_ context.Context) ([]protocol.McpServer, error) {
	return []protocol.McpServer{}, nil
}

func (i *Server) WorkspaceMCPListTools(_ context.Context, _ protocol.MCPListToolsRequest) ([]protocol.McpTool, error) {
	return []protocol.McpTool{}, nil
}

func (i *Server) WorkspaceMCPReconnect(_ context.Context, _ string) error {
	return notImpl("workspace.mcp.reconnect")
}

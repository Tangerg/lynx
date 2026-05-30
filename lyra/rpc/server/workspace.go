package server

import (
	"context"

	"github.com/Tangerg/lynx/lyra/rpc/protocol"
)

// workspace.* — no real workspace probes yet (git diff / ripgrep / pty
// not wired into the engine). The three round-1 P0 list endpoints
// (filesChanged / projects / mcp.list) return an EMPTY slice so the
// frontend panels render an empty state instead of eating -32601; the
// rest stay notImpl until the engine grows the corresponding probe.

func (i *Server) WorkspaceFilesChanged(_ context.Context) ([]protocol.FileChange, error) {
	return []protocol.FileChange{}, nil
}

func (i *Server) WorkspaceDiff(_ context.Context, _ string) ([]protocol.DiffRow, error) {
	return nil, notImpl("workspace.diff")
}

func (i *Server) WorkspaceFileHead(_ context.Context, _ string) ([]protocol.FileLine, error) {
	return nil, notImpl("workspace.fileHead")
}

func (i *Server) WorkspaceGrep(_ context.Context, _ string) (*protocol.GrepResult, error) {
	return nil, notImpl("workspace.grep")
}

func (i *Server) WorkspaceTerminalSubscribe(_ context.Context, _ string) (<-chan protocol.TermLine, error) {
	return nil, notImpl("workspace.terminal.subscribe")
}

func (i *Server) WorkspaceProjects(_ context.Context) ([]protocol.Project, error) {
	return []protocol.Project{}, nil
}

func (i *Server) WorkspaceMCPList(_ context.Context) ([]protocol.MCPServer, error) {
	return []protocol.MCPServer{}, nil
}

func (i *Server) WorkspaceMCPReconnect(_ context.Context, _ string) error {
	return notImpl("workspace.mcp.reconnect")
}

func (i *Server) WorkspaceSkills(_ context.Context) ([]protocol.Skill, error) {
	return nil, notImpl("workspace.skills")
}

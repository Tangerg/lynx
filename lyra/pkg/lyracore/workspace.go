package lyracore

import (
	"context"

	"github.com/Tangerg/lynx/lyra/pkg/coreapi"
)

// workspace.* — none of the workspace endpoints have a runtime
// equivalent today. The frontend reads from fixture data; once the
// engine grows real workspace probes (git diff / ripgrep / pty)
// these stubs come down.

func (i *Server) WorkspaceFilesChanged(_ context.Context) ([]coreapi.FileChange, error) {
	return nil, notImpl("workspace.filesChanged")
}

func (i *Server) WorkspaceDiff(_ context.Context, _ string) ([]coreapi.DiffRow, error) {
	return nil, notImpl("workspace.diff")
}

func (i *Server) WorkspaceFileHead(_ context.Context, _ string) ([]coreapi.FileLine, error) {
	return nil, notImpl("workspace.fileHead")
}

func (i *Server) WorkspaceGrep(_ context.Context, _ string) (*coreapi.GrepResult, error) {
	return nil, notImpl("workspace.grep")
}

func (i *Server) WorkspaceTerminalSubscribe(_ context.Context, _ string) (<-chan coreapi.TermLine, error) {
	return nil, notImpl("workspace.terminal.subscribe")
}

func (i *Server) WorkspaceProjects(_ context.Context) ([]coreapi.Project, error) {
	return nil, notImpl("workspace.projects")
}

func (i *Server) WorkspaceMCPList(_ context.Context) ([]coreapi.MCPServer, error) {
	return nil, notImpl("workspace.mcp.list")
}

func (i *Server) WorkspaceMCPReconnect(_ context.Context, _ string) error {
	return notImpl("workspace.mcp.reconnect")
}

func (i *Server) WorkspaceSkills(_ context.Context) ([]coreapi.Skill, error) {
	return nil, notImpl("workspace.skills")
}

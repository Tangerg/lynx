package coreimpl

import (
	"context"

	"github.com/Tangerg/lynx/lyra/pkg/coreapi"
)

// workspace.* — none of the workspace endpoints have a runtime
// equivalent today. The frontend reads from fixture data; once the
// engine grows real workspace probes (git diff / ripgrep / pty)
// these stubs come down.

func (i *Impl) WorkspaceFilesChanged(_ context.Context) ([]coreapi.FileChange, error) {
	return nil, notImpl("workspace.filesChanged")
}

func (i *Impl) WorkspaceDiff(_ context.Context, _ string) ([]coreapi.DiffRow, error) {
	return nil, notImpl("workspace.diff")
}

func (i *Impl) WorkspaceFileHead(_ context.Context, _ string) ([]coreapi.FileLine, error) {
	return nil, notImpl("workspace.fileHead")
}

func (i *Impl) WorkspaceGrep(_ context.Context, _ string) (*coreapi.GrepResult, error) {
	return nil, notImpl("workspace.grep")
}

func (i *Impl) WorkspaceTerminalSubscribe(_ context.Context, _ string) (<-chan coreapi.TermLine, error) {
	return nil, notImpl("workspace.terminal.subscribe")
}

func (i *Impl) WorkspaceProjects(_ context.Context) ([]coreapi.Project, error) {
	return nil, notImpl("workspace.projects")
}

func (i *Impl) WorkspaceMCPList(_ context.Context) ([]coreapi.MCPServer, error) {
	return nil, notImpl("workspace.mcp.list")
}

func (i *Impl) WorkspaceMCPReconnect(_ context.Context, _ string) error {
	return notImpl("workspace.mcp.reconnect")
}

func (i *Impl) WorkspaceSkills(_ context.Context) ([]coreapi.Skill, error) {
	return nil, notImpl("workspace.skills")
}

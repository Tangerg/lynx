package server

import (
	"context"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Tangerg/lynx/lyra/internal/service/agentdoc"
	"github.com/Tangerg/lynx/lyra/internal/service/session"
	"github.com/Tangerg/lynx/lyra/rpc/protocol"
)

// workspace.* (API.md §7.5). listProjects + listAgentDocs are real
// (derived from sessions / AGENTS.md discovery); the git/ripgrep-backed
// reads (listFileChanges / getDiff / getFileHead / grep / mcp.reconnect)
// stay notImpl until the engine grows the corresponding probe.

func (i *Server) WorkspaceListFileChanges(_ context.Context, _ protocol.WorkspaceListQuery) (*protocol.Page[protocol.WorkspaceFileChange], error) {
	return nil, notImpl("workspace.listFileChanges")
}

func (i *Server) WorkspaceGetDiff(_ context.Context, _ protocol.GetDiffRequest) (*protocol.Diff, error) {
	return nil, notImpl("workspace.getDiff")
}

func (i *Server) WorkspaceGetFileHead(_ context.Context, _ protocol.GetFileHeadRequest) (*protocol.FileHead, error) {
	return nil, notImpl("workspace.getFileHead")
}

func (i *Server) WorkspaceGrep(_ context.Context, _ protocol.GrepRequest) (*protocol.GrepResult, error) {
	return nil, notImpl("workspace.grep")
}

// WorkspaceListProjects derives the Project view from sessions: one
// entry per distinct Session.cwd (API.md §0.2 / §7.5), newest-active
// first. projectRoot / branch are best-effort decorations left empty
// until the engine grows a git probe.
func (i *Server) WorkspaceListProjects(ctx context.Context, _ protocol.PageQuery) (*protocol.Page[protocol.Project], error) {
	sessions, err := i.rt.Session().List(ctx)
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

// WorkspaceListSkills is gated off (features.skills=false) — return
// capability_not_negotiated rather than a misleading empty list, until
// the engine grows skill discovery.
func (i *Server) WorkspaceListSkills(_ context.Context, _ protocol.WorkspaceListQuery) (*protocol.Page[protocol.Skill], error) {
	return nil, notImpl("workspace.listSkills")
}

// WorkspaceListAgentDocs lists the AGENTS.md files discovered from cwd
// (or the serve cwd) up to home — the same cascade the engine injects
// into the system prompt (API.md §7.5).
func (i *Server) WorkspaceListAgentDocs(ctx context.Context, q protocol.WorkspaceListQuery) (*protocol.Page[protocol.AgentDoc], error) {
	cwd := q.Cwd
	if cwd == "" {
		cwd = i.serverInfo.Cwd
	}
	home := i.serverInfo.Home
	files, err := agentdoc.Discover(ctx, cwd, home)
	if err != nil {
		return nil, err
	}
	out := make([]protocol.AgentDoc, 0, len(files))
	for _, f := range files {
		out = append(out, protocol.AgentDoc{Path: f.Path, Scope: agentDocScope(f.Path, cwd, home)})
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

// WorkspaceMCPListServers lists the MCP servers dialed at startup. They
// are all "connected" — a dial failure fails runtime construction, so a
// running server only knows connected ones (API.md §7.5).
func (i *Server) WorkspaceMCPListServers(_ context.Context, _ protocol.PageQuery) (*protocol.Page[protocol.McpServer], error) {
	names := i.rt.MCPServerNames()
	out := make([]protocol.McpServer, 0, len(names))
	for _, n := range names {
		out = append(out, protocol.McpServer{Name: n, Status: "connected"})
	}
	return protocol.NewPage(out), nil
}

// WorkspaceMCPListTools — per-server MCP tool enumeration isn't wired
// yet (MCP tools merge into the engine's flat tool set, surfaced via
// tools.list; segmenting them by server needs an engine accessor).
func (i *Server) WorkspaceMCPListTools(_ context.Context, _ protocol.MCPListToolsRequest) (*protocol.Page[protocol.McpTool], error) {
	return nil, notImpl("workspace.mcp.listTools")
}

func (i *Server) WorkspaceMCPReconnect(_ context.Context, _ string) error {
	return notImpl("workspace.mcp.reconnect")
}

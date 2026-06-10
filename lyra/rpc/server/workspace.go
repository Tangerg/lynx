package server

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Tangerg/lynx/lyra/internal/service/agentdoc"
	"github.com/Tangerg/lynx/lyra/internal/service/session"
	"github.com/Tangerg/lynx/lyra/rpc/protocol"
	"github.com/Tangerg/lynx/tools/fs"
)

// defaultFileHeadLines caps a workspace.getFileHead preview when the client
// gives no (or a non-positive) line count.
const defaultFileHeadLines = 200

// workspaceRoot resolves the effective root for a workspace read: the
// request's cwd, or the serve directory when omitted (API.md §7.5 "default =
// serve directory"). It returns ErrCwdUnavailable when the root doesn't
// resolve to an existing directory, so reads against a stale cwd fail
// honestly rather than returning empty.
func (s *Server) workspaceRoot(cwd string) (string, error) {
	root := cwd
	if root == "" {
		root = s.serverInfo.Cwd
	}
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return "", fmt.Errorf("%w: %s", protocol.ErrCwdUnavailable, root)
	}
	return root, nil
}

// resolveInRoot lexically confines a client-supplied path to root and returns
// it relative to root (the form fs.LocalExecutor wants). It is the path-jail
// fs.LocalExecutor itself doesn't enforce (its Root only anchors; "../" and
// absolute paths escape — see tools/fs local.go TODO(security)). Absolute
// paths are accepted only when already inside root; anything climbing out
// (or "..") is rejected as path_outside_root (API.md §7.5).
func resolveInRoot(root, p string) (rel string, err error) {
	if p == "" {
		return "", fmt.Errorf("%w: path required", protocol.ErrInvalidParams)
	}
	abs := p
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(root, p)
	}
	abs = filepath.Clean(abs)
	rel, err = filepath.Rel(root, abs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", protocol.ErrPathOutsideRoot
	}
	return rel, nil
}

// workspace.* (API.md §7.5). listProjects + listAgentDocs are real
// (derived from sessions / AGENTS.md discovery); the git/ripgrep-backed
// reads (listFileChanges / getDiff / getFileHead / grep / mcp.reconnect)
// stay notImpl until the engine grows the corresponding probe.

func (s *Server) WorkspaceListFileChanges(_ context.Context, _ protocol.WorkspaceListQuery) (*protocol.Page[protocol.WorkspaceFileChange], error) {
	return nil, notImpl("workspace.listFileChanges")
}

func (s *Server) WorkspaceGetDiff(_ context.Context, _ protocol.GetDiffRequest) (*protocol.Diff, error) {
	return nil, notImpl("workspace.getDiff")
}

// WorkspaceGetFileHead returns the first N lines of a cwd-relative file
// (API.md §7.5). The path is jailed to the workspace root (resolveInRoot);
// binary files surface fs.ErrBinaryFile. Lines default to defaultFileHeadLines.
func (s *Server) WorkspaceGetFileHead(ctx context.Context, in protocol.GetFileHeadRequest) (*protocol.FileHead, error) {
	root, err := s.workspaceRoot(in.Cwd)
	if err != nil {
		return nil, err
	}
	rel, err := resolveInRoot(root, in.Path)
	if err != nil {
		return nil, err
	}
	lines := in.Lines
	if lines <= 0 {
		lines = defaultFileHeadLines
	}
	out, err := fs.NewLocalExecutor(root).Read(ctx, fs.ReadInput{Path: rel, Limit: lines})
	if err != nil {
		return nil, err
	}
	return &protocol.FileHead{Path: in.Path, Lines: fileLines(out)}, nil
}

// fileLines splits a Read result into numbered preview lines. StartLine is
// 0-based; the wire LineNumber is 1-based. A read that windowed nothing (an
// empty file) yields no lines rather than one spurious blank.
func fileLines(out fs.ReadOutput) []protocol.FileLine {
	if out.Content == "" && out.TotalLines == 0 {
		return []protocol.FileLine{}
	}
	parts := strings.Split(out.Content, "\n")
	lines := make([]protocol.FileLine, 0, len(parts))
	for i, text := range parts {
		lines = append(lines, protocol.FileLine{LineNumber: out.StartLine + i + 1, Text: text})
	}
	return lines
}

// defaultGrepLimit caps workspace.grep matches when the client gives no
// (or a non-positive) limit.
const defaultGrepLimit = 100

// WorkspaceGrep runs a regex search under the workspace root (API.md §7.5),
// optionally scoped to a sub-path. Matches are capped at limit; Total is
// self-describing per §7.5's no-silent-caps rule — when the capped search
// truncates, a count-mode pass recovers the true total so Total > len(Matches)
// signals "more exist" rather than under-reporting.
func (s *Server) WorkspaceGrep(ctx context.Context, in protocol.GrepRequest) (*protocol.GrepResult, error) {
	if in.Query == "" {
		return nil, fmt.Errorf("%w: query required", protocol.ErrInvalidParams)
	}
	root, err := s.workspaceRoot(in.Cwd)
	if err != nil {
		return nil, err
	}
	searchRoot := root
	if in.Path != "" {
		rel, err := resolveInRoot(root, in.Path)
		if err != nil {
			return nil, err
		}
		searchRoot = filepath.Join(root, rel)
	}
	limit := in.Limit
	if limit <= 0 {
		limit = defaultGrepLimit
	}

	exec := fs.NewLocalExecutor(root)
	out, err := exec.Grep(ctx, fs.GrepInput{Pattern: in.Query, Root: searchRoot, MaxResults: limit})
	if err != nil {
		return nil, err
	}
	matches := make([]protocol.GrepMatch, 0, len(out.Matches))
	for _, m := range out.Matches {
		matches = append(matches, protocol.GrepMatch{Path: m.Path, LineNumber: m.Line, Text: m.Text})
	}

	total := len(matches)
	if out.Truncated {
		// The capped content search hid some hits; a count-mode pass gives the
		// honest total so the client sees total > len(matches) and knows to
		// narrow the query. Best-effort: if the count pass fails, fall back to
		// "at least one more" so we never claim completeness we don't have.
		if n, cerr := grepTotal(ctx, exec, in.Query, searchRoot); cerr == nil && n > total {
			total = n
		} else if total == limit {
			total = limit + 1
		}
	}
	return &protocol.GrepResult{Matches: matches, Total: total}, nil
}

// grepTotal counts every match for pattern under root (uncapped count mode),
// summing the per-file counts into one total.
func grepTotal(ctx context.Context, exec fs.Executor, pattern, root string) (int, error) {
	out, err := exec.Grep(ctx, fs.GrepInput{Pattern: pattern, Root: root, OutputMode: fs.GrepOutputCount})
	if err != nil {
		return 0, err
	}
	total := 0
	for _, c := range out.Counts {
		total += c.Count
	}
	return total, nil
}

// WorkspaceListProjects derives the Project view from sessions: one
// entry per distinct Session.cwd (API.md §0.2 / §7.5), newest-active
// first. projectRoot / branch are best-effort decorations left empty
// until the engine grows a git probe.
func (s *Server) WorkspaceListProjects(ctx context.Context, _ protocol.PageQuery) (*protocol.Page[protocol.Project], error) {
	sessions, err := s.rt.Session().List(ctx)
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

// WorkspaceListSkills enumerates the skills visible from cwd — project skills
// (<cwd>/.lyra/skills) layered over the global directory, project winning on a
// name collision (the same set + precedence the engine gives the model). Each
// entry's Source records its scope ("project" | "global"). cwd defaults to the
// serve directory (API.md §7.5).
func (s *Server) WorkspaceListSkills(ctx context.Context, in protocol.WorkspaceListQuery) (*protocol.Page[protocol.Skill], error) {
	root, err := s.workspaceRoot(in.Cwd)
	if err != nil {
		return nil, err
	}
	found, err := s.rt.ListSkills(ctx, root)
	if err != nil {
		return nil, err
	}
	out := make([]protocol.Skill, 0, len(found))
	for _, sk := range found {
		out = append(out, protocol.Skill{Name: sk.Name, Description: sk.Description, Source: sk.Scope})
	}
	return protocol.NewPage(out), nil
}

// WorkspaceListAgentDocs lists the AGENTS.md files discovered from cwd
// (or the serve cwd) up to home — the same cascade the engine injects
// into the system prompt (API.md §7.5).
func (s *Server) WorkspaceListAgentDocs(ctx context.Context, q protocol.WorkspaceListQuery) (*protocol.Page[protocol.AgentDoc], error) {
	cwd := q.Cwd
	if cwd == "" {
		cwd = s.serverInfo.Cwd
	}
	home := s.serverInfo.Home
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
func (s *Server) WorkspaceMCPListServers(_ context.Context, _ protocol.PageQuery) (*protocol.Page[protocol.McpServer], error) {
	names := s.rt.MCPServerNames()
	out := make([]protocol.McpServer, 0, len(names))
	for _, n := range names {
		out = append(out, protocol.McpServer{Name: n, Status: "connected"})
	}
	return protocol.NewPage(out), nil
}

// WorkspaceMCPListTools — per-server MCP tool enumeration isn't wired
// yet (MCP tools merge into the engine's flat tool set, surfaced via
// tools.list; segmenting them by server needs an engine accessor).
func (s *Server) WorkspaceMCPListTools(_ context.Context, _ protocol.MCPListToolsRequest) (*protocol.Page[protocol.McpTool], error) {
	return nil, notImpl("workspace.mcp.listTools")
}

func (s *Server) WorkspaceMCPReconnect(_ context.Context, _ string) error {
	return notImpl("workspace.mcp.reconnect")
}

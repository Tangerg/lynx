package server

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/Tangerg/lynx/lyra/internal/engine"
	"github.com/Tangerg/lynx/lyra/internal/git"
	"github.com/Tangerg/lynx/lyra/internal/service/session"
	"github.com/Tangerg/lynx/lyra/rpc/protocol"
)

// TestProjectsFromSessions: distinct cwds collapse to one project each,
// session-counted, lastActiveAt = newest session, ordered newest-first;
// empty-cwd sessions are dropped.
func TestProjectsFromSessions(t *testing.T) {
	t0 := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	sessions := []session.Session{
		{ID: "s1", Cwd: "/a/proj", UpdatedAt: t0},
		{ID: "s2", Cwd: "/a/proj", UpdatedAt: t0.Add(2 * time.Hour)}, // newer in /a/proj
		{ID: "s3", Cwd: "/b/other", UpdatedAt: t0.Add(time.Hour)},
		{ID: "s4", Cwd: "", UpdatedAt: t0}, // no cwd → dropped
	}

	got := projectsFromSessions(sessions)
	if len(got) != 2 {
		t.Fatalf("projects = %d, want 2 (empty-cwd dropped)", len(got))
	}
	// newest-active first: /a/proj (t0+2h) before /b/other (t0+1h)
	if got[0].Cwd != "/a/proj" || got[0].Name != "proj" || got[0].SessionCount != 2 {
		t.Fatalf("got[0] = %+v, want /a/proj name=proj count=2", got[0])
	}
	if !got[0].LastActiveAt.Equal(t0.Add(2 * time.Hour)) {
		t.Fatalf("got[0].LastActiveAt = %v, want newest session time", got[0].LastActiveAt)
	}
	if got[1].Cwd != "/b/other" || got[1].SessionCount != 1 {
		t.Fatalf("got[1] = %+v, want /b/other count=1", got[1])
	}
}

// TestResolveInRoot pins the path-jail: in-root paths resolve relative to
// root; anything climbing out (or an absolute path elsewhere) is rejected.
func TestResolveInRoot(t *testing.T) {
	root := "/work/proj"
	ok := []struct{ in, want string }{
		{"main.go", "main.go"},
		{"pkg/util.go", "pkg/util.go"},
		{"./a/../b.go", "b.go"},
		{"/work/proj/sub/x.go", "sub/x.go"}, // absolute but inside root
	}
	for _, c := range ok {
		got, err := resolveInRoot(root, c.in)
		if err != nil || got != c.want {
			t.Errorf("resolveInRoot(%q) = (%q, %v), want (%q, nil)", c.in, got, err, c.want)
		}
	}
	bad := []string{"../escape.go", "../../etc/passwd", "/etc/passwd", "sub/../../out.go"}
	for _, p := range bad {
		if _, err := resolveInRoot(root, p); !errors.Is(err, protocol.ErrPathOutsideRoot) {
			t.Errorf("resolveInRoot(%q) err = %v, want ErrPathOutsideRoot", p, err)
		}
	}
	if _, err := resolveInRoot(root, ""); !errors.Is(err, protocol.ErrInvalidParams) {
		t.Errorf("resolveInRoot(\"\") err = %v, want ErrInvalidParams", err)
	}
}

// TestWorkspaceGetFileHead reads the first N lines of a cwd-relative file,
// numbers them 1-based, and refuses a path that climbs out of the root.
func TestWorkspaceGetFileHead(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "f.txt"), []byte("a\nb\nc\nd\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := &Server{serverInfo: protocol.ServerInfo{Cwd: dir}}

	got, err := s.WorkspaceGetFileHead(context.Background(), protocol.GetFileHeadRequest{Path: "f.txt", Lines: 2})
	if err != nil {
		t.Fatalf("getFileHead: %v", err)
	}
	if len(got.Lines) != 2 || got.Lines[0].LineNumber != 1 || got.Lines[0].Text != "a" || got.Lines[1].LineNumber != 2 {
		t.Fatalf("lines = %+v, want first two lines numbered 1,2", got.Lines)
	}

	if _, err := s.WorkspaceGetFileHead(context.Background(), protocol.GetFileHeadRequest{Path: "../escape"}); !errors.Is(err, protocol.ErrPathOutsideRoot) {
		t.Errorf("escape path err = %v, want ErrPathOutsideRoot", err)
	}
}

// TestWorkspaceGrep searches the workspace root, requires a query, and jails
// the optional sub-path. Depends on rg or grep being on PATH (skips if not).
func TestWorkspaceGrep(t *testing.T) {
	dir := t.TempDir()
	for name, body := range map[string]string{
		"a.go": "package a\nfunc Needle() {}\n",
		"b.go": "package b\n// no match here\n",
		"c.go": "package c\nvar Needle = 1\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	s := &Server{serverInfo: protocol.ServerInfo{Cwd: dir}}

	if _, err := s.WorkspaceGrep(context.Background(), protocol.GrepRequest{}); !errors.Is(err, protocol.ErrInvalidParams) {
		t.Errorf("empty query err = %v, want ErrInvalidParams", err)
	}

	got, err := s.WorkspaceGrep(context.Background(), protocol.GrepRequest{Query: "Needle"})
	if err != nil {
		t.Skipf("grep backend unavailable: %v", err) // no rg/grep on PATH
	}
	if got.Total != 2 || len(got.Matches) != 2 {
		t.Fatalf("grep Needle = %d matches / total %d, want 2/2", len(got.Matches), got.Total)
	}

	if _, err := s.WorkspaceGrep(context.Background(), protocol.GrepRequest{Query: "x", Path: "../out"}); !errors.Is(err, protocol.ErrPathOutsideRoot) {
		t.Errorf("escape path err = %v, want ErrPathOutsideRoot", err)
	}
}

// TestWorkspaceListSkills maps the engine's discovered skills onto the wire,
// carrying each one's scope through Source, and defaults cwd to the serve dir.
func TestWorkspaceListSkills(t *testing.T) {
	dir := t.TempDir()
	s := &Server{
		serverInfo: protocol.ServerInfo{Cwd: dir},
		rt: stubRuntime{skills: []engine.SkillInfo{
			{Name: "pdf", Description: "PDF tools", Scope: "project"},
			{Name: "web", Description: "web tools", Scope: "global"},
		}},
	}
	got, err := s.WorkspaceListSkills(context.Background(), protocol.WorkspaceListQuery{})
	if err != nil {
		t.Fatalf("listSkills: %v", err)
	}
	if len(got.Data) != 2 || got.Data[0].Name != "pdf" || got.Data[0].Source != "project" || got.Data[1].Source != "global" {
		t.Fatalf("skills = %+v, want pdf(project) + web(global)", got.Data)
	}
}

// TestWorkspaceMCPListServers renders each server's real status (AUX_API §5.1):
// a connected server inlines its tool count (so the client needn't ⨝ listTools),
// a boot-failed server carries its failure reason as Error and no tool count.
func TestWorkspaceMCPListServers(t *testing.T) {
	s := &Server{rt: stubRuntime{
		mcpStatuses: []engine.McpServerStatus{
			{Name: "fs", Status: "connected"},
			{Name: "down", Status: "failed", Err: errors.New("connection refused")},
		},
		mcpTools: []engine.McpToolInfo{
			{Server: "fs", Name: "read"}, {Server: "fs", Name: "write"},
		},
	}}
	page, err := s.WorkspaceMCPListServers(context.Background(), protocol.PageQuery{})
	if err != nil {
		t.Fatalf("listServers: %v", err)
	}
	if len(page.Data) != 2 {
		t.Fatalf("servers = %+v, want 2 (connected + failed)", page.Data)
	}
	fs := page.Data[0]
	if fs.Status != "connected" || fs.ToolCount == nil || *fs.ToolCount != 2 || fs.Error != nil {
		t.Fatalf("fs = %+v, want connected toolCount=2 no error", fs)
	}
	down := page.Data[1]
	if down.Status != "failed" || down.ToolCount != nil || down.Error == nil || down.Error.Detail != "connection refused" {
		t.Fatalf("down = %+v, want failed + Error(connection refused) no toolCount", down)
	}
}

// TestWorkspaceMCPListTools maps engine tool info onto the wire (keeping
// server + bare name separate) and passes the server scope through.
func TestWorkspaceMCPListTools(t *testing.T) {
	s := &Server{rt: stubRuntime{mcpTools: []engine.McpToolInfo{
		{Server: "fs", Name: "read", Description: "read a file", InputSchema: map[string]any{"type": "object"}},
		{Server: "fs", Name: "write"},
		{Server: "git", Name: "log"},
	}}}

	all, err := s.WorkspaceMCPListTools(context.Background(), protocol.MCPListToolsRequest{})
	if err != nil {
		t.Fatalf("listTools: %v", err)
	}
	if len(all.Data) != 3 || all.Data[0].Server != "fs" || all.Data[0].Name != "read" || all.Data[0].InputSchema["type"] != "object" {
		t.Fatalf("all = %+v, want 3 with fs/read carrying its schema", all.Data)
	}

	scoped, err := s.WorkspaceMCPListTools(context.Background(), protocol.MCPListToolsRequest{Server: "git"})
	if err != nil {
		t.Fatalf("listTools(git): %v", err)
	}
	if len(scoped.Data) != 1 || scoped.Data[0].Server != "git" {
		t.Fatalf("scoped = %+v, want only git tools", scoped.Data)
	}
}

// TestWorkspaceVcsUnavailable: git present but cwd is not a repo → both git
// methods report vcs_unavailable (distinct from "clean repo" = empty result).
func TestWorkspaceVcsUnavailable(t *testing.T) {
	if !git.Available() {
		t.Skip("git not on PATH")
	}
	s := &Server{serverInfo: protocol.ServerInfo{Cwd: t.TempDir()}} // exists, not a repo
	if _, err := s.WorkspaceListFileChanges(context.Background(), protocol.WorkspaceListQuery{}); !errors.Is(err, protocol.ErrVcsUnavailable) {
		t.Errorf("listFileChanges err = %v, want ErrVcsUnavailable", err)
	}
	if _, err := s.WorkspaceGetDiff(context.Background(), protocol.GetDiffRequest{}); !errors.Is(err, protocol.ErrVcsUnavailable) {
		t.Errorf("getDiff err = %v, want ErrVcsUnavailable", err)
	}
}

// TestWorkspaceGitWireMapping: a real repo with one modified file maps onto the
// wire with non-nil added/removed (non-binary), and getDiff returns rows.
func TestWorkspaceGitWireMapping(t *testing.T) {
	if !git.Available() {
		t.Skip("git not on PATH")
	}
	dir := t.TempDir()
	env := append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	gitCmd := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir, cmd.Env = dir, env
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	gitCmd("init", "-b", "main")
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a\nb\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitCmd("add", ".")
	gitCmd("commit", "-m", "init")
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a\nB\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	s := &Server{serverInfo: protocol.ServerInfo{Cwd: dir}}
	page, err := s.WorkspaceListFileChanges(context.Background(), protocol.WorkspaceListQuery{})
	if err != nil {
		t.Fatalf("listFileChanges: %v", err)
	}
	if len(page.Data) != 1 || page.Data[0].Status != "modified" || page.Data[0].Added == nil {
		t.Fatalf("changes = %+v, want one modified with non-nil added", page.Data)
	}

	diff, err := s.WorkspaceGetDiff(context.Background(), protocol.GetDiffRequest{})
	if err != nil {
		t.Fatalf("getDiff: %v", err)
	}
	if len(diff.Files) != 1 || len(diff.Files[0].Rows) == 0 {
		t.Fatalf("diff = %+v, want one file with rows", diff.Files)
	}
}

// TestWorkspaceSubscribe: a watch-less subscribe receives published events and
// closes on ctx cancel; a subscribe carrying watches is rejected while
// features.fileWatch is off.
func TestWorkspaceSubscribe(t *testing.T) {
	s := &Server{wsHub: newWorkspaceHub()}

	if _, _, err := s.WorkspaceSubscribe(context.Background(), protocol.WorkspaceSubscribeRequest{
		Watches: []protocol.WatchSpec{{WatchID: "w", Path: "."}},
	}); !errors.Is(err, protocol.ErrCapabilityNotNeg) {
		t.Errorf("watches err = %v, want ErrCapabilityNotNeg", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, events, err := s.WorkspaceSubscribe(ctx, protocol.WorkspaceSubscribeRequest{})
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	s.PublishWorkspaceEvent(protocol.WorkspaceEvent{Type: "skills.changed"})
	select {
	case ev := <-events:
		if ev.Type != "skills.changed" {
			t.Fatalf("event = %+v, want skills.changed", ev)
		}
	case <-time.After(time.Second):
		t.Fatal("no event received")
	}

	cancel() // ctx done → unsubscribe closes the channel
	select {
	case _, ok := <-events:
		for ok { // drain any buffered, then expect close
			_, ok = <-events
		}
	case <-time.After(time.Second):
		t.Fatal("channel not closed after ctx cancel")
	}
}

// TestAgentDocScope pins the cwd→home cascade classification.
func TestAgentDocScope(t *testing.T) {
	cwd, home := "/Users/x/proj", "/Users/x"
	cases := []struct {
		path, want string
	}{
		{"/Users/x/proj/AGENTS.md", "cwd"},
		{"/Users/x/proj/pkg/AGENTS.md", "cwd"},
		{"/Users/x/AGENTS.md", "home"},
		{"/Users/x/mid/AGENTS.md", "projectRoot"}, // ancestor between cwd and home
	}
	for _, c := range cases {
		if got := agentDocScope(c.path, cwd, home); got != c.want {
			t.Errorf("agentDocScope(%q) = %q, want %q", c.path, got, c.want)
		}
	}
}

package server

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

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

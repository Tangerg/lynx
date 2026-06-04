package server

import (
	"testing"
	"time"

	"github.com/Tangerg/lynx/lyra/internal/service/session"
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

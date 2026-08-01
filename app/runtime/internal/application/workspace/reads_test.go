package workspace

import (
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
)

func TestWorkspacesFromSessions(t *testing.T) {
	t0 := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	workspaces := workspacesFromSessions([]session.Session{
		{ID: "s1", Cwd: "/a/proj", UpdatedAt: t0},
		{ID: "s2", Cwd: "/a/proj", UpdatedAt: t0.Add(2 * time.Hour)},
		{ID: "s3", Cwd: "/b/other", UpdatedAt: t0.Add(time.Hour)},
		{ID: "s4", UpdatedAt: t0},
	})
	if len(workspaces) != 2 {
		t.Fatalf("workspaces = %d, want 2", len(workspaces))
	}
	if workspaces[0].Path != "/a/proj" || workspaces[0].Name != "proj" || workspaces[0].SessionCount != 2 {
		t.Fatalf("first workspace = %+v", workspaces[0])
	}
	if !workspaces[0].LastActiveAt.Equal(t0.Add(2 * time.Hour)) {
		t.Fatalf("last active = %v", workspaces[0].LastActiveAt)
	}
	if workspaces[1].Path != "/b/other" || workspaces[1].SessionCount != 1 {
		t.Fatalf("second workspace = %+v", workspaces[1])
	}
}

func TestAgentDocScope(t *testing.T) {
	cwd, home := "/Users/x/proj", "/Users/x"
	cases := []struct {
		path string
		want AgentDocScope
	}{
		{"/Users/x/proj/AGENTS.md", "cwd"},
		{"/Users/x/proj/pkg/AGENTS.md", "cwd"},
		{"/Users/x/AGENTS.md", "home"},
		{"/Users/x/mid/AGENTS.md", "projectRoot"},
	}
	for _, test := range cases {
		if got := agentDocScope(test.path, cwd, home); got != test.want {
			t.Errorf("agentDocScope(%q) = %q, want %q", test.path, got, test.want)
		}
	}
}

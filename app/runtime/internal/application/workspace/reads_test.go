package workspace

import (
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
)

func TestProjectsFromSessions(t *testing.T) {
	t0 := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	projects := projectsFromSessions([]session.Session{
		{ID: "s1", Cwd: "/a/proj", UpdatedAt: t0},
		{ID: "s2", Cwd: "/a/proj", UpdatedAt: t0.Add(2 * time.Hour)},
		{ID: "s3", Cwd: "/b/other", UpdatedAt: t0.Add(time.Hour)},
		{ID: "s4", UpdatedAt: t0},
	})
	if len(projects) != 2 {
		t.Fatalf("projects = %d, want 2", len(projects))
	}
	if projects[0].Cwd != "/a/proj" || projects[0].Name != "proj" || projects[0].SessionCount != 2 {
		t.Fatalf("first project = %+v", projects[0])
	}
	if !projects[0].LastActiveAt.Equal(t0.Add(2 * time.Hour)) {
		t.Fatalf("last active = %v", projects[0].LastActiveAt)
	}
	if projects[1].Cwd != "/b/other" || projects[1].SessionCount != 1 {
		t.Fatalf("second project = %+v", projects[1])
	}
}

func TestAgentDocScope(t *testing.T) {
	cwd, home := "/Users/x/proj", "/Users/x"
	cases := []struct{ path, want string }{
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

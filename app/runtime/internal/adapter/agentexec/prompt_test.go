package agentexec

import (
	"context"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/agentmemory"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/knowledge"
)

// TestComposeSystemPrompt_BaseOnly verifies a nil memory store
// yields the base prompt verbatim (no markdown headers).
func TestComposeSystemPrompt_BaseOnly(t *testing.T) {
	got := composePrompt(context.Background(), nil, nil, "")
	if !strings.Contains(got, "You are Lyra") {
		t.Errorf("base prompt missing identity, got %q", got)
	}
	if strings.Contains(got, "## User preferences") || strings.Contains(got, "## Project knowledge") {
		t.Error("nil memory should not produce section headers")
	}
}

// TestComposeSystemPrompt_WithMemory verifies the cascade — user
// then project — appears under stable headers.
func TestComposeSystemPrompt_WithMemory(t *testing.T) {
	store := &stubKnowledgeStore{
		user:    "prefer terse output",
		project: "build with `make test`",
	}
	got := composePrompt(context.Background(), store, nil, "")
	if !strings.Contains(got, "## User preferences") {
		t.Error("user section missing")
	}
	if !strings.Contains(got, "## Project knowledge") {
		t.Error("project section missing")
	}
	// User precedes project.
	userIdx := strings.Index(got, "## User preferences")
	projIdx := strings.Index(got, "## Project knowledge")
	if userIdx > projIdx {
		t.Error("user section should appear before project section")
	}
}

// TestComposeSystemPrompt_SkipsEmptyScopes verifies absent scopes
// don't produce empty markdown headers.
func TestComposeSystemPrompt_SkipsEmptyScopes(t *testing.T) {
	store := &stubKnowledgeStore{project: "only project"}
	got := composePrompt(context.Background(), store, nil, "")
	if strings.Contains(got, "## User preferences") {
		t.Error("empty user scope should be skipped")
	}
	if !strings.Contains(got, "## Project knowledge") {
		t.Error("project scope should appear")
	}
}

// TestComposePrompt_ProjectMemoryFollowsCwd — the project scope must
// read the LYRA.md of the TURN's working directory (the per-session
// cwd), not a directory fixed at construction time.
func TestComposePrompt_ProjectMemoryFollowsCwd(t *testing.T) {
	store := &stubKnowledgeStore{project: "project body"}
	composePrompt(context.Background(), store, nil, "/projects/alpha")
	if store.projectDir != "/projects/alpha" {
		t.Fatalf("project memory read dir = %q, want /projects/alpha", store.projectDir)
	}
}

func TestComposePromptPlacesCuratedMemoryBelowHumanProjectKnowledge(t *testing.T) {
	store := &stubKnowledgeStore{user: "global", project: "human project rule"}
	memory := stubAgentMemory{content: "agent learned fact"}
	got := composePrompt(context.Background(), store, memory, "/projects/alpha")
	curatedIndex := strings.Index(got, "## Agent-curated project memory")
	projectIndex := strings.Index(got, "## Project knowledge")
	if curatedIndex < 0 || projectIndex < 0 || curatedIndex > projectIndex {
		t.Fatalf("prompt precedence is wrong:\n%s", got)
	}
}

// ------------------------------------------------------------------
// helpers
// ------------------------------------------------------------------

type stubKnowledgeStore struct {
	user    string
	project string

	// projectDir records the dir the last ScopeProject Get received —
	// the per-session-cwd regression assertions read it.
	projectDir string
}

type stubAgentMemory struct{ content string }

func (s stubAgentMemory) Items(_ context.Context, _ agentmemory.Scope, _ string) ([]agentmemory.Item, error) {
	if strings.TrimSpace(s.content) == "" {
		return nil, nil
	}
	return []agentmemory.Item{{Content: s.content}}, nil
}

func (s *stubKnowledgeStore) Get(_ context.Context, scope knowledge.Scope, dir string) (string, error) {
	if scope == knowledge.ScopeProject {
		s.projectDir = dir
	}
	return s.get(scope)
}

func (s *stubKnowledgeStore) get(scope knowledge.Scope) (string, error) {
	switch scope {
	case knowledge.ScopeUser:
		return s.user, nil
	case knowledge.ScopeProject:
		return s.project, nil
	}
	return "", nil
}

func (s *stubKnowledgeStore) Update(_ context.Context, scope knowledge.Scope, _ string, content string) error {
	switch scope {
	case knowledge.ScopeUser:
		s.user = content
	case knowledge.ScopeProject:
		s.project = content
	}
	return nil
}

func (s *stubKnowledgeStore) List(_ context.Context, _ string) ([]knowledge.Entry, error) {
	return nil, nil
}

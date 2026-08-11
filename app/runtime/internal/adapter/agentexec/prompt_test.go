package agentexec

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/agentmemory"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/knowledge"
)

// TestComposeSystemPrompt_BaseOnly verifies a nil memory store
// yields the base prompt verbatim (no markdown headers).
func TestComposeSystemPrompt_BaseOnly(t *testing.T) {
	got := composeSystemPromptText(t, WorkingContextConfig{}, "")
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
		home: "prefer terse output",
		cwd:  "build with `make test`",
	}
	got := composeSystemPromptText(t, WorkingContextConfig{Knowledge: store}, "")
	if !strings.Contains(got, "## User preferences") {
		t.Error("user section missing")
	}
	if !strings.Contains(got, "## Workspace knowledge") {
		t.Error("workspace section missing")
	}
	// User precedes project.
	userIdx := strings.Index(got, "## User preferences")
	projIdx := strings.Index(got, "## Workspace knowledge")
	if userIdx > projIdx {
		t.Error("user section should appear before project section")
	}
}

// TestComposeSystemPrompt_SkipsEmptyScopes verifies absent scopes
// don't produce empty markdown headers.
func TestComposeSystemPrompt_SkipsEmptyScopes(t *testing.T) {
	store := &stubKnowledgeStore{cwd: "only workspace"}
	got := composeSystemPromptText(t, WorkingContextConfig{Knowledge: store}, "")
	if strings.Contains(got, "## User preferences") {
		t.Error("empty user scope should be skipped")
	}
	if !strings.Contains(got, "## Workspace knowledge") {
		t.Error("workspace scope should appear")
	}
}

// TestComposePrompt_ProjectMemoryFollowsCWD — the project scope must
// read the LYRA.md of the TURN's working directory (the per-session
// cwd), not a directory fixed at construction time.
func TestComposePrompt_ProjectMemoryFollowsCWD(t *testing.T) {
	store := &stubKnowledgeStore{cwd: "workspace body"}
	composeSystemPromptText(t, WorkingContextConfig{Knowledge: store}, "/projects/alpha")
	if store.workspaceDir != "/projects/alpha" {
		t.Fatalf("knowledge read dir = %q, want /projects/alpha", store.workspaceDir)
	}
}

func TestComposePromptPlacesCuratedMemoryBelowHumanProjectKnowledge(t *testing.T) {
	store := &stubKnowledgeStore{home: "global", cwd: "human workspace rule"}
	memory := stubAgentMemory{content: "agent learned fact"}
	got := composeSystemPromptText(t, WorkingContextConfig{
		Knowledge: store, AgentMemory: memory,
	}, "/projects/alpha")
	curatedIndex := strings.Index(got, "## Pinned memory")
	projectIndex := strings.Index(got, "## Workspace knowledge")
	if curatedIndex < 0 || projectIndex < 0 || curatedIndex > projectIndex {
		t.Fatalf("prompt precedence is wrong:\n%s", got)
	}
}

func TestComposePromptPreservesTheThreeKnowledgeScopes(t *testing.T) {
	store := &stubKnowledgeStore{
		home:        "global preference",
		projectRoot: "repository convention",
		cwd:         "workspace override",
	}
	got := composeSystemPromptText(t, WorkingContextConfig{Knowledge: store}, "/repo/packages/app")
	home := strings.Index(got, "global preference")
	project := strings.Index(got, "repository convention")
	workspace := strings.Index(got, "workspace override")
	if home < 0 || project <= home || workspace <= project {
		t.Fatalf("knowledge cascade precedence is wrong:\n%s", got)
	}
}

func TestComposePromptUsesInjectedUserHomeForAgentDocs(t *testing.T) {
	userHome := t.TempDir()
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(userHome, ".lyra"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(userHome, ".lyra", "AGENTS.md"), []byte("injected home rule"), 0o644); err != nil {
		t.Fatal(err)
	}

	got := composeSystemPromptText(t, WorkingContextConfig{UserHome: userHome}, workspace)
	if !strings.Contains(got, "injected home rule") {
		t.Fatalf("prompt did not use injected user home:\n%s", got)
	}
}

// ------------------------------------------------------------------
// helpers
// ------------------------------------------------------------------

func composeSystemPromptText(t *testing.T, config WorkingContextConfig, cwd string) string {
	t.Helper()
	message, err := NewWorkingContextComposer(config).composeSystemMessage(t.Context(), "", cwd)
	if err != nil {
		t.Fatal(err)
	}
	return message.Text()
}

type stubKnowledgeStore struct {
	home         string
	projectRoot  string
	cwd          string
	workspaceDir string
}

type stubAgentMemory struct{ content string }

func (s stubAgentMemory) Items(_ context.Context, scope agentmemory.Scope, _ string) ([]agentmemory.Item, error) {
	if scope != agentmemory.ScopeProject || strings.TrimSpace(s.content) == "" {
		return nil, nil
	}
	// Pinned so it reaches the always-on core (the composer injects pinned only).
	return []agentmemory.Item{{Content: s.content, Pinned: true, Status: agentmemory.StatusActive}}, nil
}

func (s *stubKnowledgeStore) Entries(_ context.Context, cwd string) ([]knowledge.Entry, error) {
	s.workspaceDir = cwd
	entries := make([]knowledge.Entry, 0, 3)
	if s.home != "" {
		entries = append(entries, knowledge.Entry{Scope: knowledge.ScopeHome, Path: "/home/.lyra/LYRA.md", Content: s.home})
	}
	if s.projectRoot != "" {
		entries = append(entries, knowledge.Entry{Scope: knowledge.ScopeProjectRoot, Path: "/repo/LYRA.md", Content: s.projectRoot})
	}
	if s.cwd != "" {
		entries = append(entries, knowledge.Entry{Scope: knowledge.ScopeCWD, Path: filepath.Join(cwd, "LYRA.md"), Content: s.cwd})
	}
	return entries, nil
}

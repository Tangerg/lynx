package engine

import (
	"context"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/core/model/chat/memory"

	lyramem "github.com/Tangerg/lynx/lyra/internal/service/memory"
)

// TestComposeSystemPrompt_BaseOnly verifies a nil memory service
// yields the base prompt verbatim (no markdown headers).
func TestComposeSystemPrompt_BaseOnly(t *testing.T) {
	got := composeSystemPrompt(context.Background(), nil)
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
	svc := &stubMemoryService{
		user:    "prefer terse output",
		project: "build with `make test`",
	}
	got := composeSystemPrompt(context.Background(), svc)
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
	svc := &stubMemoryService{project: "only project"}
	got := composeSystemPrompt(context.Background(), svc)
	if strings.Contains(got, "## User preferences") {
		t.Error("empty user scope should be skipped")
	}
	if !strings.Contains(got, "## Project knowledge") {
		t.Error("project scope should appear")
	}
}

// TestCompactor_NoOpBelowThreshold doesn't talk to a real LLM —
// confirms the early-return path when there aren't enough
// messages to bother compacting.
func TestCompactor_NoOpBelowThreshold(t *testing.T) {
	store := memory.NewInMemoryStore()
	const sessID = "s"
	_ = store.Write(context.Background(), sessID,
		chat.NewUserMessage("a"),
		chat.NewAssistantMessage("b"),
	)
	c := newCompactor(store, nil /* never called */, CompactionConfig{MaxMessages: 10})
	compacted, err := c.maybeCompact(context.Background(), sessID)
	if err != nil {
		t.Fatal(err)
	}
	if compacted {
		t.Error("below threshold should not compact")
	}
}

// TestCompactor_Compacts drives the full path with a stub model
// that returns a fixed summary. After compaction:
//   - store size == 1 (summary) + keepRecent
//   - the surviving first message is a SystemMessage carrying
//     the [Earlier conversation summary] preamble
func TestCompactor_Compacts(t *testing.T) {
	store := memory.NewInMemoryStore()
	const sessID = "sess-compact"
	const total = 20
	for i := 0; i < total; i++ {
		_ = store.Write(context.Background(), sessID, chat.NewUserMessage("msg"))
	}

	stub := newStreamingStubModel("BULLETS")
	client, _ := chat.NewClient(stub)

	c := newCompactor(store, client, CompactionConfig{MaxMessages: total, KeepRecent: 4})
	compacted, err := c.maybeCompact(context.Background(), sessID)
	if err != nil {
		t.Fatal(err)
	}
	if !compacted {
		t.Fatal("expected compaction to fire")
	}

	after, _ := store.Read(context.Background(), sessID)
	if len(after) != 5 {
		t.Fatalf("post-compact len = %d, want 5", len(after))
	}
	sys, ok := after[0].(*chat.SystemMessage)
	if !ok {
		t.Fatalf("first message is %T, want *chat.SystemMessage", after[0])
	}
	if !strings.HasPrefix(sys.Text, "[Earlier conversation summary]") {
		t.Errorf("summary preamble missing, got %q", sys.Text)
	}
}

// ------------------------------------------------------------------
// helpers
// ------------------------------------------------------------------

type stubMemoryService struct {
	user    string
	project string
}

func (s *stubMemoryService) Get(_ context.Context, scope lyramem.Scope) (string, error) {
	switch scope {
	case lyramem.ScopeUser:
		return s.user, nil
	case lyramem.ScopeProject:
		return s.project, nil
	}
	return "", nil
}

func (s *stubMemoryService) Update(_ context.Context, scope lyramem.Scope, content string) error {
	switch scope {
	case lyramem.ScopeUser:
		s.user = content
	case lyramem.ScopeProject:
		s.project = content
	}
	return nil
}

func (s *stubMemoryService) List(_ context.Context) ([]lyramem.Entry, error) {
	return nil, nil
}

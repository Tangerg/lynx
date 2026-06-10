package engine

import (
	"context"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/core/model/chat/middleware/memory"

	lyramem "github.com/Tangerg/lynx/lyra/internal/service/memory"
)

// TestComposeSystemPrompt_BaseOnly verifies a nil memory service
// yields the base prompt verbatim (no markdown headers).
func TestComposeSystemPrompt_BaseOnly(t *testing.T) {
	got := composePrompt(context.Background(), nil, "")
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
	got := composePrompt(context.Background(), svc, "")
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
	got := composePrompt(context.Background(), svc, "")
	if strings.Contains(got, "## User preferences") {
		t.Error("empty user scope should be skipped")
	}
	if !strings.Contains(got, "## Project knowledge") {
		t.Error("project scope should appear")
	}
}

// TestCompactor_NopBelowThreshold doesn't talk to a real LLM —
// confirms the early-return path when there aren't enough
// messages to bother compacting.
func TestCompactor_NopBelowThreshold(t *testing.T) {
	store := memory.NewInMemoryStore()
	const sessID = "s"
	_ = store.Write(context.Background(), sessID,
		chat.NewUserMessage("a"),
		chat.NewAssistantMessage("b"),
	)
	c := newCompactor(store, nil /* never called */, CompactionConfig{MaxMessages: 10})
	res, err := c.maybeCompact(context.Background(), sessID)
	if err != nil {
		t.Fatal(err)
	}
	if res.Compacted {
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
	res, err := c.maybeCompact(context.Background(), sessID)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Compacted {
		t.Fatal("expected compaction to fire")
	}
	if res.MessagesBefore != total || res.MessagesAfter != 5 {
		t.Errorf("result counts = (%d → %d), want (%d → 5)", res.MessagesBefore, res.MessagesAfter, total)
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

// TestCompactor_CutBoundary is the regression for the DeepSeek 400 bug:
// when the naive cutoff (len-keepRecent) lands mid-turn — splitting an
// AssistantMessage-with-tool_calls from its ToolMessage — the compactor
// must advance the cut to the next UserMessage boundary so the kept
// `recent` slice never starts with an orphaned ToolMessage.
func TestCompactor_CutBoundary(t *testing.T) {
	store := memory.NewInMemoryStore()
	const sessID = "sess-boundary"

	// Build a history that triggers the boundary bug:
	//   [0] user      <- first turn
	//   [1] assistant (with tool call — represented here as AssistantMessage)
	//   [2] tool      (result)
	//   [3] assistant (reply)
	//   [4] user      <- second turn
	//   [5] assistant
	// With MaxMessages=6, keepRecent=4 → naive cutoff = 6-4 = 2.
	// msgs[2] is a ToolMessage → recent would start orphaned.
	// The fix must advance cutoff to 4 (the UserMessage at [4]).

	asst := func(text string) chat.Message { return chat.NewAssistantMessage(text) }
	user := func(text string) chat.Message { return chat.NewUserMessage(text) }
	tool := func(id, result string) chat.Message {
		m, _ := chat.NewToolMessage([]*chat.ToolReturn{{ID: id, Name: "bash", Result: result}})
		return m
	}

	msgs := []chat.Message{
		user("first question"),
		asst(""),         // assistant turn with (notional) tool_calls
		tool("c1", "ok"), // tool result — must not be orphaned at recent[0]
		asst("done"),
		user("second question"),
		asst("answer"),
	}
	for _, m := range msgs {
		_ = store.Write(context.Background(), sessID, m)
	}

	stub := newStreamingStubModel("BULLETS")
	client, _ := chat.NewClient(stub)
	c := newCompactor(store, client, CompactionConfig{MaxMessages: 6, KeepRecent: 4})
	res, err := c.maybeCompact(context.Background(), sessID)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Compacted {
		t.Fatal("expected compaction to fire")
	}

	after, _ := store.Read(context.Background(), sessID)
	// First message must be the system summary.
	if _, ok := after[0].(*chat.SystemMessage); !ok {
		t.Fatalf("after[0] = %T, want *chat.SystemMessage", after[0])
	}
	// Second message must be the UserMessage from the second turn, never a ToolMessage.
	if _, ok := after[1].(*chat.UserMessage); !ok {
		t.Fatalf("after[1] = %T, want *chat.UserMessage (not orphaned ToolMessage)", after[1])
	}
}

// ------------------------------------------------------------------
// helpers
// ------------------------------------------------------------------

type stubMemoryService struct {
	user    string
	project string

	// projectDir records the dir the last ScopeProject Get received —
	// the per-session-cwd regression assertions read it.
	projectDir string
}

func (s *stubMemoryService) Get(_ context.Context, scope lyramem.Scope, dir string) (string, error) {
	if scope == lyramem.ScopeProject {
		s.projectDir = dir
	}
	return s.get(scope)
}

func (s *stubMemoryService) get(scope lyramem.Scope) (string, error) {
	switch scope {
	case lyramem.ScopeUser:
		return s.user, nil
	case lyramem.ScopeProject:
		return s.project, nil
	}
	return "", nil
}

func (s *stubMemoryService) Update(_ context.Context, scope lyramem.Scope, _ string, content string) error {
	switch scope {
	case lyramem.ScopeUser:
		s.user = content
	case lyramem.ScopeProject:
		s.project = content
	}
	return nil
}

func (s *stubMemoryService) List(_ context.Context, _ string) ([]lyramem.Entry, error) {
	return nil, nil
}

// TestComposePrompt_ProjectMemoryFollowsCwd — the project scope must
// read the LYRA.md of the TURN's working directory (the per-session
// cwd), not a directory fixed at construction time.
func TestComposePrompt_ProjectMemoryFollowsCwd(t *testing.T) {
	svc := &stubMemoryService{project: "project body"}
	composePrompt(context.Background(), svc, "/projects/alpha")
	if svc.projectDir != "/projects/alpha" {
		t.Fatalf("project memory read dir = %q, want /projects/alpha", svc.projectDir)
	}
}

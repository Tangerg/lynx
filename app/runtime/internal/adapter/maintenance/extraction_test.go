package maintenance

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Tangerg/lynx/chatclient"
	history "github.com/Tangerg/lynx/chathistory"
	"github.com/Tangerg/lynx/core/chat"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/agentmemory"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/storage/sqlite"
)

type scriptedReply struct {
	text string
	err  error
}

type scriptedModel struct {
	mu       sync.Mutex
	replies  []scriptedReply
	requests []*chat.Request
}

func (m *scriptedModel) Call(_ context.Context, request *chat.Request) (*chat.Response, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.requests = append(m.requests, request)
	if len(m.replies) == 0 {
		return nil, errors.New("scripted model exhausted")
	}
	reply := m.replies[0]
	m.replies = m.replies[1:]
	if reply.err != nil {
		return nil, reply.err
	}
	message := chat.NewAssistantMessage(chat.NewTextPart(reply.text))
	return chat.NewResponse(chat.Choice{Index: 0, Message: &message, FinishReason: chat.FinishReasonStop})
}

func extractionFixture(t *testing.T, replies ...scriptedReply) (*Extractor, *sqlite.AgentMemoryStore, *scriptedModel) {
	t.Helper()
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "lyra.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	memory := sqlite.NewAgentMemoryStore(db)
	messages := history.NewInMemoryStore()
	if err := messages.Write(t.Context(), "ses_1",
		chat.NewUserMessage(chat.NewTextPart("first")),
		chat.NewAssistantMessage(chat.NewTextPart("reply")),
		chat.NewUserMessage(chat.NewTextPart("second")),
		chat.NewAssistantMessage(chat.NewTextPart("reply")),
	); err != nil {
		t.Fatal(err)
	}
	model := &scriptedModel{replies: replies}
	client, err := chatclient.New(model)
	if err != nil {
		t.Fatal(err)
	}
	extractor := NewExtractor(messages, memory, func(context.Context) *chatclient.Client { return client }, nil, CurationConfig{MinPendingFacts: 1})
	extractor.now = func() time.Time { return time.Date(2026, 7, 19, 9, 30, 0, 0, time.FixedZone("CST", 8*60*60)) }
	return extractor, memory, model
}

func TestExtractorAppendsDailyLedgerAndCuratesItems(t *testing.T) {
	extractor, memory, model := extractionFixture(t,
		scriptedReply{text: "- use make test\n- prefer concise errors"},
		scriptedReply{text: "- Run `make test`.\n- Prefer concise errors."},
	)
	if err := extractor.MaybeExtract(t.Context(), "ses_1", "/repo"); err != nil {
		t.Fatal(err)
	}
	state, err := memory.State(t.Context(), "/repo")
	if err != nil || state.Watermark == 0 {
		t.Fatalf("curation watermark = %+v, err=%v", state, err)
	}
	// Curated facts land as pending proposals awaiting review (not yet injected).
	items, err := memory.List(t.Context(), agentmemory.ScopeProject, "/repo")
	if err != nil || len(items) != 2 {
		t.Fatalf("curated items = (%+v, %v)", items, err)
	}
	found := false
	for _, item := range items {
		if item.Status != agentmemory.StatusPending {
			t.Fatalf("proposal not pending: %+v", item)
		}
		if strings.Contains(item.Content, "make test") {
			found = true
		}
	}
	if !found {
		t.Fatalf("curated items missing the make-test fact: %+v", items)
	}
	ledger, err := memory.PendingLedger(t.Context(), "/repo", 0, 10)
	if err != nil || len(ledger) != 2 || ledger[0].Day != "2026-07-19" {
		t.Fatalf("ledger = (%+v, %v)", ledger, err)
	}
	if len(model.requests) != 2 {
		t.Fatalf("model calls = %d, want extraction + curation", len(model.requests))
	}
	curationPrompt := model.requests[1].Messages[1].Text()
	if !strings.Contains(curationPrompt, "[2026-07-19 #") {
		t.Fatalf("curation prompt lacks daily provenance: %q", curationPrompt)
	}
}

func TestExtractorLeavesWatermarkOnCurationFailureThenRecovers(t *testing.T) {
	providerFailure := errors.New("provider unavailable")
	extractor, memory, _ := extractionFixture(t,
		scriptedReply{text: "- durable fact"},
		scriptedReply{err: providerFailure},
		scriptedReply{text: "- durable fact"},
	)
	if err := extractor.MaybeExtract(t.Context(), "ses_1", "/repo"); !errors.Is(err, providerFailure) {
		t.Fatalf("first extraction error = %v", err)
	}
	state, err := memory.State(t.Context(), "/repo")
	if err != nil || state.Watermark != 0 {
		t.Fatalf("failed curation advanced watermark: (%+v, %v)", state, err)
	}
	if items, err := memory.Items(t.Context(), agentmemory.ScopeProject, "/repo"); err != nil || len(items) != 0 {
		t.Fatalf("failed curation published items: (%+v, %v)", items, err)
	}
	pending, err := memory.PendingLedger(t.Context(), "/repo", 0, 10)
	if err != nil || len(pending) != 1 {
		t.Fatalf("durable pending ledger = (%+v, %v)", pending, err)
	}

	// Extraction is no longer eligible, but curation must still recover the
	// durable backlog instead of waiting for another long conversation.
	extractor.minMsgs = 100
	if err := extractor.MaybeExtract(t.Context(), "ses_1", "/repo"); err != nil {
		t.Fatal(err)
	}
	state, _ = memory.State(t.Context(), "/repo")
	items, _ := memory.List(t.Context(), agentmemory.ScopeProject, "/repo")
	if state.Watermark != pending[0].Sequence || len(items) != 1 || items[0].Content != "- durable fact" {
		t.Fatalf("recovered curation: state=%+v items=%+v", state, items)
	}
}

func TestCurationGateAndTokenEstimate(t *testing.T) {
	extractor := &Extractor{config: CurationConfig{MinPendingFacts: 3, MaxAge: time.Hour}}
	now := time.Date(2026, 7, 19, 10, 0, 0, 0, time.UTC)
	state := agentmemory.State{Watermark: 1, UpdatedAt: now}
	if extractor.curationDue(state, 2, now) {
		t.Fatal("small fresh backlog should not curate")
	}
	if !extractor.curationDue(state, 3, now) {
		t.Fatal("fact threshold should curate")
	}
	if !extractor.curationDue(state, 1, now.Add(time.Hour)) {
		t.Fatal("age threshold should curate")
	}
	if !extractor.curationDue(agentmemory.State{}, 1, now) {
		t.Fatal("first generation should curate immediately")
	}

	if tokens := estimateTextTokens(strings.Repeat("界", 100)); tokens != 100 {
		t.Fatalf("CJK token estimate = %d, want 100", tokens)
	}
}

func TestExtractorDoesNotAdvanceWatermarkForOversizedCuration(t *testing.T) {
	extractor, memory, _ := extractionFixture(t,
		scriptedReply{text: "- durable fact"},
		scriptedReply{text: strings.Repeat("界", 20)},
	)
	extractor.config.MaxTokens = 10
	if err := extractor.MaybeExtract(t.Context(), "ses_1", "/repo"); err == nil || !strings.Contains(err.Error(), "limit is 10") {
		t.Fatalf("oversized curation error = %v", err)
	}
	state, err := memory.State(t.Context(), "/repo")
	if err != nil || state.Watermark != 0 {
		t.Fatalf("oversized curation advanced watermark: (%+v, %v)", state, err)
	}
	pending, err := memory.PendingLedger(t.Context(), "/repo", 0, 10)
	if err != nil || len(pending) != 1 {
		t.Fatalf("oversized curation lost pending facts: (%+v, %v)", pending, err)
	}
}

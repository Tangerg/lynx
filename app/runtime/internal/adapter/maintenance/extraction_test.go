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

	"github.com/Tangerg/lynx/app/runtime/internal/domain/knowledge"
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
	extractor := NewExtractor(messages, memory, func(context.Context) *chatclient.Client { return client }, CurationConfig{MinPendingFacts: 1})
	extractor.now = func() time.Time { return time.Date(2026, 7, 19, 9, 30, 0, 0, time.FixedZone("CST", 8*60*60)) }
	return extractor, memory, model
}

func TestExtractorAppendsDailyLedgerAndPublishesCuratedMemory(t *testing.T) {
	extractor, memory, model := extractionFixture(t,
		scriptedReply{text: "- use make test\n- prefer concise errors"},
		scriptedReply{text: "# Project memory\n\n- Run `make test`.\n- Prefer concise errors."},
	)
	result, err := extractor.MaybeExtract(t.Context(), "ses_1", "/repo")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Extracted || !result.Curated || !strings.Contains(result.Facts, "make test") {
		t.Fatalf("extraction result = %+v", result)
	}
	curated, err := memory.CuratedMemory(t.Context(), "/repo")
	if err != nil {
		t.Fatal(err)
	}
	if curated.Watermark == 0 || !strings.Contains(curated.Content, "# Project memory") {
		t.Fatalf("curated memory = %+v", curated)
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
	if _, err := extractor.MaybeExtract(t.Context(), "ses_1", "/repo"); !errors.Is(err, providerFailure) {
		t.Fatalf("first extraction error = %v", err)
	}
	curated, err := memory.CuratedMemory(t.Context(), "/repo")
	if err != nil || curated.Watermark != 0 || curated.Content != "" {
		t.Fatalf("failed curation published half-state: (%+v, %v)", curated, err)
	}
	pending, err := memory.PendingLedger(t.Context(), "/repo", 0, 10)
	if err != nil || len(pending) != 1 {
		t.Fatalf("durable pending ledger = (%+v, %v)", pending, err)
	}

	// Extraction is no longer eligible, but curation must still recover the
	// durable backlog instead of waiting for another long conversation.
	extractor.minMsgs = 100
	result, err := extractor.MaybeExtract(t.Context(), "ses_1", "/repo")
	if err != nil {
		t.Fatal(err)
	}
	if result.Extracted || !result.Curated {
		t.Fatalf("recovery result = %+v", result)
	}
	curated, _ = memory.CuratedMemory(t.Context(), "/repo")
	if curated.Watermark != pending[0].Sequence || curated.Content != "- durable fact" {
		t.Fatalf("recovered curation = %+v", curated)
	}
}

func TestCurationGateAndTokenEstimate(t *testing.T) {
	extractor := &Extractor{config: CurationConfig{MinPendingFacts: 3, MaxAge: time.Hour}}
	now := time.Date(2026, 7, 19, 10, 0, 0, 0, time.UTC)
	current := knowledge.Curated{Watermark: 1, UpdatedAt: now}
	if extractor.curationDue(current, 2, now) {
		t.Fatal("small fresh backlog should not curate")
	}
	if !extractor.curationDue(current, 3, now) {
		t.Fatal("fact threshold should curate")
	}
	if !extractor.curationDue(current, 1, now.Add(time.Hour)) {
		t.Fatal("age threshold should curate")
	}
	if !extractor.curationDue(knowledge.Curated{}, 1, now) {
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
	if _, err := extractor.MaybeExtract(t.Context(), "ses_1", "/repo"); err == nil || !strings.Contains(err.Error(), "limit is 10") {
		t.Fatalf("oversized curation error = %v", err)
	}
	curated, err := memory.CuratedMemory(t.Context(), "/repo")
	if err != nil || curated.Watermark != 0 {
		t.Fatalf("oversized curation advanced watermark: (%+v, %v)", curated, err)
	}
	pending, err := memory.PendingLedger(t.Context(), "/repo", 0, 10)
	if err != nil || len(pending) != 1 {
		t.Fatalf("oversized curation lost pending facts: (%+v, %v)", pending, err)
	}
}

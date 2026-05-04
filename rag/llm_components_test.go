package rag_test

import (
	"context"
	"errors"
	"iter"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/core/document"
	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/core/rag"
)

// fakeChatModel is the mock used by every LLM-backed component test.
type fakeChatModel struct {
	defaults *chat.Options
	reply    string
	err      error

	// captured holds the last rendered prompt so tests can assert that
	// per-call variables (Number, Target, Query, ...) reached the LLM.
	captured string
}

func newFakeChatModel(t *testing.T, reply string) *fakeChatModel {
	t.Helper()
	defaults, err := chat.NewOptions("rag-fake")
	if err != nil {
		t.Fatal(err)
	}
	return &fakeChatModel{defaults: defaults, reply: reply}
}

func (m *fakeChatModel) DefaultOptions() *chat.Options { return m.defaults }
func (m *fakeChatModel) Info() chat.ModelInfo          { return chat.ModelInfo{Provider: "fake"} }

func (m *fakeChatModel) Call(_ context.Context, req *chat.Request) (*chat.Response, error) {
	if user, ok := req.Messages[len(req.Messages)-1].(*chat.UserMessage); ok {
		m.captured = user.Text
	}
	if m.err != nil {
		return nil, m.err
	}
	resp, _ := chat.NewResponse(
		[]*chat.Result{{
			AssistantMessage: chat.NewAssistantMessage(m.reply),
			Metadata:         &chat.ResultMetadata{FinishReason: chat.FinishReasonStop},
		}},
		&chat.ResponseMetadata{},
	)
	return resp, nil
}

func (m *fakeChatModel) Stream(_ context.Context, _ *chat.Request) iter.Seq2[*chat.Response, error] {
	return func(yield func(*chat.Response, error) bool) {}
}

// --- ContextualQueryAugmenter -------------------------------------------

func TestContextualAugmenter_RendersDocsAsContext(t *testing.T) {
	aug, err := rag.NewContextualQueryAugmenter(rag.ContextualQueryAugmenterConfig{})
	if err != nil {
		t.Fatal(err)
	}

	q, _ := rag.NewQuery("what is GOAP?")
	doc, _ := document.NewDocument("GOAP is goal-oriented action planning.", nil)

	got, err := aug.Augment(context.Background(), q, []*document.Document{doc})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got.Text, "GOAP is goal-oriented") {
		t.Fatalf("docs not embedded in augmented prompt: %q", got.Text)
	}
	if !strings.Contains(got.Text, "what is GOAP?") {
		t.Fatalf("query missing from augmented prompt: %q", got.Text)
	}
}

func TestContextualAugmenter_EmptyDocs_DefaultRefusal(t *testing.T) {
	aug, _ := rag.NewContextualQueryAugmenter(rag.ContextualQueryAugmenterConfig{})

	q, _ := rag.NewQuery("hi")
	got, err := aug.Augment(context.Background(), q, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got.Text, "knowledge base") {
		t.Fatalf("default empty-context message missing: %q", got.Text)
	}
}

func TestContextualAugmenter_EmptyDocs_AllowEmptyPassesThrough(t *testing.T) {
	aug, _ := rag.NewContextualQueryAugmenter(rag.ContextualQueryAugmenterConfig{
		AllowEmptyContext: true,
	})

	q, _ := rag.NewQuery("hi")
	got, err := aug.Augment(context.Background(), q, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got != q {
		t.Fatal("AllowEmptyContext=true should pass the original query through unchanged")
	}
}

func TestContextualAugmenter_NilQuery(t *testing.T) {
	aug, _ := rag.NewContextualQueryAugmenter(rag.ContextualQueryAugmenterConfig{})
	if _, err := aug.Augment(context.Background(), nil, nil); err == nil {
		t.Fatal("nil query must error")
	}
}

// --- MultiQueryExpander -------------------------------------------------

func TestMultiQueryExpander_ParsesNewlineVariants(t *testing.T) {
	model := newFakeChatModel(t, "variant 1\nvariant 2\nvariant 3")
	exp, err := rag.NewMultiQueryExpander(rag.MultiQueryExpanderConfig{
		ChatModel:       model,
		NumberOfQueries: 3,
	})
	if err != nil {
		t.Fatal(err)
	}

	q, _ := rag.NewQuery("hi")
	got, err := exp.Expand(context.Background(), q)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d variants, want 3", len(got))
	}
	if got[0].Text != "variant 1" {
		t.Fatalf("first variant = %q", got[0].Text)
	}
}

func TestMultiQueryExpander_IncludeOriginal(t *testing.T) {
	model := newFakeChatModel(t, "v1\nv2")
	exp, _ := rag.NewMultiQueryExpander(rag.MultiQueryExpanderConfig{
		ChatModel:       model,
		NumberOfQueries: 2,
		IncludeOriginal: true,
	})

	q, _ := rag.NewQuery("orig")
	got, _ := exp.Expand(context.Background(), q)
	if len(got) != 3 || got[0].Text != "orig" {
		t.Fatalf("IncludeOriginal=true should prepend original; got %d entries, first=%q",
			len(got), got[0].Text)
	}
}

func TestMultiQueryExpander_EmptyLLMFallsBackToOriginal(t *testing.T) {
	model := newFakeChatModel(t, "")
	exp, _ := rag.NewMultiQueryExpander(rag.MultiQueryExpanderConfig{
		ChatModel: model,
	})

	q, _ := rag.NewQuery("orig")
	got, _ := exp.Expand(context.Background(), q)
	if len(got) != 1 || got[0] != q {
		t.Fatal("empty LLM output must fall back to the original query")
	}
}

func TestMultiQueryExpanderConfig_RejectsMissingChatModel(t *testing.T) {
	if _, err := rag.NewMultiQueryExpander(rag.MultiQueryExpanderConfig{}); err == nil {
		t.Fatal("missing ChatModel must error")
	}
}

// --- CompressionQueryTransformer ----------------------------------------

func TestCompressionTransformer_UsesChatHistory(t *testing.T) {
	model := newFakeChatModel(t, "compressed query")
	tr, err := rag.NewCompressionQueryTransformer(rag.CompressionQueryTransformerConfig{ChatModel: model})
	if err != nil {
		t.Fatal(err)
	}

	q, _ := rag.NewQuery("follow-up")
	q.Set(rag.ChatHistoryKey, []chat.Message{
		chat.NewUserMessage("first turn"),
		chat.NewAssistantMessage("first reply"),
	})

	out, err := tr.Transform(context.Background(), q)
	if err != nil {
		t.Fatal(err)
	}
	if out.Text != "compressed query" {
		t.Fatalf("Text = %q, want compressed query", out.Text)
	}
	if !strings.Contains(model.captured, "first turn") {
		t.Fatal("history was not threaded into the prompt")
	}
}

func TestCompressionTransformer_EmptyOutputPreservesOriginal(t *testing.T) {
	model := newFakeChatModel(t, "")
	tr, _ := rag.NewCompressionQueryTransformer(rag.CompressionQueryTransformerConfig{ChatModel: model})

	q, _ := rag.NewQuery("orig")
	got, _ := tr.Transform(context.Background(), q)
	if got.Text != "orig" {
		t.Fatalf("Text = %q, want orig (empty LLM reply must preserve)", got.Text)
	}
}

// --- RewriteQueryTransformer --------------------------------------------

func TestRewriteTransformer_DefaultsToVectorStoreTarget(t *testing.T) {
	model := newFakeChatModel(t, "tightened query")
	tr, _ := rag.NewRewriteQueryTransformer(rag.RewriteQueryTransformerConfig{ChatModel: model})

	q, _ := rag.NewQuery("user input")
	if _, err := tr.Transform(context.Background(), q); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(model.captured, "vector store") {
		t.Fatalf("Target=vector store not threaded into prompt: %q", model.captured)
	}
}

func TestRewriteTransformer_HonorsCustomTarget(t *testing.T) {
	model := newFakeChatModel(t, "tightened")
	tr, _ := rag.NewRewriteQueryTransformer(rag.RewriteQueryTransformerConfig{
		ChatModel:          model,
		TargetSearchSystem: "elasticsearch",
	})

	q, _ := rag.NewQuery("input")
	_, _ = tr.Transform(context.Background(), q)
	if !strings.Contains(model.captured, "elasticsearch") {
		t.Fatalf("custom target not threaded: %q", model.captured)
	}
}

// --- TranslationQueryTransformer ----------------------------------------

func TestTranslationTransformer_RequiresTargetLanguage(t *testing.T) {
	model := newFakeChatModel(t, "")
	if _, err := rag.NewTranslationQueryTransformer(rag.TranslationQueryTransformerConfig{
		ChatModel: model,
	}); err == nil {
		t.Fatal("missing TargetLanguage must error")
	}
}

func TestTranslationTransformer_TranslatesText(t *testing.T) {
	model := newFakeChatModel(t, "你好")
	tr, _ := rag.NewTranslationQueryTransformer(rag.TranslationQueryTransformerConfig{
		ChatModel:      model,
		TargetLanguage: "Chinese",
	})

	q, _ := rag.NewQuery("hello")
	got, err := tr.Transform(context.Background(), q)
	if err != nil {
		t.Fatal(err)
	}
	if got.Text != "你好" {
		t.Fatalf("Text = %q, want 你好", got.Text)
	}
}

func TestTranslationTransformer_PropagatesError(t *testing.T) {
	model := newFakeChatModel(t, "")
	model.err = errors.New("boom")

	tr, _ := rag.NewTranslationQueryTransformer(rag.TranslationQueryTransformerConfig{
		ChatModel:      model,
		TargetLanguage: "English",
	})

	q, _ := rag.NewQuery("hi")
	if _, err := tr.Transform(context.Background(), q); err == nil {
		t.Fatal("error must propagate")
	}
}

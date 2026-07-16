package rag_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/chatclient"
	"github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/core/document"
	"github.com/Tangerg/lynx/rag"
)

// fakeChatModel is the target core/chat mock used by every LLM-backed
// component test.
type fakeChatModel struct {
	reply string
	err   error

	// captured holds the last rendered prompt so tests can assert that
	// per-call variables (Number, Target, Query, ...) reached the LLM.
	captured string
}

func newFakeChatModel(_ *testing.T, reply string) *fakeChatModel {
	return &fakeChatModel{reply: reply}
}

func (m *fakeChatModel) Call(_ context.Context, req *chat.Request) (*chat.Response, error) {
	if len(req.Messages) != 0 {
		m.captured = req.Messages[len(req.Messages)-1].Text()
	}
	if m.err != nil {
		return nil, m.err
	}
	choice := chat.Choice{Index: 0, FinishReason: chat.FinishReasonStop}
	if m.reply != "" {
		message := chat.NewAssistantMessage(chat.NewTextPart(m.reply))
		choice.Message = &message
	}
	return chat.NewResponse(choice)
}

// --- ContextualAugmenter -------------------------------------------

func TestContextualAugmenter_RendersDocsAsContext(t *testing.T) {
	aug, err := rag.NewContextualAugmenter(rag.ContextualAugmenterConfig{})
	if err != nil {
		t.Fatal(err)
	}

	q, _ := rag.NewQuery("what is GOAP?")
	doc, _ := document.NewDocument("GOAP is goal-oriented action planning.", nil)

	got, err := aug.Augment(context.Background(), q, []rag.Candidate{candidate(doc)})
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

func TestContextualAugmenter_PreservesQueryExtra(t *testing.T) {
	aug, err := rag.NewContextualAugmenter(rag.ContextualAugmenterConfig{})
	if err != nil {
		t.Fatal(err)
	}

	q, _ := rag.NewQuery("what is GOAP?")
	q.Set("route", "docs")
	doc, _ := document.NewDocument("GOAP is goal-oriented action planning.", nil)

	got, err := aug.Augment(context.Background(), q, []rag.Candidate{candidate(doc)})
	if err != nil {
		t.Fatal(err)
	}
	if v, _ := got.Get("route"); v != "docs" {
		t.Fatalf("query metadata was not preserved: route=%v", v)
	}
}

func TestContextualAugmenter_EmptyDocs_DefaultRefusal(t *testing.T) {
	aug, _ := rag.NewContextualAugmenter(rag.ContextualAugmenterConfig{})

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
	aug, _ := rag.NewContextualAugmenter(rag.ContextualAugmenterConfig{AllowEmptyContext: true})

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
	aug, _ := rag.NewContextualAugmenter(rag.ContextualAugmenterConfig{})
	if _, err := aug.Augment(context.Background(), nil, nil); err == nil {
		t.Fatal("nil query must error")
	}
}

func TestLLMComponentsRejectTemplatesMissingRequiredFields(t *testing.T) {
	prompt, err := chatclient.ParseTemplate("{{.Other}}")
	if err != nil {
		t.Fatal(err)
	}
	model := newFakeChatModel(t, "")

	for name, build := range map[string]func() error{
		"contextual augmenter": func() error {
			_, err := rag.NewContextualAugmenter(rag.ContextualAugmenterConfig{
				PromptTemplate: prompt,
			})
			return err
		},
		"multi-query expander": func() error {
			_, err := rag.NewMultiQueryExpander(rag.MultiQueryExpanderConfig{
				ChatModel:      model,
				PromptTemplate: prompt,
			})
			return err
		},
		"compression transformer": func() error {
			_, err := rag.NewCompressionTransformer(rag.CompressionTransformerConfig{
				ChatModel:      model,
				PromptTemplate: prompt,
			})
			return err
		},
		"rewrite transformer": func() error {
			_, err := rag.NewRewriteTransformer(rag.RewriteTransformerConfig{
				ChatModel:      model,
				PromptTemplate: prompt,
			})
			return err
		},
		"translation transformer": func() error {
			_, err := rag.NewTranslationTransformer(rag.TranslationTransformerConfig{
				ChatModel:      model,
				TargetLanguage: "English",
				PromptTemplate: prompt,
			})
			return err
		},
	} {
		t.Run(name, func(t *testing.T) {
			if err := build(); !errors.Is(err, chatclient.ErrInvalidTemplate) {
				t.Fatalf("constructor error = %v, want ErrInvalidTemplate", err)
			}
		})
	}
}

// --- MultiQueryExpander --------------------------------------------

func TestMultiQueryExpander_ParsesNewlineVariants(t *testing.T) {
	model := newFakeChatModel(t, " variant 1 \n\nvariant 2\nvariant 3")
	exp, err := rag.NewMultiQueryExpander(rag.MultiQueryExpanderConfig{
		ChatModel:       model,
		NumberOfQueries: 3,
	})
	if err != nil {
		t.Fatal(err)
	}

	q, _ := rag.NewQuery("hi")
	q.Set("route", "docs")
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
	if v, _ := got[0].Get("route"); v != "docs" {
		t.Fatalf("variant metadata was not preserved: route=%v", v)
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
		t.Fatalf("IncludeOriginal=true should prepend original; got %d entries, first=%q", len(got), got[0].Text)
	}
}

func TestMultiQueryExpander_EmptyLLMFallsBackToOriginal(t *testing.T) {
	model := newFakeChatModel(t, "")
	exp, _ := rag.NewMultiQueryExpander(rag.MultiQueryExpanderConfig{ChatModel: model})

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

// --- CompressionTransformer ---------------------------------------

func TestCompressionTransformer_UsesChatHistory(t *testing.T) {
	model := newFakeChatModel(t, "compressed query")
	tr, err := rag.NewCompressionTransformer(rag.CompressionTransformerConfig{ChatModel: model})
	if err != nil {
		t.Fatal(err)
	}

	q, _ := rag.NewQuery("follow-up")
	q.Set(rag.ChatHistoryKey, []chat.Message{
		chat.NewUserMessage(chat.NewTextPart("first turn")),
		chat.NewAssistantMessage(chat.NewTextPart("first reply")),
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
	tr, _ := rag.NewCompressionTransformer(rag.CompressionTransformerConfig{ChatModel: model})

	q, _ := rag.NewQuery("orig")
	got, _ := tr.Transform(context.Background(), q)
	if got.Text != "orig" {
		t.Fatalf("Text = %q, want orig (empty LLM reply must preserve)", got.Text)
	}
}

// --- RewriteTransformer -------------------------------------------

func TestRewriteTransformer_DefaultsToVectorStoreTarget(t *testing.T) {
	model := newFakeChatModel(t, "tightened query")
	tr, _ := rag.NewRewriteTransformer(rag.RewriteTransformerConfig{ChatModel: model})

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
	tr, _ := rag.NewRewriteTransformer(rag.RewriteTransformerConfig{
		ChatModel:          model,
		TargetSearchSystem: "elasticsearch",
	})

	q, _ := rag.NewQuery("input")
	_, _ = tr.Transform(context.Background(), q)
	if !strings.Contains(model.captured, "elasticsearch") {
		t.Fatalf("custom target not threaded: %q", model.captured)
	}
}

// --- TranslationTransformer ---------------------------------------

func TestTranslationTransformer_RequiresTargetLanguage(t *testing.T) {
	model := newFakeChatModel(t, "")
	if _, err := rag.NewTranslationTransformer(rag.TranslationTransformerConfig{ChatModel: model}); err == nil {
		t.Fatal("missing TargetLanguage must error")
	}
}

func TestTranslationTransformer_TranslatesText(t *testing.T) {
	model := newFakeChatModel(t, "你好")
	tr, _ := rag.NewTranslationTransformer(rag.TranslationTransformerConfig{
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

	tr, _ := rag.NewTranslationTransformer(rag.TranslationTransformerConfig{
		ChatModel:      model,
		TargetLanguage: "English",
	})

	q, _ := rag.NewQuery("hi")
	if _, err := tr.Transform(context.Background(), q); err == nil {
		t.Fatal("error must propagate")
	}
}

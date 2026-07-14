package rag_test

import (
	"context"
	"errors"
	"iter"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/core/document"
	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/rag"
)

// stubRetriever returns a fixed document set; used to exercise the
// middleware without a real vector store.
type stubRetriever struct {
	docs []rag.Candidate
}

func (s *stubRetriever) Retrieve(_ context.Context, _ *rag.Query) ([]rag.Candidate, error) {
	return s.docs, nil
}

// echoChatModel mirrors the user's last message back. Lets the test
// observe what the middleware did to the request.
type echoChatModel struct {
	defaults *chat.Options
	captured string
}

func newEchoChatModel(t *testing.T) *echoChatModel {
	t.Helper()
	defaults, _ := chat.NewOptions("echo")
	return &echoChatModel{defaults: defaults}
}

func (m *echoChatModel) DefaultOptions() chat.Options { return *m.defaults }
func (m *echoChatModel) Metadata() chat.ModelMetadata { return chat.ModelMetadata{Provider: "fake"} }

func (m *echoChatModel) Call(_ context.Context, req *chat.Request) (*chat.Response, error) {
	m.captured = req.UserMessage().Text
	resp, _ := chat.NewResponse(
		&chat.Result{
			AssistantMessage: chat.NewAssistantMessage(m.captured),
			Metadata:         &chat.ResultMetadata{FinishReason: chat.FinishReasonStop},
		},
		&chat.ResponseMetadata{},
	)
	return resp, nil
}

func (m *echoChatModel) Stream(_ context.Context, _ *chat.Request) iter.Seq2[*chat.Response, error] {
	return func(yield func(*chat.Response, error) bool) {}
}

func TestNewMiddlewareRejectsInvalidConfig(t *testing.T) {
	if _, _, err := rag.NewMiddleware(rag.MiddlewareConfig{}); err == nil {
		t.Fatal("missing retrievers must error")
	}
}

func TestMiddlewareAugmentsRequestAndAttachesDocs(t *testing.T) {
	doc, _ := document.NewDocument("retrieved info", nil)
	retriever := &stubRetriever{docs: []rag.Candidate{candidate(doc)}}

	aug, _ := rag.NewContextualAugmenter(rag.ContextualAugmenterConfig{})

	callMW, _, err := rag.NewMiddleware(rag.MiddlewareConfig{
		Retriever: retriever,
		Augmenter: aug,
	})
	if err != nil {
		t.Fatal(err)
	}

	model := newEchoChatModel(t)
	client, _ := chat.NewClient(model)

	resp, err := client.Chat().
		WithCallMiddlewares(callMW).
		WithMessages(chat.NewUserMessage("what is RAG?")).
		Call().
		Response(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	// Augmented user message should embed the retrieved doc text.
	if !strings.Contains(model.captured, "retrieved info") {
		t.Fatalf("augmented user message did not embed retrieved doc: %q", model.captured)
	}

	// Response metadata should carry the retrieved docs.
	v, ok := resp.Metadata.Get(rag.DocumentContextKey)
	if !ok {
		t.Fatal("DocumentContextKey not attached to response metadata")
	}
	if docs, _ := v.([]rag.Candidate); len(docs) != 1 {
		t.Fatalf("attached docs len = %d, want 1", len(docs))
	}
}

func TestMiddlewarePropagatesRetrieverError(t *testing.T) {
	want := errors.New("boom")
	failingRetriever := &errorRetriever{err: want}

	callMW, _, err := rag.NewMiddleware(rag.MiddlewareConfig{
		Retriever: failingRetriever,
	})
	if err != nil {
		t.Fatal(err)
	}

	model := newEchoChatModel(t)
	client, _ := chat.NewClient(model)

	_, err = client.Chat().
		WithCallMiddlewares(callMW).
		WithMessages(chat.NewUserMessage("hi")).
		Call().
		Response(context.Background())
	if !errors.Is(err, want) {
		t.Fatalf("err = %v", err)
	}
}

type errorRetriever struct {
	err error
}

func (r *errorRetriever) Retrieve(_ context.Context, _ *rag.Query) ([]rag.Candidate, error) {
	return nil, r.err
}

// TestMiddlewareDoesNotMutateCallerMessages verifies the
// middleware augments a COPY: the caller's original *chat.UserMessage
// text must survive the call unchanged (buildRequest shares message
// pointers with the ClientRequest, so an in-place edit would corrupt
// reuse / re-consumed streams).
func TestMiddlewareDoesNotMutateCallerMessages(t *testing.T) {
	doc, _ := document.NewDocument("retrieved info", nil)
	retriever := &stubRetriever{docs: []rag.Candidate{candidate(doc)}}
	aug, _ := rag.NewContextualAugmenter(rag.ContextualAugmenterConfig{})

	callMW, _, err := rag.NewMiddleware(rag.MiddlewareConfig{
		Retriever: retriever,
		Augmenter: aug,
	})
	if err != nil {
		t.Fatal(err)
	}

	model := newEchoChatModel(t)
	client, _ := chat.NewClient(model)

	userMsg := chat.NewUserMessage("what is RAG?")
	if _, err := client.Chat().
		WithCallMiddlewares(callMW).
		WithMessages(userMsg).
		Call().
		Response(context.Background()); err != nil {
		t.Fatal(err)
	}

	// The model saw the augmented text...
	if !strings.Contains(model.captured, "retrieved info") {
		t.Fatalf("model did not see augmented text: %q", model.captured)
	}
	// ...but the caller's own message is untouched.
	if userMsg.Text != "what is RAG?" {
		t.Fatalf("caller message was mutated: %q", userMsg.Text)
	}
}

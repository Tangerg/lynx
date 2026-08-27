package rag_test

import (
	"context"
	"errors"
	"iter"
	"strings"
	"testing"

	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/document"
	"github.com/Tangerg/scope/rag"
)

// stubRetriever returns a fixed document set; used to exercise the
// middleware without a real vector store.
type stubRetriever struct {
	docs rag.Candidates
}

func (s *stubRetriever) Retrieve(_ context.Context, _ rag.Query) (rag.Candidates, error) {
	return s.docs, nil
}

// echoChatModel mirrors the user's last message back. It implements both
// target chat capabilities so call and stream middleware share one fixture.
type echoChatModel struct {
	captured string
}

func (e *echoChatModel) capture(req *chat.Request) string {
	for index := len(req.Messages) - 1; index >= 0; index-- {
		if req.Messages[index].Role == chat.RoleUser {
			e.captured = req.Messages[index].Text()
			return e.captured
		}
	}
	return ""
}

func textResponse(text string) *chat.Response {
	message := chat.NewAssistantMessage(chat.NewTextPart(text))
	response, err := chat.NewResponse(&chat.Output{
		Message:      &message,
		FinishReason: chat.FinishReasonStop,
	}, nil)
	if err != nil {
		panic(err)
	}
	return response
}

func (e *echoChatModel) Call(_ context.Context, req *chat.Request) (*chat.Response, error) {
	return textResponse(e.capture(req)), nil
}

func (e *echoChatModel) Stream(_ context.Context, req *chat.Request) iter.Seq2[*chat.Response, error] {
	return func(yield func(*chat.Response, error) bool) {
		yield(textResponse(e.capture(req)), nil)
	}
}

func TestNewMiddlewareRejectsInvalidConfig(t *testing.T) {
	if _, err := rag.NewMiddleware(rag.MiddlewareConfig{}); err == nil {
		t.Fatal("missing retrievers must error")
	}
	var typedNilRetriever *stubRetriever
	if _, err := rag.NewMiddleware(rag.MiddlewareConfig{Retriever: typedNilRetriever}); err == nil {
		t.Fatal("typed nil retriever must error")
	}
}

func TestMiddlewarePreservesMissingCapabilities(t *testing.T) {
	middleware, err := rag.NewMiddleware(rag.MiddlewareConfig{Retriever: &stubRetriever{}})
	if err != nil {
		t.Fatal(err)
	}
	if middleware.Call(nil) != nil {
		t.Fatal("call middleware synthesized a model capability")
	}
	if middleware.Stream(nil) != nil {
		t.Fatal("stream middleware synthesized a streaming capability")
	}
}

func TestMiddlewareAugmentsRequestAndAttachesDocs(t *testing.T) {
	doc, _ := document.NewDocument("retrieved info", nil)
	retriever := &stubRetriever{docs: rag.Candidates{candidate(doc)}}
	aug, _ := rag.NewContextualAugmenter(rag.ContextualAugmenterConfig{})
	middleware, err := rag.NewMiddleware(rag.MiddlewareConfig{
		Retriever: retriever,
		Augmenter: aug,
	})
	if err != nil {
		t.Fatal(err)
	}

	model := &echoChatModel{}
	request, _ := chat.NewRequest(chat.NewUserMessage(chat.NewTextPart("what is RAG?")))
	response, err := middleware.Call(model).Call(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(model.captured, "retrieved info") {
		t.Fatalf("augmented user message did not embed retrieved doc: %q", model.captured)
	}
	docs, ok, err := rag.CandidatesFromResponse(response)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("retrieved candidates not attached to response")
	}
	if len(docs) != 1 {
		t.Fatalf("attached docs len = %d, want 1", len(docs))
	}
	citations, found, err := rag.CitationsFromResponse(response)
	if err != nil {
		t.Fatal(err)
	}
	if !found || len(citations) != 1 || citations[0].Marker() != "[1]" || citations[0].Candidate.Document.Text != doc.Text {
		t.Fatalf("citations = %#v, present %v", citations, found)
	}
}

func TestMiddlewarePreservesChatExtensionsAndExposesTypedHistory(t *testing.T) {
	var capturedHistory []chat.Message
	retriever := rag.RetrieverFunc(func(_ context.Context, query rag.Query) (rag.Candidates, error) {
		var err error
		capturedHistory, _, err = query.Value(rag.HistoryValueKey())
		return nil, err
	})
	middleware, err := rag.NewMiddleware(rag.MiddlewareConfig{Retriever: retriever})
	if err != nil {
		t.Fatal(err)
	}
	request, _ := chat.NewRequest(chat.NewUserMessage(chat.NewTextPart("question")))
	if setExtensionErr := request.Options.SetExtension("test/tenant", "acme"); setExtensionErr != nil {
		t.Fatal(setExtensionErr)
	}
	var downstreamTenant string
	model := chat.ModelFunc(func(_ context.Context, request *chat.Request) (*chat.Response, error) {
		var found bool
		var err error
		downstreamTenant, found, err = request.Options.Extensions.Decode[string]("test/tenant")
		if err != nil || !found {
			return nil, err
		}
		return textResponse("answer"), nil
	})
	if _, callErr := middleware.Call(model).Call(t.Context(), request); callErr != nil {
		t.Fatal(callErr)
	}
	if downstreamTenant != "acme" {
		t.Fatalf("downstream request extension = %q, want acme", downstreamTenant)
	}
	if len(capturedHistory) != 1 || capturedHistory[0].Text() != "question" {
		t.Fatalf("chat history = %#v, want the request message", capturedHistory)
	}
}

func TestMiddlewareStreamAugmentsOnceAndAttachesDocs(t *testing.T) {
	doc, _ := document.NewDocument("streamed context", nil)
	retriever := &countingRetriever{docs: rag.Candidates{candidate(doc)}}
	aug, _ := rag.NewContextualAugmenter(rag.ContextualAugmenterConfig{})
	middleware, err := rag.NewMiddleware(rag.MiddlewareConfig{Retriever: retriever, Augmenter: aug})
	if err != nil {
		t.Fatal(err)
	}

	model := &echoChatModel{}
	request, _ := chat.NewRequest(chat.NewUserMessage(chat.NewTextPart("question")))
	var chunks int
	for response, streamErr := range middleware.Stream(model).Stream(t.Context(), request) {
		if streamErr != nil {
			t.Fatal(streamErr)
		}
		chunks++
		if _, ok, decodeErr := rag.CandidatesFromResponse(response); decodeErr != nil || !ok {
			t.Fatalf("document extension = present %v, error %v", ok, decodeErr)
		}
	}
	if chunks != 1 || retriever.hits != 1 {
		t.Fatalf("chunks = %d, retrievals = %d; want 1, 1", chunks, retriever.hits)
	}
	if !strings.Contains(model.captured, "streamed context") {
		t.Fatalf("stream model did not see augmented text: %q", model.captured)
	}
}

type countingRetriever struct {
	docs rag.Candidates
	hits int
}

func (c *countingRetriever) Retrieve(_ context.Context, _ rag.Query) (rag.Candidates, error) {
	c.hits++
	return c.docs, nil
}

func TestMiddlewarePropagatesRetrieverError(t *testing.T) {
	want := errors.New("boom")
	middleware, err := rag.NewMiddleware(rag.MiddlewareConfig{Retriever: &errorRetriever{err: want}})
	if err != nil {
		t.Fatal(err)
	}

	request, _ := chat.NewRequest(chat.NewUserMessage(chat.NewTextPart("hi")))
	_, err = middleware.Call(&echoChatModel{}).Call(t.Context(), request)
	if !errors.Is(err, want) {
		t.Fatalf("err = %v", err)
	}
}

func TestMiddlewareRejectsInvalidAugmentation(t *testing.T) {
	middleware, err := rag.NewMiddleware(rag.MiddlewareConfig{
		Retriever: &stubRetriever{},
		Augmenter: rag.AugmenterFunc(func(context.Context, rag.Query, rag.Candidates) (rag.Augmentation, error) {
			return rag.Augmentation{}, nil
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	called := false
	model := chat.ModelFunc(func(context.Context, *chat.Request) (*chat.Response, error) {
		called = true
		return textResponse("unexpected"), nil
	})
	request, _ := chat.NewRequest(chat.NewUserMessage(chat.NewTextPart("question")))

	if _, err := middleware.Call(model).Call(t.Context(), request); !errors.Is(err, rag.ErrInvalidAugmentation) {
		t.Fatalf("invalid augmentation error = %v", err)
	}
	if called {
		t.Fatal("model called with an invalid augmentation")
	}
}

func TestMiddlewarePreservesPartialModelResponse(t *testing.T) {
	doc, _ := document.NewDocument("retrieved info", nil)
	middleware, err := rag.NewMiddleware(rag.MiddlewareConfig{
		Retriever: &stubRetriever{docs: rag.Candidates{candidate(doc)}},
	})
	if err != nil {
		t.Fatal(err)
	}
	wantErr := errors.New("partial model failure")
	partial := textResponse("partial")
	model := chat.ModelFunc(func(context.Context, *chat.Request) (*chat.Response, error) {
		return partial, wantErr
	})
	request, _ := chat.NewRequest(chat.NewUserMessage(chat.NewTextPart("question")))

	response, err := middleware.Call(model).Call(t.Context(), request)
	if response != partial || !errors.Is(err, wantErr) {
		t.Fatalf("response/error = %p/%v, want %p/%v", response, err, partial, wantErr)
	}
	if _, found, decodeErr := rag.CandidatesFromResponse(response); decodeErr != nil || !found {
		t.Fatalf("partial response document extension = present %v, error %v", found, decodeErr)
	}
}

type errorRetriever struct {
	err error
}

func (e *errorRetriever) Retrieve(_ context.Context, _ rag.Query) (rag.Candidates, error) {
	return nil, e.err
}

func TestMiddlewareDoesNotMutateCallerMessages(t *testing.T) {
	doc, _ := document.NewDocument("retrieved info", nil)
	retriever := &stubRetriever{docs: rag.Candidates{candidate(doc)}}
	aug, _ := rag.NewContextualAugmenter(rag.ContextualAugmenterConfig{})
	middleware, err := rag.NewMiddleware(rag.MiddlewareConfig{Retriever: retriever, Augmenter: aug})
	if err != nil {
		t.Fatal(err)
	}

	model := &echoChatModel{}
	userMessage := chat.NewUserMessage(chat.NewTextPart("what is RAG?"))
	request, _ := chat.NewRequest(userMessage)
	if _, err := middleware.Call(model).Call(t.Context(), request); err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(model.captured, "retrieved info") {
		t.Fatalf("model did not see augmented text: %q", model.captured)
	}
	if got := request.Messages[0].Text(); got != "what is RAG?" {
		t.Fatalf("caller message was mutated: %q", got)
	}
}

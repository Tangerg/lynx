package rag_test

import (
	"context"
	"errors"
	"iter"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/core/document"
	"github.com/Tangerg/lynx/core/metadata"
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
	if _, _, err := rag.NewMiddleware(rag.MiddlewareConfig{}); err == nil {
		t.Fatal("missing retrievers must error")
	}
	var typedNilRetriever *stubRetriever
	if _, _, err := rag.NewMiddleware(rag.MiddlewareConfig{Retriever: typedNilRetriever}); err == nil {
		t.Fatal("typed nil retriever must error")
	}
}

func TestMiddlewarePreservesMissingCapabilities(t *testing.T) {
	callMiddleware, streamMiddleware, err := rag.NewMiddleware(rag.MiddlewareConfig{Retriever: &stubRetriever{}})
	if err != nil {
		t.Fatal(err)
	}
	if callMiddleware(nil) != nil {
		t.Fatal("call middleware synthesized a model capability")
	}
	if streamMiddleware(nil) != nil {
		t.Fatal("stream middleware synthesized a streaming capability")
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

	model := &echoChatModel{}
	request, _ := chat.NewRequest(chat.NewUserMessage(chat.NewTextPart("what is RAG?")))
	response, err := callMW(model).Call(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(model.captured, "retrieved info") {
		t.Fatalf("augmented user message did not embed retrieved doc: %q", model.captured)
	}
	docs, ok, err := response.Metadata.Extra.Decode[[]rag.Candidate](rag.RetrievedCandidatesKey)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("RetrievedCandidatesKey not attached to response extensions")
	}
	if len(docs) != 1 {
		t.Fatalf("attached docs len = %d, want 1", len(docs))
	}
	raw := response.Metadata.Extra[rag.RetrievedCandidatesKey]
	if !strings.Contains(string(raw), `"document"`) || strings.Contains(string(raw), `"Document"`) {
		t.Fatalf("candidate metadata does not use the stable JSON model: %s", raw)
	}
}

func TestMiddlewareKeepsChatExtensionsAndHistoryInTypedSlots(t *testing.T) {
	var capturedExtensions metadata.Map
	var capturedHistory []chat.Message
	retriever := rag.RetrieverFunc(func(_ context.Context, query *rag.Query) ([]rag.Candidate, error) {
		var err error
		capturedExtensions, _, err = query.Value(rag.RequestOptionsExtensionsValueKey())
		if err != nil {
			return nil, err
		}
		capturedHistory, _, err = query.Value(rag.HistoryValueKey())
		return nil, err
	})
	callMiddleware, _, err := rag.NewMiddleware(rag.MiddlewareConfig{Retriever: retriever})
	if err != nil {
		t.Fatal(err)
	}
	request, _ := chat.NewRequest(chat.NewUserMessage(chat.NewTextPart("question")))
	if setExtensionErr := request.Options.SetExtension("test/tenant", "acme"); setExtensionErr != nil {
		t.Fatal(setExtensionErr)
	}
	if _, callErr := callMiddleware(&echoChatModel{}).Call(t.Context(), request); callErr != nil {
		t.Fatal(callErr)
	}
	tenant, found, err := capturedExtensions.Decode[string]("test/tenant")
	if err != nil || !found || tenant != "acme" {
		t.Fatalf("request extension = (%q, %v, %v), want (acme, true, nil)", tenant, found, err)
	}
	if len(capturedHistory) != 1 || capturedHistory[0].Text() != "question" {
		t.Fatalf("chat history = %#v, want the request message", capturedHistory)
	}
}

func TestMiddlewareStreamAugmentsOnceAndAttachesDocs(t *testing.T) {
	doc, _ := document.NewDocument("streamed context", nil)
	retriever := &countingRetriever{docs: []rag.Candidate{candidate(doc)}}
	aug, _ := rag.NewContextualAugmenter(rag.ContextualAugmenterConfig{})
	_, streamMW, err := rag.NewMiddleware(rag.MiddlewareConfig{Retriever: retriever, Augmenter: aug})
	if err != nil {
		t.Fatal(err)
	}

	model := &echoChatModel{}
	request, _ := chat.NewRequest(chat.NewUserMessage(chat.NewTextPart("question")))
	var chunks int
	for response, streamErr := range streamMW(model).Stream(t.Context(), request) {
		if streamErr != nil {
			t.Fatal(streamErr)
		}
		chunks++
		if _, ok, decodeErr := response.Metadata.Extra.Decode[[]rag.Candidate](rag.RetrievedCandidatesKey); decodeErr != nil || !ok {
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
	docs []rag.Candidate
	hits int
}

func (c *countingRetriever) Retrieve(_ context.Context, _ *rag.Query) ([]rag.Candidate, error) {
	c.hits++
	return c.docs, nil
}

func TestMiddlewarePropagatesRetrieverError(t *testing.T) {
	want := errors.New("boom")
	callMW, _, err := rag.NewMiddleware(rag.MiddlewareConfig{Retriever: &errorRetriever{err: want}})
	if err != nil {
		t.Fatal(err)
	}

	request, _ := chat.NewRequest(chat.NewUserMessage(chat.NewTextPart("hi")))
	_, err = callMW(&echoChatModel{}).Call(t.Context(), request)
	if !errors.Is(err, want) {
		t.Fatalf("err = %v", err)
	}
}

func TestMiddlewareRejectsInvalidAugmentedQuery(t *testing.T) {
	callMiddleware, _, err := rag.NewMiddleware(rag.MiddlewareConfig{
		Retriever: &stubRetriever{},
		Augmenter: rag.AugmenterFunc(func(context.Context, *rag.Query, []rag.Candidate) (*rag.Query, error) {
			return &rag.Query{}, nil
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

	if _, err := callMiddleware(model).Call(t.Context(), request); !errors.Is(err, rag.ErrInvalidQuery) {
		t.Fatalf("invalid augmented query error = %v", err)
	}
	if called {
		t.Fatal("model called with an invalid augmented query")
	}
}

func TestMiddlewarePreservesPartialModelResponse(t *testing.T) {
	doc, _ := document.NewDocument("retrieved info", nil)
	callMiddleware, _, err := rag.NewMiddleware(rag.MiddlewareConfig{
		Retriever: &stubRetriever{docs: []rag.Candidate{candidate(doc)}},
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

	response, err := callMiddleware(model).Call(t.Context(), request)
	if response != partial || !errors.Is(err, wantErr) {
		t.Fatalf("response/error = %p/%v, want %p/%v", response, err, partial, wantErr)
	}
	if _, found, decodeErr := response.Metadata.Extra.Decode[[]rag.Candidate](rag.RetrievedCandidatesKey); decodeErr != nil || !found {
		t.Fatalf("partial response document extension = present %v, error %v", found, decodeErr)
	}
}

type errorRetriever struct {
	err error
}

func (e *errorRetriever) Retrieve(_ context.Context, _ *rag.Query) ([]rag.Candidate, error) {
	return nil, e.err
}

func TestMiddlewareDoesNotMutateCallerMessages(t *testing.T) {
	doc, _ := document.NewDocument("retrieved info", nil)
	retriever := &stubRetriever{docs: []rag.Candidate{candidate(doc)}}
	aug, _ := rag.NewContextualAugmenter(rag.ContextualAugmenterConfig{})
	callMW, _, err := rag.NewMiddleware(rag.MiddlewareConfig{Retriever: retriever, Augmenter: aug})
	if err != nil {
		t.Fatal(err)
	}

	model := &echoChatModel{}
	userMessage := chat.NewUserMessage(chat.NewTextPart("what is RAG?"))
	request, _ := chat.NewRequest(userMessage)
	if _, err := callMW(model).Call(t.Context(), request); err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(model.captured, "retrieved info") {
		t.Fatalf("model did not see augmented text: %q", model.captured)
	}
	if got := request.Messages[0].Text(); got != "what is RAG?" {
		t.Fatalf("caller message was mutated: %q", got)
	}
}

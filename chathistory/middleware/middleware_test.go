package middleware_test

import (
	"context"
	"errors"
	"iter"
	"reflect"
	"sync"
	"testing"

	"github.com/Tangerg/lynx/chathistory"
	historymw "github.com/Tangerg/lynx/chathistory/middleware"
	"github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/core/media"
	"github.com/Tangerg/lynx/core/metadata"
)

func TestNewRejectsNilStore(t *testing.T) {
	if _, err := historymw.New(nil); !errors.Is(err, chathistory.ErrNilStore) {
		t.Fatalf("New(nil) error = %v", err)
	}
}

func TestCallReplaysInStableOrderAndPersistsOnlyFreshExchange(t *testing.T) {
	store := &recordingStore{read: []chat.Message{
		chat.NewSystemMessage("stale stored system"),
		chat.NewUserMessage(chat.NewTextPart("stored user")),
	}}
	middleware := mustMiddleware(t, store)

	var received *chat.Request
	model := chat.ModelFunc(func(_ context.Context, request *chat.Request) (*chat.Response, error) {
		received = request
		return response(chat.NewAssistantMessage(chat.NewTextPart("answer"))), nil
	})
	request := mustRequest(t,
		chat.NewUserMessage(chat.NewTextPart("fresh user")),
		chat.NewSystemMessage("live system"),
	)
	got, err := middleware.Call(model).Call(boundContext(t), request)
	if err != nil {
		t.Fatal(err)
	}
	if got.Text() != "answer" {
		t.Fatalf("response text = %q", got.Text())
	}
	assertMessages(t, received.Messages,
		messageKey{chat.RoleSystem, "live system"},
		messageKey{chat.RoleUser, "stored user"},
		messageKey{chat.RoleUser, "fresh user"},
	)
	writes := store.writesSnapshot()
	if len(writes) != 1 {
		t.Fatalf("write batches = %d", len(writes))
	}
	assertMessages(t, writes[0],
		messageKey{chat.RoleUser, "fresh user"},
		messageKey{chat.RoleAssistant, "answer"},
	)
}

func TestCallWithoutConversationIsTransparent(t *testing.T) {
	store := &recordingStore{}
	middleware := mustMiddleware(t, store)
	request := mustRequest(t, chat.NewUserMessage(chat.NewTextPart("hello")))
	var samePointer bool
	model := chat.ModelFunc(func(_ context.Context, got *chat.Request) (*chat.Response, error) {
		samePointer = got == request
		return response(chat.NewAssistantMessage(chat.NewTextPart("answer"))), nil
	})
	if _, err := middleware.Call(model).Call(t.Context(), request); err != nil {
		t.Fatal(err)
	}
	if !samePointer {
		t.Fatal("unbound request was not passed through unchanged")
	}
	if reads, writes := store.counts(); reads != 0 || writes != 0 {
		t.Fatalf("store calls = read %d, write %d", reads, writes)
	}
}

func TestCallRejectsInvalidConversationBeforeStoreOrModel(t *testing.T) {
	store := &recordingStore{}
	middleware := mustMiddleware(t, store)
	called := false
	model := chat.ModelFunc(func(context.Context, *chat.Request) (*chat.Response, error) {
		called = true
		return nil, nil
	})
	ctx := chathistory.WithConversationID(t.Context(), " padded")
	_, err := middleware.Call(model).Call(ctx, mustRequest(t, chat.NewUserMessage(chat.NewTextPart("hello"))))
	if !errors.Is(err, chathistory.ErrInvalidConversationID) {
		t.Fatalf("Call error = %v", err)
	}
	if called {
		t.Fatal("model was called")
	}
	if reads, writes := store.counts(); reads != 0 || writes != 0 {
		t.Fatalf("store calls = read %d, write %d", reads, writes)
	}
}

func TestCallPreservesReadModelAndWriteErrors(t *testing.T) {
	readErr := errors.New("read failed")
	store := &recordingStore{readErr: readErr}
	middleware := mustMiddleware(t, store)
	modelCalled := false
	model := chat.ModelFunc(func(context.Context, *chat.Request) (*chat.Response, error) {
		modelCalled = true
		return nil, nil
	})
	request := mustRequest(t, chat.NewUserMessage(chat.NewTextPart("hello")))
	if _, err := middleware.Call(model).Call(boundContext(t), request); !errors.Is(err, readErr) {
		t.Fatalf("read error = %v", err)
	}
	if modelCalled {
		t.Fatal("model called after read failure")
	}

	modelErr := errors.New("model failed")
	modelResponse := response(chat.NewAssistantMessage(chat.NewTextPart("partial")))
	store = &recordingStore{}
	middleware = mustMiddleware(t, store)
	got, err := middleware.Call(chat.ModelFunc(func(context.Context, *chat.Request) (*chat.Response, error) {
		return modelResponse, modelErr
	})).Call(boundContext(t), request)
	if got != modelResponse || !errors.Is(err, modelErr) {
		t.Fatalf("model result = %p, %v", got, err)
	}
	if _, writes := store.counts(); writes != 0 {
		t.Fatalf("writes after model error = %d", writes)
	}

	writeErr := errors.New("write failed")
	store = &recordingStore{writeErr: writeErr}
	middleware = mustMiddleware(t, store)
	modelResponse = response(chat.NewAssistantMessage(chat.NewTextPart("answer")))
	got, err = middleware.Call(chat.ModelFunc(func(context.Context, *chat.Request) (*chat.Response, error) {
		return modelResponse, nil
	})).Call(boundContext(t), request)
	if got != modelResponse || !errors.Is(err, writeErr) {
		t.Fatalf("write result = %p, %v", got, err)
	}
}

func TestCallDefersToolCallUntilCompleteToolExchange(t *testing.T) {
	store := &recordingStore{}
	middleware := mustMiddleware(t, store)
	user := chat.NewUserMessage(chat.NewTextPart("weather?"))
	call := chat.NewAssistantMessage(chat.NewToolCallPart(chat.ToolCall{
		ID: "call-1", Name: "weather", Arguments: `{}`,
	}))
	tool := chat.NewToolMessage(chat.ToolResult{ID: "call-1", Name: "weather", Result: "sunny"})

	if _, err := middleware.Call(chat.ModelFunc(func(context.Context, *chat.Request) (*chat.Response, error) {
		return response(call), nil
	})).Call(boundContext(t), mustRequest(t, user)); err != nil {
		t.Fatal(err)
	}
	if _, writes := store.counts(); writes != 0 {
		t.Fatalf("tool-call turn writes = %d", writes)
	}

	if _, err := middleware.Call(chat.ModelFunc(func(context.Context, *chat.Request) (*chat.Response, error) {
		return response(chat.NewAssistantMessage(chat.NewTextPart("sunny"))), nil
	})).Call(boundContext(t), mustRequest(t, user, call, tool)); err != nil {
		t.Fatal(err)
	}
	writes := store.writesSnapshot()
	if len(writes) != 1 {
		t.Fatalf("write batches = %d", len(writes))
	}
	if len(writes[0]) != 4 || writes[0][0].Role != chat.RoleUser || writes[0][1].Role != chat.RoleAssistant || writes[0][2].Role != chat.RoleTool || writes[0][3].Text() != "sunny" {
		t.Fatalf("complete exchange = %#v", writes[0])
	}
}

func TestCallSnapshotsAllRequestReferences(t *testing.T) {
	image, err := media.NewBytes("image/png", []byte{1, 2, 3})
	if err != nil {
		t.Fatal(err)
	}
	user := chat.NewUserMessage(chat.NewMediaPart(image))
	user.Metadata = metadata.New()
	if err := metadata.Set(user.Metadata, "turn", 1); err != nil {
		t.Fatal(err)
	}
	temperature := 0.5
	request := mustRequest(t, user)
	request.Tools = []chat.ToolDefinition{{Name: "weather", InputSchema: []byte(`{"type":"object"}`)}}
	request.Options = chat.Options{Temperature: &temperature, Stop: []string{"END"}}
	if err := request.SetExtension("test/value", "original"); err != nil {
		t.Fatal(err)
	}

	store := &recordingStore{}
	middleware := mustMiddleware(t, store)
	_, err = middleware.Call(chat.ModelFunc(func(_ context.Context, got *chat.Request) (*chat.Response, error) {
		got.Messages[0].Metadata["turn"][0] = '9'
		got.Messages[0].Parts[0].Media.Source.Bytes[0] = 9
		got.Tools[0].InputSchema[0] = '['
		*got.Options.Temperature = 1.5
		got.Options.Stop[0] = "MUTATED"
		got.Extensions["test/value"][1] = 'X'
		return response(chat.NewAssistantMessage(chat.NewTextPart("answer"))), nil
	})).Call(boundContext(t), request)
	if err != nil {
		t.Fatal(err)
	}
	if string(request.Messages[0].Metadata["turn"]) != "1" || request.Messages[0].Parts[0].Media.Source.Bytes[0] != 1 || request.Tools[0].InputSchema[0] != '{' || *request.Options.Temperature != 0.5 || request.Options.Stop[0] != "END" || string(request.Extensions["test/value"]) != `"original"` {
		t.Fatalf("caller request was mutated: %#v", request)
	}
	writes := store.writesSnapshot()
	if len(writes) != 1 || string(writes[0][0].Metadata["turn"]) != "1" || writes[0][0].Parts[0].Media.Source.Bytes[0] != 1 {
		t.Fatalf("persisted fresh message was aliased: %#v", writes)
	}
}

func TestStreamIsLazyAndPersistsOnlyNaturalCompletion(t *testing.T) {
	store := &recordingStore{}
	middleware := mustMiddleware(t, store)
	started := false
	closed := false
	streamer := chat.StreamerFunc(func(context.Context, *chat.Request) iter.Seq2[*chat.Response, error] {
		started = true
		return func(yield func(*chat.Response, error) bool) {
			defer func() { closed = true }()
			if !yield(chunk("hel", ""), nil) {
				return
			}
			yield(chunk("lo", chat.FinishReasonStop), nil)
		}
	})
	sequence := middleware.Stream(streamer).Stream(boundContext(t), mustRequest(t, chat.NewUserMessage(chat.NewTextPart("hello"))))
	if started {
		t.Fatal("streamer started before iteration")
	}
	if reads, _ := store.counts(); reads != 0 {
		t.Fatalf("reads before iteration = %d", reads)
	}
	var texts []string
	for response, err := range sequence {
		if err != nil {
			t.Fatal(err)
		}
		texts = append(texts, response.Text())
	}
	if !started || !closed || !reflect.DeepEqual(texts, []string{"hel", "lo"}) {
		t.Fatalf("stream state started=%v closed=%v texts=%v", started, closed, texts)
	}
	writes := store.writesSnapshot()
	if len(writes) != 1 {
		t.Fatalf("write batches = %d", len(writes))
	}
	assertMessages(t, writes[0],
		messageKey{chat.RoleUser, "hello"},
		messageKey{chat.RoleAssistant, "hello"},
	)
}

func TestStreamWithoutConversationForwardsWithoutHistory(t *testing.T) {
	store := &recordingStore{}
	middleware := mustMiddleware(t, store)
	request := mustRequest(t, chat.NewUserMessage(chat.NewTextPart("hello")))
	samePointer := false
	streamer := chat.StreamerFunc(func(_ context.Context, got *chat.Request) iter.Seq2[*chat.Response, error] {
		samePointer = got == request
		return func(yield func(*chat.Response, error) bool) {
			yield(chunk("answer", chat.FinishReasonStop), nil)
		}
	})
	var got string
	for response, err := range middleware.Stream(streamer).Stream(t.Context(), request) {
		if err != nil {
			t.Fatal(err)
		}
		got = response.Text()
	}
	if !samePointer || got != "answer" {
		t.Fatalf("forward result samePointer=%v text=%q", samePointer, got)
	}
	if reads, writes := store.counts(); reads != 0 || writes != 0 {
		t.Fatalf("store calls = read %d, write %d", reads, writes)
	}

	nilStreamer := chat.StreamerFunc(func(context.Context, *chat.Request) iter.Seq2[*chat.Response, error] { return nil })
	var gotErr error
	for _, err := range middleware.Stream(nilStreamer).Stream(t.Context(), request) {
		gotErr = err
	}
	if !errors.Is(gotErr, historymw.ErrNilStream) {
		t.Fatalf("nil forwarded sequence error = %v", gotErr)
	}
}

func TestStreamDoesNotPersistOnEarlyStopProviderErrorOrMalformedChunk(t *testing.T) {
	t.Run("early stop", func(t *testing.T) {
		store := &recordingStore{}
		middleware := mustMiddleware(t, store)
		closed := false
		streamer := chat.StreamerFunc(func(context.Context, *chat.Request) iter.Seq2[*chat.Response, error] {
			return func(yield func(*chat.Response, error) bool) {
				defer func() { closed = true }()
				if !yield(chunk("first", ""), nil) {
					return
				}
				yield(chunk("second", chat.FinishReasonStop), nil)
			}
		})
		sequence := middleware.Stream(streamer).Stream(boundContext(t), mustRequest(t, chat.NewUserMessage(chat.NewTextPart("hello"))))
		sequence(func(*chat.Response, error) bool { return false })
		if !closed {
			t.Fatal("provider resources were not released")
		}
		if _, writes := store.counts(); writes != 0 {
			t.Fatalf("writes = %d", writes)
		}
	})

	t.Run("provider error", func(t *testing.T) {
		providerErr := errors.New("provider failed")
		store := &recordingStore{}
		middleware := mustMiddleware(t, store)
		streamer := chat.StreamerFunc(func(context.Context, *chat.Request) iter.Seq2[*chat.Response, error] {
			return func(yield func(*chat.Response, error) bool) {
				if !yield(chunk("partial", ""), nil) {
					return
				}
				yield(nil, providerErr)
			}
		})
		var gotErr error
		for _, err := range middleware.Stream(streamer).Stream(boundContext(t), mustRequest(t, chat.NewUserMessage(chat.NewTextPart("hello")))) {
			if err != nil {
				gotErr = err
			}
		}
		if !errors.Is(gotErr, providerErr) {
			t.Fatalf("stream error = %v", gotErr)
		}
		if _, writes := store.counts(); writes != 0 {
			t.Fatalf("writes = %d", writes)
		}
	})

	t.Run("malformed chunk", func(t *testing.T) {
		store := &recordingStore{}
		middleware := mustMiddleware(t, store)
		streamer := chat.StreamerFunc(func(context.Context, *chat.Request) iter.Seq2[*chat.Response, error] {
			return func(yield func(*chat.Response, error) bool) {
				yield(&chat.Response{Choices: []chat.Choice{{Index: -1}}}, nil)
			}
		})
		var errorsSeen int
		for _, err := range middleware.Stream(streamer).Stream(boundContext(t), mustRequest(t, chat.NewUserMessage(chat.NewTextPart("hello")))) {
			if err != nil {
				errorsSeen++
			}
		}
		if errorsSeen != 1 {
			t.Fatalf("errors seen = %d", errorsSeen)
		}
		if _, writes := store.counts(); writes != 0 {
			t.Fatalf("writes = %d", writes)
		}
	})
}

func TestStreamReportsWriteFailureAndNilSequence(t *testing.T) {
	writeErr := errors.New("write failed")
	store := &recordingStore{writeErr: writeErr}
	middleware := mustMiddleware(t, store)
	streamer := chat.StreamerFunc(func(context.Context, *chat.Request) iter.Seq2[*chat.Response, error] {
		return func(yield func(*chat.Response, error) bool) {
			yield(chunk("answer", chat.FinishReasonStop), nil)
		}
	})
	var gotErr error
	var chunks int
	for response, err := range middleware.Stream(streamer).Stream(boundContext(t), mustRequest(t, chat.NewUserMessage(chat.NewTextPart("hello")))) {
		if err != nil {
			gotErr = err
		} else if response != nil {
			chunks++
		}
	}
	if chunks != 1 || !errors.Is(gotErr, writeErr) {
		t.Fatalf("stream result chunks=%d error=%v", chunks, gotErr)
	}

	store = &recordingStore{}
	middleware = mustMiddleware(t, store)
	nilStreamer := chat.StreamerFunc(func(context.Context, *chat.Request) iter.Seq2[*chat.Response, error] { return nil })
	gotErr = nil
	for _, err := range middleware.Stream(nilStreamer).Stream(boundContext(t), mustRequest(t, chat.NewUserMessage(chat.NewTextPart("hello")))) {
		gotErr = err
	}
	if !errors.Is(gotErr, historymw.ErrNilStream) {
		t.Fatalf("nil sequence error = %v", gotErr)
	}
}

func TestMiddlewarePreservesContextCancellation(t *testing.T) {
	store := chathistory.NewInMemoryStore()
	middleware := mustMiddleware(t, store)
	ctx, cancel := context.WithCancel(boundContext(t))
	cancel()
	_, err := middleware.Call(chat.ModelFunc(func(context.Context, *chat.Request) (*chat.Response, error) {
		t.Fatal("model called after cancellation")
		return nil, nil
	})).Call(ctx, mustRequest(t, chat.NewUserMessage(chat.NewTextPart("hello"))))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation error = %v", err)
	}
}

type recordingStore struct {
	mu         sync.Mutex
	read       []chat.Message
	writes     [][]chat.Message
	readErr    error
	writeErr   error
	readCalls  int
	writeCalls int
}

func (s *recordingStore) Read(context.Context, string) ([]chat.Message, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.readCalls++
	if s.readErr != nil {
		return nil, s.readErr
	}
	return cloneMessages(s.read), nil
}

func (s *recordingStore) Write(_ context.Context, _ string, messages ...chat.Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.writeCalls++
	if s.writeErr != nil {
		return s.writeErr
	}
	s.writes = append(s.writes, cloneMessages(messages))
	return nil
}

func (s *recordingStore) counts() (int, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.readCalls, s.writeCalls
}

func (s *recordingStore) writesSnapshot() [][]chat.Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	cloned := make([][]chat.Message, len(s.writes))
	for i := range s.writes {
		cloned[i] = cloneMessages(s.writes[i])
	}
	return cloned
}

func cloneMessages(messages []chat.Message) []chat.Message {
	raw := make([]chat.Message, len(messages))
	for i := range messages {
		encoded, err := messages[i].MarshalJSON()
		if err != nil {
			panic(err)
		}
		if err := raw[i].UnmarshalJSON(encoded); err != nil {
			panic(err)
		}
	}
	return raw
}

type messageKey struct {
	role chat.Role
	text string
}

func assertMessages(t *testing.T, messages []chat.Message, want ...messageKey) {
	t.Helper()
	if len(messages) != len(want) {
		t.Fatalf("messages len = %d, want %d: %#v", len(messages), len(want), messages)
	}
	for i := range want {
		if messages[i].Role != want[i].role || messages[i].Text() != want[i].text {
			t.Fatalf("messages[%d] = %s/%q, want %s/%q", i, messages[i].Role, messages[i].Text(), want[i].role, want[i].text)
		}
	}
}

func mustMiddleware(t *testing.T, store historymw.Store) *historymw.Middleware {
	t.Helper()
	middleware, err := historymw.New(store)
	if err != nil {
		t.Fatal(err)
	}
	return middleware
}

func mustRequest(t *testing.T, messages ...chat.Message) *chat.Request {
	t.Helper()
	request, err := chat.NewRequest(messages...)
	if err != nil {
		t.Fatal(err)
	}
	return request
}

func boundContext(t *testing.T) context.Context {
	t.Helper()
	return chathistory.WithConversationID(t.Context(), "conversation-1")
}

func response(message chat.Message) *chat.Response {
	return &chat.Response{Choices: []chat.Choice{{Index: 0, Message: &message, FinishReason: chat.FinishReasonStop}}}
}

func chunk(text string, finish chat.FinishReason) *chat.Response {
	message := chat.NewAssistantMessage(chat.NewTextPart(text))
	return &chat.Response{Choices: []chat.Choice{{Index: 0, Message: &message, FinishReason: finish}}}
}

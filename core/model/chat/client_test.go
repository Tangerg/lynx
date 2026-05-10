package chat_test

import (
	"context"
	"errors"
	"iter"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/core/model/chat"
)

// fakeChatModel is a hand-rolled mock of [chat.Model] used across the
// client test suite. Each call captures the request so tests can assert
// what reached the model layer.
type fakeChatModel struct {
	provider     string
	defaultOpts  *chat.Options
	lastReq      *chat.Request
	respond      func(req *chat.Request) (*chat.Response, error)
	streamYield  []*chat.Response
	streamErr    error
}

func newFakeChatModel(t *testing.T) *fakeChatModel {
	t.Helper()
	defaults, err := chat.NewOptions("fake-model")
	if err != nil {
		t.Fatalf("NewOptions: %v", err)
	}
	return &fakeChatModel{
		provider:    "fake",
		defaultOpts: defaults,
	}
}

func (m *fakeChatModel) DefaultOptions() *chat.Options { return m.defaultOpts }
func (m *fakeChatModel) Info() chat.ModelInfo          { return chat.ModelInfo{Provider: m.provider} }

func (m *fakeChatModel) Call(ctx context.Context, req *chat.Request) (*chat.Response, error) {
	m.lastReq = req
	if m.respond != nil {
		return m.respond(req)
	}
	return responseWithText("hi back"), nil
}

func (m *fakeChatModel) Stream(ctx context.Context, req *chat.Request) iter.Seq2[*chat.Response, error] {
	m.lastReq = req
	return func(yield func(*chat.Response, error) bool) {
		if m.streamErr != nil {
			yield(nil, m.streamErr)
			return
		}
		for _, resp := range m.streamYield {
			if !yield(resp, nil) {
				return
			}
		}
	}
}

func responseWithText(text string) *chat.Response {
	resp, _ := chat.NewResponse(
		[]*chat.Result{{
			AssistantMessage: chat.NewAssistantMessage(text),
			Metadata:         &chat.ResultMetadata{FinishReason: chat.FinishReasonStop},
		}},
		&chat.ResponseMetadata{},
	)
	return resp
}

func TestNewClientRequest_RejectsNilModel(t *testing.T) {
	if _, err := chat.NewClientRequest(nil); err == nil {
		t.Fatal("nil model must error")
	}
}

func TestClientRequest_Build_DefaultGreetingWhenEmpty(t *testing.T) {
	model := newFakeChatModel(t)
	req, err := chat.NewClientRequest(model)
	if err != nil {
		t.Fatal(err)
	}

	resp, err := req.Call().Response(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if resp == nil {
		t.Fatal("nil response")
	}

	if model.lastReq == nil {
		t.Fatal("model never received a request")
	}
	if len(model.lastReq.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(model.lastReq.Messages))
	}
	user, ok := model.lastReq.Messages[0].(*chat.UserMessage)
	if !ok {
		t.Fatalf("first message type = %T, want *UserMessage", model.lastReq.Messages[0])
	}
	if user.Text != "Hi!" {
		t.Fatalf("default greeting = %q, want %q", user.Text, "Hi!")
	}
}

func TestClientRequest_WithMessagesPassesThrough(t *testing.T) {
	model := newFakeChatModel(t)
	req, _ := chat.NewClientRequest(model)
	req.WithMessages(chat.NewUserMessage("custom"))

	if _, err := req.Call().Response(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := model.lastReq.Messages[0].(*chat.UserMessage).Text; got != "custom" {
		t.Fatalf("Text = %q, want custom", got)
	}
}

func TestClientRequest_WithSystemPrompt_PrependsSystemMessage(t *testing.T) {
	model := newFakeChatModel(t)
	req, _ := chat.NewClientRequest(model)
	req.
		WithSystemPrompt("Be concise.").
		WithMessages(chat.NewUserMessage("hi"))

	if _, err := req.Call().Response(context.Background()); err != nil {
		t.Fatal(err)
	}

	if len(model.lastReq.Messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(model.lastReq.Messages))
	}
	sys, ok := model.lastReq.Messages[0].(*chat.SystemMessage)
	if !ok {
		t.Fatalf("first message type = %T, want *SystemMessage", model.lastReq.Messages[0])
	}
	if sys.Text != "Be concise." {
		t.Fatalf("system Text = %q", sys.Text)
	}
}

func TestClientRequest_WithMiddlewares_AppliesToCall(t *testing.T) {
	model := newFakeChatModel(t)
	req, _ := chat.NewClientRequest(model)

	calls := 0
	mw := chat.CallMiddleware(func(next chat.CallHandler) chat.CallHandler {
		return chat.CallHandlerFunc(func(ctx context.Context, r *chat.Request) (*chat.Response, error) {
			calls++
			return next.Call(ctx, r)
		})
	})
	req.WithMiddlewares(mw).WithMessages(chat.NewUserMessage("hi"))

	if _, err := req.Call().Response(context.Background()); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("middleware ran %d times, want 1", calls)
	}
}

func TestClientCaller_Text_ReturnsAssistantTextAndResponse(t *testing.T) {
	model := newFakeChatModel(t)
	model.respond = func(*chat.Request) (*chat.Response, error) { return responseWithText("answer"), nil }

	req, _ := chat.NewClientRequest(model)
	req.WithMessages(chat.NewUserMessage("hi"))

	text, resp, err := req.Call().Text(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if text != "answer" {
		t.Fatalf("text = %q, want answer", text)
	}
	if resp == nil {
		t.Fatal("response is nil")
	}
}

func TestClientCaller_Structured_AppendsParserInstructions(t *testing.T) {
	model := newFakeChatModel(t)
	model.respond = func(req *chat.Request) (*chat.Response, error) {
		return responseWithText(`["a","b"]`), nil
	}

	req, _ := chat.NewClientRequest(model)
	req.WithMessages(chat.NewUserMessage("seed"))

	type listType []string
	parser := chat.WrapParserAsAny(chat.NewJSONParser[listType]())

	got, _, err := req.Call().Structured(context.Background(), parser)
	if err != nil {
		t.Fatal(err)
	}

	// User message should be augmented with parser instructions.
	user := model.lastReq.Messages[0].(*chat.UserMessage)
	if !strings.Contains(user.Text, "JSON") {
		t.Fatalf("user message did not gain parser instructions: %q", user.Text)
	}
	parsed, ok := got.(listType)
	if !ok {
		t.Fatalf("parser returned %T, want listType", got)
	}
	if len(parsed) != 2 {
		t.Fatalf("len = %d, want 2", len(parsed))
	}
}

func TestClientCaller_Response_PropagatesError(t *testing.T) {
	want := errors.New("boom")
	model := newFakeChatModel(t)
	model.respond = func(*chat.Request) (*chat.Response, error) { return nil, want }

	req, _ := chat.NewClientRequest(model)
	req.WithMessages(chat.NewUserMessage("hi"))

	if _, err := req.Call().Response(context.Background()); !errors.Is(err, want) {
		t.Fatalf("err = %v, want %v", err, want)
	}
}

func TestClientStreamer_Text_StreamsChunks(t *testing.T) {
	model := newFakeChatModel(t)
	model.streamYield = []*chat.Response{
		responseWithText("Hello, "),
		responseWithText("world."),
	}

	req, _ := chat.NewClientRequest(model)
	req.WithMessages(chat.NewUserMessage("seed"))

	got := make([]string, 0, 2)
	for chunk, err := range req.Stream().Text(context.Background()) {
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, chunk)
	}
	if len(got) != 2 {
		t.Fatalf("got %d chunks, want 2", len(got))
	}
}

func TestClient_ChatWithTextBuildsUserMessage(t *testing.T) {
	model := newFakeChatModel(t)
	client, err := chat.NewClient(model)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := client.ChatWithText("hello").Call().Response(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := model.lastReq.Messages[0].(*chat.UserMessage).Text; got != "hello" {
		t.Fatalf("Text = %q, want hello", got)
	}
}

func TestNewClient_RejectsNilModel(t *testing.T) {
	if _, err := chat.NewClient(nil); err == nil {
		t.Fatal("nil model must error")
	}
}

func TestNewClientFromRequest_RejectsNil(t *testing.T) {
	if _, err := chat.NewClientFromRequest(nil); err == nil {
		t.Fatal("nil request must error")
	}
}

func TestClient_ChatClonesDefaults(t *testing.T) {
	model := newFakeChatModel(t)
	defaultReq, _ := chat.NewClientRequest(model)
	defaultReq.WithMessages(chat.NewUserMessage("default"))

	client, _ := chat.NewClientFromRequest(defaultReq)

	a := client.Chat()
	b := client.Chat()
	a.WithMessages(chat.NewUserMessage("modified"))

	// b must remain unaffected.
	if _, err := b.Call().Response(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := model.lastReq.Messages[0].(*chat.UserMessage).Text; got != "default" {
		t.Fatalf("clone leaked: Text = %q, want default", got)
	}
}

func TestClient_ChatWithRequest_CopiesMessagesOptionsParams(t *testing.T) {
	model := newFakeChatModel(t)
	client, _ := chat.NewClient(model)

	src, _ := chat.NewRequest([]chat.Message{chat.NewUserMessage("src")})
	src.Options, _ = chat.NewOptions("override-model")
	src.Set("trace", "abc")

	if _, err := client.ChatWithRequest(src).Call().Response(context.Background()); err != nil {
		t.Fatal(err)
	}

	if model.lastReq.Options.Model != "override-model" {
		t.Fatalf("Options.Model = %q, want override-model", model.lastReq.Options.Model)
	}
	if v, _ := model.lastReq.Get("trace"); v != "abc" {
		t.Fatalf("Param trace = %v, want abc", v)
	}
}

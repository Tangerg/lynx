package toolloop

import (
	"context"
	"iter"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/core/model/chat"
)

type fakeChatModel struct {
	provider    string
	defaultOpts *chat.Options
	lastReq     *chat.Request
	respond     func(req *chat.Request) (*chat.Response, error)
	streamYield []*chat.Response
	streamErr   error

	streamRespond func(req *chat.Request) []*chat.Response
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

func (m *fakeChatModel) DefaultOptions() chat.Options { return *m.defaultOpts }
func (m *fakeChatModel) Metadata() chat.ModelMetadata {
	return chat.ModelMetadata{Provider: m.provider}
}

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
		chunks := m.streamYield
		if m.streamRespond != nil {
			chunks = m.streamRespond(req)
		}
		for _, resp := range chunks {
			if !yield(resp, nil) {
				return
			}
		}
	}
}

func responseWithText(text string) *chat.Response {
	resp, _ := chat.NewResponse(
		&chat.Result{
			AssistantMessage: chat.NewAssistantMessage(text),
			Metadata:         &chat.ResultMetadata{FinishReason: chat.FinishReasonStop},
		},
		&chat.ResponseMetadata{},
	)
	return resp
}

func mustNewTool(t *testing.T, name string) chat.Tool {
	t.Helper()
	tl, err := chat.NewTool(
		chat.ToolDefinition{Name: name, InputSchema: `{"type":"object"}`},
		func(context.Context, string) (string, error) { return "", nil },
	)
	if err != nil {
		t.Fatalf("NewTool: %v", err)
	}
	return tl
}

func mustNewCallable(t *testing.T, name string, returnDirect bool, fn func(context.Context, string) (string, error)) chat.Tool {
	t.Helper()
	if fn == nil {
		fn = func(context.Context, string) (string, error) { return "", nil }
	}
	tl, err := chat.NewTool(
		chat.ToolDefinition{Name: name, InputSchema: `{"type":"object"}`},
		fn,
	)
	if err != nil {
		t.Fatalf("NewTool: %v", err)
	}
	if returnDirect {
		return ReturnDirect(tl)
	}
	return tl
}

func mustNewRequest(t *testing.T) *chat.Request {
	t.Helper()
	req, err := chat.NewRequest([]chat.Message{chat.NewUserMessage("hi")})
	if err != nil {
		t.Fatal(err)
	}
	opts, _ := chat.NewOptions("m")
	req.Options = opts
	return req
}

func responseWithToolCall(t *testing.T, name, args string) *chat.Response {
	t.Helper()
	resp, err := chat.NewResponse(
		&chat.Result{
			AssistantMessage: chat.NewAssistantMessage(chat.MessageParams{
				Parts: []chat.OutputPart{&chat.ToolCallPart{
					ID:        "call_1",
					Name:      name,
					Arguments: args,
				}},
			}),
			Metadata: &chat.ResultMetadata{FinishReason: chat.FinishReasonToolCalls},
		},
		&chat.ResponseMetadata{},
	)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func twoToolCallResponse(first, second string) *chat.Response {
	resp, _ := chat.NewResponse(
		&chat.Result{
			AssistantMessage: chat.NewAssistantMessage(chat.MessageParams{
				Parts: []chat.OutputPart{
					&chat.ToolCallPart{ID: "c1", Name: first, Arguments: "{}"},
					&chat.ToolCallPart{ID: "c2", Name: second, Arguments: "{}"},
				},
			}),
			Metadata: &chat.ResultMetadata{FinishReason: chat.FinishReasonToolCalls},
		},
		&chat.ResponseMetadata{},
	)
	return resp
}

func collectStream(seq func(func(*chat.Response, error) bool)) (msgs []chat.Message, finalText string, err error) {
	for resp, e := range seq {
		if e != nil {
			return msgs, finalText, e
		}
		if resp == nil || resp.Result == nil {
			continue
		}
		if am := resp.Result.AssistantMessage; am != nil {
			if txt := am.JoinedText(); txt != "" {
				finalText = txt
			}
		}
	}
	return msgs, finalText, nil
}

func toolCallResponseID(t *testing.T, id, name string) *chat.Response {
	t.Helper()
	resp, err := chat.NewResponse(&chat.Result{
		AssistantMessage: chat.NewAssistantMessage(chat.MessageParams{
			Parts: []chat.OutputPart{&chat.ToolCallPart{ID: id, Name: name, Arguments: "{}"}},
		}),
		Metadata: &chat.ResultMetadata{FinishReason: chat.FinishReasonToolCalls},
	}, &chat.ResponseMetadata{})
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func countToolMsgs(msgs []chat.Message) int {
	n := 0
	for _, m := range msgs {
		if m.Type() == chat.MessageTypeTool {
			n++
		}
	}
	return n
}

func assertValidToolHistory(t *testing.T, msgs []chat.Message) {
	t.Helper()
	calls := map[string]bool{}
	answered := map[string]bool{}
	for i, m := range msgs {
		switch msg := m.(type) {
		case *chat.AssistantMessage:
			for _, c := range msg.CollectToolCalls() {
				calls[c.ID] = true
			}
		case *chat.ToolMessage:
			prev, ok := msgs[i-1].(*chat.AssistantMessage)
			if i == 0 || !ok || !prev.HasToolCalls() {
				t.Fatalf("tool message at %d not preceded by assistant(tool_calls): history=%s", i, historyShape(msgs))
			}
			for _, ret := range msg.ToolReturns {
				answered[ret.ID] = true
				if !calls[ret.ID] {
					t.Fatalf("tool return id %q has no matching tool_call: history=%s", ret.ID, historyShape(msgs))
				}
			}
		}
	}
	for id := range calls {
		if !answered[id] {
			t.Fatalf("tool_call id %q has no tool response: history=%s", id, historyShape(msgs))
		}
	}
}

func historyShape(msgs []chat.Message) string {
	parts := make([]string, 0, len(msgs))
	for _, m := range msgs {
		switch msg := m.(type) {
		case *chat.AssistantMessage:
			if msg.HasToolCalls() {
				parts = append(parts, "assistant(tool_calls)")
			} else {
				parts = append(parts, "assistant(text)")
			}
		default:
			parts = append(parts, string(m.Type()))
		}
	}
	return strings.Join(parts, " -> ")
}

func continuationToolResult(t *testing.T, result *invocationResult) string {
	t.Helper()
	cont, err := result.buildContinueRequest()
	if err != nil {
		t.Fatalf("BuildContinueRequest: %v", err)
	}
	var b strings.Builder
	for _, m := range cont.Messages {
		if tm, ok := m.(*chat.ToolMessage); ok {
			for _, r := range tm.ToolReturns {
				b.WriteString(r.Result)
			}
		}
	}
	return b.String()
}

type abortErr struct{}

func (abortErr) Error() string { return "abort: fatal" }
func (abortErr) Abort() bool   { return true }

type interruptErr struct{}

func (interruptErr) Error() string { return "interrupt: awaiting approval" }
func (interruptErr) Abort() bool   { return false }

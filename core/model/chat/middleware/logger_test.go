package middleware_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"iter"
	"log/slog"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/core/model/chat/middleware"
)

// fakeHandler is a minimal CallHandler + StreamHandler the tests use
// to drive the middleware deterministically.
type fakeHandler struct {
	callResponse *chat.Response
	callErr      error
	streamChunks []*chat.Response
}

func (f *fakeHandler) Call(_ context.Context, _ *chat.Request) (*chat.Response, error) {
	return f.callResponse, f.callErr
}

func (f *fakeHandler) Stream(_ context.Context, _ *chat.Request) iter.Seq2[*chat.Response, error] {
	return func(yield func(*chat.Response, error) bool) {
		for _, c := range f.streamChunks {
			if !yield(c, nil) {
				return
			}
		}
	}
}

func newRequest(t *testing.T, model string, prompt string) *chat.Request {
	t.Helper()
	msgs := []chat.Message{}
	if prompt != "" {
		msgs = append(msgs, chat.NewUserMessage(prompt))
	}
	return &chat.Request{
		Messages: msgs,
		Options:  &chat.Options{Model: model},
	}
}

func newSuccessResponse(t *testing.T, body string) *chat.Response {
	t.Helper()
	res := &chat.Result{
		AssistantMessage: chat.NewAssistantMessage(body),
		Metadata:         &chat.ResultMetadata{FinishReason: chat.FinishReasonStop},
	}
	resp, err := chat.NewResponse(res, &chat.ResponseMetadata{})
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func TestLoggerMiddleware_Call_Success(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	callMW, _ := middleware.NewLoggerMiddleware(middleware.NewSlogLogger(logger))
	handler := callMW(&fakeHandler{callResponse: newSuccessResponse(t, "hello")})

	resp, err := handler.Call(context.Background(), newRequest(t, "gpt-x", "hi"))
	if err != nil {
		t.Fatalf("handler.Call: %v", err)
	}
	if resp == nil {
		t.Fatal("nil response")
	}

	lines := splitJSONLines(t, buf.Bytes())
	if len(lines) != 2 {
		t.Fatalf("want 2 log lines, got %d: %s", len(lines), buf.String())
	}
	if lines[0]["msg"] != "chat.request" {
		t.Errorf("first line msg=%v", lines[0]["msg"])
	}
	if lines[1]["msg"] != "chat.response" {
		t.Errorf("second line msg=%v", lines[1]["msg"])
	}
	if lines[1]["gen_ai.finish_reason"] != "stop" {
		t.Errorf("finish_reason missing: %v", lines[1])
	}
}

func TestLoggerMiddleware_Call_Error(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))

	callMW, _ := middleware.NewLoggerMiddleware(middleware.NewSlogLogger(logger))
	handler := callMW(&fakeHandler{callErr: errors.New("boom")})

	_, err := handler.Call(context.Background(), newRequest(t, "gpt-x", "hi"))
	if err == nil || err.Error() != "boom" {
		t.Fatalf("expected boom error, got %v", err)
	}

	lines := splitJSONLines(t, buf.Bytes())
	if len(lines) != 2 {
		t.Fatalf("want 2 log lines, got %d", len(lines))
	}
	if lines[1]["msg"] != "chat.error" {
		t.Errorf("expected error line, got %v", lines[1]["msg"])
	}
	if lines[1]["error.message"] != "boom" {
		t.Errorf("missing error.message: %v", lines[1])
	}
}

func TestLoggerMiddleware_NilLoggerIsSafe(t *testing.T) {
	callMW, _ := middleware.NewLoggerMiddleware(nil) // nopLogger fallback
	handler := callMW(&fakeHandler{callResponse: newSuccessResponse(t, "ok")})
	if _, err := handler.Call(context.Background(), newRequest(t, "m", "p")); err != nil {
		t.Fatal(err)
	}
}

func splitJSONLines(t *testing.T, b []byte) []map[string]any {
	t.Helper()
	var out []map[string]any
	for line := range strings.SplitSeq(strings.TrimSpace(string(b)), "\n") {
		if line == "" {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("unmarshal log line %q: %v", line, err)
		}
		out = append(out, m)
	}
	return out
}

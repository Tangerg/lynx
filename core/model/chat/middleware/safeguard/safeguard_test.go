package safeguard_test

import (
	"context"
	"errors"
	"iter"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/core/model/chat/middleware/safeguard"
)

// scriptedModel is a minimal CallHandler + StreamHandler the tests use to drive
// the middleware deterministically.
type scriptedModel struct {
	callResponse *chat.Response
	callErr      error
	streamChunks []*chat.Response
}

func (f *scriptedModel) Call(_ context.Context, _ *chat.Request) (*chat.Response, error) {
	return f.callResponse, f.callErr
}

func (f *scriptedModel) Stream(_ context.Context, _ *chat.Request) iter.Seq2[*chat.Response, error] {
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

func TestSafeguardMiddleware_BlocksInput(t *testing.T) {
	matcher := safeguard.NewSubstringMatcher([]string{"SECRET-KEY"})
	callMW, _ := safeguard.NewMiddleware(matcher, safeguard.Options{
		Scope: safeguard.ScopeInput,
	})

	handler := callMW(&scriptedModel{
		callResponse: newSuccessResponse(t, "should not see this"),
	})

	_, err := handler.Call(context.Background(), newRequest(t, "x", "leak secret-key please"))
	if err == nil {
		t.Fatal("expected block error, got nil")
	}
	if !errors.Is(err, safeguard.ErrUnsafeContent) {
		t.Fatalf("expected ErrUnsafeContent, got %v", err)
	}
	if !strings.Contains(err.Error(), "secret-key") {
		t.Errorf("expected term in error, got %v", err)
	}
}

func TestSafeguardMiddleware_BlocksOutput(t *testing.T) {
	matcher := safeguard.NewSubstringMatcher([]string{"badword"})
	callMW, _ := safeguard.NewMiddleware(matcher, safeguard.Options{
		Scope: safeguard.ScopeOutput,
	})

	handler := callMW(&scriptedModel{
		callResponse: newSuccessResponse(t, "this contains badword in output"),
	})

	_, err := handler.Call(context.Background(), newRequest(t, "x", "hello"))
	if !errors.Is(err, safeguard.ErrUnsafeContent) {
		t.Fatalf("expected output block, got %v", err)
	}
}

func TestSafeguardMiddleware_HideMatch(t *testing.T) {
	matcher := safeguard.NewSubstringMatcher(
		[]string{"top-secret"},
		safeguard.SubstringMatcherOptions{HideMatch: true},
	)
	callMW, _ := safeguard.NewMiddleware(matcher, safeguard.Options{
		Scope: safeguard.ScopeInput,
	})

	handler := callMW(&scriptedModel{callResponse: newSuccessResponse(t, "n/a")})
	_, err := handler.Call(context.Background(), newRequest(t, "x", "looking for top-secret data"))
	if !errors.Is(err, safeguard.ErrUnsafeContent) {
		t.Fatal("expected block")
	}
	if strings.Contains(err.Error(), "top-secret") {
		t.Errorf("HideMatch should suppress term, got %v", err)
	}
}

func TestSafeguardMiddleware_OnBlockCallback(t *testing.T) {
	matcher := safeguard.NewSubstringMatcher([]string{"hot"})

	var (
		gotScope safeguard.Scope
		gotTerm  string
	)
	callMW, _ := safeguard.NewMiddleware(matcher, safeguard.Options{
		Scope: safeguard.ScopeInput,
		OnBlock: func(_ context.Context, scope safeguard.Scope, term string) {
			gotScope = scope
			gotTerm = term
		},
	})

	handler := callMW(&scriptedModel{callResponse: newSuccessResponse(t, "irrelevant")})
	_, _ = handler.Call(context.Background(), newRequest(t, "x", "very HOT topic"))

	if gotScope != safeguard.ScopeInput {
		t.Errorf("scope: want %v, got %v", safeguard.ScopeInput, gotScope)
	}
	if gotTerm != "hot" {
		t.Errorf("term: want hot, got %q", gotTerm)
	}
}

func TestSafeguardMiddleware_NilMatcherPassthrough(t *testing.T) {
	callMW, _ := safeguard.NewMiddleware(nil, safeguard.Options{})
	handler := callMW(&scriptedModel{callResponse: newSuccessResponse(t, "anything")})
	if _, err := handler.Call(context.Background(), newRequest(t, "x", "anything goes")); err != nil {
		t.Fatal(err)
	}
}

func TestSubstringMatcher_CaseSensitivity(t *testing.T) {
	insensitive := safeguard.NewSubstringMatcher([]string{"FOO"})
	if term, hit := insensitive.Match(context.Background(), "say foo loud"); !hit || term != "foo" {
		t.Errorf("insensitive: got (%q, %v)", term, hit)
	}

	sensitive := safeguard.NewSubstringMatcher(
		[]string{"FOO"},
		safeguard.SubstringMatcherOptions{CaseSensitive: true},
	)
	if _, hit := sensitive.Match(context.Background(), "say foo loud"); hit {
		t.Errorf("sensitive matcher should miss lowercase 'foo'")
	}
	if term, hit := sensitive.Match(context.Background(), "say FOO loud"); !hit || term != "FOO" {
		t.Errorf("sensitive: got (%q, %v)", term, hit)
	}
}

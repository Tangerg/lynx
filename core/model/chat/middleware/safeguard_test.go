package middleware_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/core/model/chat/middleware"
)

func TestSafeguardMiddleware_BlocksInput(t *testing.T) {
	matcher := middleware.NewSubstringMatcher([]string{"SECRET-KEY"})
	callMW, _ := middleware.NewSafeguardMiddleware(matcher, middleware.SafeguardOptions{
		Scope: middleware.ScopeInput,
	})

	handler := callMW(&fakeHandler{
		callResponse: newSuccessResponse(t, "should not see this"),
	})

	_, err := handler.Call(context.Background(), newRequest(t, "x", "leak secret-key please"))
	if err == nil {
		t.Fatal("expected block error, got nil")
	}
	if !errors.Is(err, middleware.ErrUnsafeContent) {
		t.Fatalf("expected ErrUnsafeContent, got %v", err)
	}
	if !strings.Contains(err.Error(), "secret-key") {
		t.Errorf("expected term in error, got %v", err)
	}
}

func TestSafeguardMiddleware_BlocksOutput(t *testing.T) {
	matcher := middleware.NewSubstringMatcher([]string{"badword"})
	callMW, _ := middleware.NewSafeguardMiddleware(matcher, middleware.SafeguardOptions{
		Scope: middleware.ScopeOutput,
	})

	handler := callMW(&fakeHandler{
		callResponse: newSuccessResponse(t, "this contains badword in output"),
	})

	_, err := handler.Call(context.Background(), newRequest(t, "x", "hello"))
	if !errors.Is(err, middleware.ErrUnsafeContent) {
		t.Fatalf("expected output block, got %v", err)
	}
}

func TestSafeguardMiddleware_HideMatch(t *testing.T) {
	matcher := middleware.NewSubstringMatcher(
		[]string{"top-secret"},
		middleware.SubstringMatcherOptions{HideMatch: true},
	)
	callMW, _ := middleware.NewSafeguardMiddleware(matcher, middleware.SafeguardOptions{
		Scope: middleware.ScopeInput,
	})

	handler := callMW(&fakeHandler{callResponse: newSuccessResponse(t, "n/a")})
	_, err := handler.Call(context.Background(), newRequest(t, "x", "looking for top-secret data"))
	if !errors.Is(err, middleware.ErrUnsafeContent) {
		t.Fatal("expected block")
	}
	if strings.Contains(err.Error(), "top-secret") {
		t.Errorf("HideMatch should suppress term, got %v", err)
	}
}

func TestSafeguardMiddleware_OnBlockCallback(t *testing.T) {
	matcher := middleware.NewSubstringMatcher([]string{"hot"})

	var (
		gotScope middleware.SafeguardScope
		gotTerm  string
	)
	callMW, _ := middleware.NewSafeguardMiddleware(matcher, middleware.SafeguardOptions{
		Scope: middleware.ScopeInput,
		OnBlock: func(_ context.Context, scope middleware.SafeguardScope, term string) {
			gotScope = scope
			gotTerm = term
		},
	})

	handler := callMW(&fakeHandler{callResponse: newSuccessResponse(t, "irrelevant")})
	_, _ = handler.Call(context.Background(), newRequest(t, "x", "very HOT topic"))

	if gotScope != middleware.ScopeInput {
		t.Errorf("scope: want %v, got %v", middleware.ScopeInput, gotScope)
	}
	if gotTerm != "hot" {
		t.Errorf("term: want hot, got %q", gotTerm)
	}
}

func TestSafeguardMiddleware_NilMatcherPassthrough(t *testing.T) {
	callMW, _ := middleware.NewSafeguardMiddleware(nil, middleware.SafeguardOptions{})
	handler := callMW(&fakeHandler{callResponse: newSuccessResponse(t, "anything")})
	if _, err := handler.Call(context.Background(), newRequest(t, "x", "anything goes")); err != nil {
		t.Fatal(err)
	}
}

func TestSubstringMatcher_CaseSensitivity(t *testing.T) {
	insensitive := middleware.NewSubstringMatcher([]string{"FOO"})
	if term, hit := insensitive.Match(context.Background(), "say foo loud"); !hit || term != "foo" {
		t.Errorf("insensitive: got (%q, %v)", term, hit)
	}

	sensitive := middleware.NewSubstringMatcher(
		[]string{"FOO"},
		middleware.SubstringMatcherOptions{CaseSensitive: true},
	)
	if _, hit := sensitive.Match(context.Background(), "say foo loud"); hit {
		t.Errorf("sensitive matcher should miss lowercase 'foo'")
	}
	if term, hit := sensitive.Match(context.Background(), "say FOO loud"); !hit || term != "FOO" {
		t.Errorf("sensitive: got (%q, %v)", term, hit)
	}
}

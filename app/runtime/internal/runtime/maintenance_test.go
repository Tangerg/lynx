package runtime

import (
	"context"
	"errors"
	"testing"
)

func TestRuntimeGenerateTitleUsesTitlePort(t *testing.T) {
	titles := &fakeTitleGenerator{title: "Build Runtime Ports"}
	rt := &Runtime{titles: titles}

	got, err := rt.GenerateTitle(context.Background(), "please clean up runtime dependencies")
	if err != nil {
		t.Fatalf("GenerateTitle err = %v", err)
	}
	if got != "Build Runtime Ports" {
		t.Fatalf("title = %q", got)
	}
	if titles.firstMessage != "please clean up runtime dependencies" {
		t.Fatalf("first message = %q", titles.firstMessage)
	}
}

func TestRuntimeGenerateTitleReturnsEmptyWhenUnconfigured(t *testing.T) {
	got, err := (&Runtime{}).GenerateTitle(context.Background(), "hello")
	if err != nil {
		t.Fatalf("GenerateTitle err = %v", err)
	}
	if got != "" {
		t.Fatalf("title = %q, want empty", got)
	}
}

type fakeTitleGenerator struct {
	firstMessage string
	title        string
	err          error
}

func (f *fakeTitleGenerator) Generate(_ context.Context, firstMessage string) (string, error) {
	f.firstMessage = firstMessage
	if f.err != nil {
		return "", f.err
	}
	return f.title, nil
}

func TestRuntimeGenerateTitleReturnsPortError(t *testing.T) {
	fail := errors.New("title failed")
	rt := &Runtime{titles: &fakeTitleGenerator{err: fail}}

	if _, err := rt.GenerateTitle(context.Background(), "hello"); !errors.Is(err, fail) {
		t.Fatalf("GenerateTitle err = %v, want %v", err, fail)
	}
}

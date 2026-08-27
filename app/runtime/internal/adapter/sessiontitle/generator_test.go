package sessiontitle

import (
	"context"
	"errors"
	"iter"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/chatclient"
)

type failingTitleModel struct{ err error }

func (f failingTitleModel) Call(context.Context, *chat.Request) (*chat.Response, error) {
	return nil, f.err
}

func (f failingTitleModel) Stream(context.Context, *chat.Request) iter.Seq2[*chat.Response, error] {
	return func(yield func(*chat.Response, error) bool) { yield(nil, f.err) }
}

func TestSanitizeTitle(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"plain", "Fix the login bug", "Fix the login bug"},
		{"surrounding double quotes", `"Refactor Auth Module"`, "Refactor Auth Module"},
		{"backticks", "`Wire OAuth Flow`", "Wire OAuth Flow"},
		{"trailing period", "Add Dark Mode.", "Add Dark Mode"},
		{"trailing CJK punctuation", "修复会话恢复反馈。", "修复会话恢复反馈"},
		{"first line only", "Improve Streaming\n(notes about other stuff)", "Improve Streaming"},
		{"surrounding whitespace", "  Tidy Imports  ", "Tidy Imports"},
		{"blank", "   \n  ", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := sanitizeTitle(c.in); got != c.want {
				t.Fatalf("sanitizeTitle(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestSanitizeTitleCapsRunes(t *testing.T) {
	got := sanitizeTitle(strings.Repeat("word ", 40)) // ~200 chars, > titleMaxRunes
	if n := utf8.RuneCountInString(got); n > titleMaxRunes {
		t.Fatalf("title not capped to %d runes: got %d (%q)", titleMaxRunes, n, got)
	}
}

func TestGenerateReturnsOpeningMessageFallbackWhenProviderFails(t *testing.T) {
	providerErr := errors.New("provider unavailable")
	client, err := chatclient.New(failingTitleModel{err: providerErr}, chatclient.Config{})
	if err != nil {
		t.Fatal(err)
	}
	generator := NewGenerator(func(context.Context) *chatclient.Client { return client })

	got, err := generator.Generate(t.Context(), "  Diagnose provider outage  \ninclude the request log")
	if !errors.Is(err, providerErr) {
		t.Fatalf("Generate error = %v, want provider failure", err)
	}
	if got != "Diagnose provider outage" {
		t.Fatalf("Generate title = %q, want deterministic opening-message fallback", got)
	}
}

func TestGenerateUsesOpeningMessageFallbackWithoutUtilityClient(t *testing.T) {
	got, err := NewGenerator(nil).Generate(t.Context(), "  修复会话恢复反馈。\n附带诊断日志")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if got != "修复会话恢复反馈" {
		t.Fatalf("Generate title = %q, want first opening-message line", got)
	}
}

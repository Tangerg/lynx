package agentexec

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/agentmemory"
)

func TestPinnedMemoryPromptOrdersPinnedThenRecent(t *testing.T) {
	base := time.Date(2026, 7, 19, 0, 0, 0, 0, time.UTC)
	items := []agentmemory.Item{
		{ID: "old", Content: "- old unpinned", UpdatedAt: base},
		{ID: "pinned", Content: "- pinned note", Pinned: true, UpdatedAt: base.Add(-time.Hour)},
		{ID: "fresh", Content: "- fresh unpinned", UpdatedAt: base.Add(time.Hour)},
	}
	got := strings.Split(newPinnedMemoryPrompt(items, 0).text, "\n")
	want := []string{"- pinned note", "- fresh unpinned", "- old unpinned"}
	if !slices.Equal(got, want) {
		t.Fatalf("rendered memory = %#v, want %#v", got, want)
	}
}

func TestPinnedMemoryPromptHonorsBudget(t *testing.T) {
	items := []agentmemory.Item{
		{ID: "pinned", Content: "- pinned", Pinned: true},
		{ID: "omitted", Content: strings.Repeat("界", 40)},
	}
	prompt := newPinnedMemoryPrompt(items, 5)
	if prompt.text != "- pinned" || len(prompt.sources) != 1 || prompt.sources[0].Reference != "pinned" {
		t.Fatalf("budgeted memory = %+v, want only pinned item", prompt)
	}
	if newPinnedMemoryPrompt(nil, 10).text != "" {
		t.Fatal("empty memory must render nothing")
	}
	if got := estimateMemoryPromptTokens(strings.Repeat("界", 100)); got != 100 {
		t.Fatalf("CJK estimate = %d, want 100", got)
	}
}

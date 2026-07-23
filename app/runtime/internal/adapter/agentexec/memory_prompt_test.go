package agentexec

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/agentmemory"
)

func TestRenderPinnedMemoryOrdersPinnedThenRecent(t *testing.T) {
	base := time.Date(2026, 7, 19, 0, 0, 0, 0, time.UTC)
	items := []agentmemory.Item{
		{Content: "- old unpinned", UpdatedAt: base},
		{Content: "- pinned note", Pinned: true, UpdatedAt: base.Add(-time.Hour)},
		{Content: "- fresh unpinned", UpdatedAt: base.Add(time.Hour)},
	}
	got := strings.Split(renderPinnedMemory(items, 0), "\n")
	want := []string{"- pinned note", "- fresh unpinned", "- old unpinned"}
	if !slices.Equal(got, want) {
		t.Fatalf("rendered memory = %#v, want %#v", got, want)
	}
}

func TestRenderPinnedMemoryHonorsBudget(t *testing.T) {
	items := []agentmemory.Item{{Content: "- pinned", Pinned: true}, {Content: strings.Repeat("界", 40)}}
	if got := renderPinnedMemory(items, 5); got != "- pinned" {
		t.Fatalf("budgeted memory = %q, want pinned item", got)
	}
	if renderPinnedMemory(nil, 10) != "" {
		t.Fatal("empty memory must render nothing")
	}
	if got := estimateMemoryPromptTokens(strings.Repeat("界", 100)); got != 100 {
		t.Fatalf("CJK estimate = %d, want 100", got)
	}
}

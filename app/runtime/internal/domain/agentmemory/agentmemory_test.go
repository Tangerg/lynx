package agentmemory

import (
	"slices"
	"strings"
	"testing"
	"time"
)

func TestNormalizeFactsCanonicalizesAndDeduplicates(t *testing.T) {
	got := NormalizeFacts("```\n- keep this\n* keep this\n1. numbered\nplain\nNO_FACTS\n```")
	want := []string{"- keep this", "- numbered", "- plain"}
	if !slices.Equal(got, want) {
		t.Fatalf("NormalizeFacts = %#v, want %#v", got, want)
	}
}

func TestFactBatchNormalizeValidatesIdentity(t *testing.T) {
	batch := FactBatch{
		Project:    " /repo ",
		SessionID:  " session ",
		Day:        "2026-07-19",
		Facts:      []string{"- one", "one", "- two"},
		CapturedAt: time.Now(),
	}
	normalized, err := batch.Normalize()
	if err != nil {
		t.Fatal(err)
	}
	if normalized.Project != "/repo" || normalized.SessionID != "session" || !slices.Equal(normalized.Facts, []string{"- one", "- two"}) {
		t.Fatalf("normalized batch = %+v", normalized)
	}
	batch.Day = "2026-7-19"
	if _, err := batch.Normalize(); err == nil {
		t.Fatal("non-canonical day was accepted")
	}
}

func TestScopeAndOriginRoundTrip(t *testing.T) {
	for _, scope := range []Scope{ScopeProject, ScopeUser} {
		if ParseScope(scope.String()) != scope {
			t.Fatalf("scope round-trip failed for %v", scope)
		}
	}
	for _, origin := range []Origin{OriginAuto, OriginUser} {
		if ParseOrigin(origin.String()) != origin {
			t.Fatalf("origin round-trip failed for %v", origin)
		}
	}
	if ParseScope("garbage") != ScopeProject || ParseOrigin("garbage") != OriginAuto {
		t.Fatal("unknown tokens must default to project / auto")
	}
}

func TestRenderPinnedFirstThenRecency(t *testing.T) {
	base := time.Date(2026, 7, 19, 0, 0, 0, 0, time.UTC)
	items := []Item{
		{Content: "- old unpinned", UpdatedAt: base},
		{Content: "- pinned note", Pinned: true, UpdatedAt: base.Add(-time.Hour)},
		{Content: "- fresh unpinned", UpdatedAt: base.Add(time.Hour)},
	}
	got := Render(items, 0)
	lines := strings.Split(got, "\n")
	want := []string{"- pinned note", "- fresh unpinned", "- old unpinned"}
	if !slices.Equal(lines, want) {
		t.Fatalf("Render order = %#v, want %#v", lines, want)
	}
}

func TestRenderHonorsTokenBudget(t *testing.T) {
	items := []Item{
		{Content: "- pinned", Pinned: true},
		{Content: strings.Repeat("界", 40)}, // ~40 tokens, over budget
	}
	got := Render(items, 5)
	if got != "- pinned" {
		t.Fatalf("Render over budget = %q, want just the pinned item", got)
	}
	if Render(nil, 10) != "" {
		t.Fatal("empty items must render nothing")
	}
}

func TestEstimateTokens(t *testing.T) {
	if got := EstimateTokens(strings.Repeat("界", 100)); got != 100 {
		t.Fatalf("CJK estimate = %d, want 100", got)
	}
	if got := EstimateTokens("abcd"); got != 1 {
		t.Fatalf("ascii estimate = %d, want 1", got)
	}
}

package agentmemory

import (
	"fmt"
	"math"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestFactBatchNormalizeValidatesIdentity(t *testing.T) {
	batch := FactBatch{
		Project:    " /repo ",
		SessionID:  " session ",
		Day:        "2026-07-19",
		Facts:      []string{"one", "one", "two", " "},
		CapturedAt: time.Now(),
	}
	normalized, err := batch.Normalize()
	if err != nil {
		t.Fatal(err)
	}
	if normalized.Project != "/repo" || normalized.SessionID != "session" || !slices.Equal(normalized.Facts, []string{"one", "two"}) {
		t.Fatalf("normalized batch = %+v", normalized)
	}
	batch.Day = "2026-7-19"
	if _, err := batch.Normalize(); err == nil {
		t.Fatal("non-canonical day was accepted")
	}
}

func TestFactBatchNormalizeRejectsUnboundedExtractionCardinality(t *testing.T) {
	facts := make([]string, 33)
	for index := range facts {
		facts[index] = fmt.Sprintf("fact %d", index)
	}
	batch := FactBatch{
		Project: "/repo", SessionID: "session", Day: "2026-08-24",
		Facts: facts, CapturedAt: time.Now(),
	}
	if _, err := batch.Normalize(); err == nil {
		t.Fatal("fact batch with 33 distinct facts was accepted")
	}
}

func TestClosedVocabularyRoundTrip(t *testing.T) {
	for _, scope := range []Scope{ScopeProject, ScopeUser} {
		parsed, err := ParseScope(scope.String())
		if err != nil || parsed != scope {
			t.Fatalf("scope round-trip failed for %v", scope)
		}
	}
	for _, status := range []Status{StatusActive, StatusPending, StatusRejected} {
		parsed, err := ParseStatus(status.String())
		if err != nil || parsed != status {
			t.Fatalf("status round-trip failed for %v", status)
		}
	}
	for _, origin := range []Origin{OriginAuto, OriginUser} {
		parsed, err := ParseOrigin(origin.String())
		if err != nil || parsed != origin {
			t.Fatalf("origin round-trip failed for %v", origin)
		}
	}
	if _, err := ParseScope("garbage"); err == nil {
		t.Fatal("unknown scope was accepted")
	}
	if _, err := ParseStatus("garbage"); err == nil {
		t.Fatal("unknown status was accepted")
	}
	if _, err := ParseOrigin("garbage"); err == nil {
		t.Fatal("unknown origin was accepted")
	}
}

func TestReviewDecisionOwnsResultingStatus(t *testing.T) {
	for _, test := range []struct {
		decision ReviewDecision
		want     Status
	}{
		{decision: ReviewApprove, want: StatusActive},
		{decision: ReviewReject, want: StatusRejected},
	} {
		got, err := test.decision.Result()
		if err != nil || got != test.want {
			t.Fatalf("%q.Result() = (%q, %v), want %q", test.decision, got, err, test.want)
		}
	}
	if _, err := ReviewDecision("later").Result(); err == nil {
		t.Fatal("unknown review decision was accepted")
	}
}

func TestItemConstructionRejectsInvalidPartition(t *testing.T) {
	now := time.Now()
	if _, err := NewProposal("mem_1", "", "fact", now); err == nil {
		t.Fatal("project proposal without project was accepted")
	}
	if _, err := NewUserItem("mem_2", ScopeUser, "/repo", "fact", now); err == nil {
		t.Fatal("user item with project was accepted")
	}
}

func TestItemConstructionBoundsContentForModelContext(t *testing.T) {
	now := time.Now()
	if _, err := NewUserItem(
		"mem_boundary",
		ScopeUser,
		"",
		strings.Repeat("界", MaxContentCharacters),
		now,
	); err != nil {
		t.Fatalf("boundary content was rejected: %v", err)
	}
	if _, err := NewUserItem(
		"mem_oversized",
		ScopeUser,
		"",
		strings.Repeat("界", MaxContentCharacters+1),
		now,
	); err == nil {
		t.Fatal("content larger than one context-safe memory item was accepted")
	}
	if _, err := NewUserItem(
		"mem_invalid_utf8",
		ScopeUser,
		"",
		string([]byte{0xff}),
		now,
	); err == nil {
		t.Fatal("invalid UTF-8 content was accepted")
	}

	batch := FactBatch{
		Project: "/repo", SessionID: "session", Day: "2026-08-24",
		Facts: []string{strings.Repeat("界", MaxContentCharacters+1)}, CapturedAt: now,
	}
	if _, err := batch.Normalize(); err == nil {
		t.Fatal("oversized ledger fact was accepted")
	}
}

func TestEmbeddingUpdateBindsContentAndDefensivelyCopiesVector(t *testing.T) {
	item := Item{ID: "mem_1", Content: "current content"}
	vector := []float32{1, 2}
	update, err := NewEmbeddingUpdate(item, "provider:model", vector)
	if err != nil {
		t.Fatal(err)
	}
	vector[0] = 9
	if update.ItemID != item.ID || update.ContentDigest != Digest(item.Content) || update.Space != "provider:model" || !slices.Equal(update.Vector, []float32{1, 2}) {
		t.Fatalf("embedding update = %+v", update)
	}
	if err := update.Validate(); err != nil {
		t.Fatal(err)
	}
	if _, err := NewEmbeddingUpdate(item, "provider:model", []float32{float32(math.NaN())}); err == nil {
		t.Fatal("non-finite embedding vector was accepted")
	}
}

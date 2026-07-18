package knowledge

import (
	"slices"
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

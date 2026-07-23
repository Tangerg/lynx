package agentmemory

import (
	"slices"
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

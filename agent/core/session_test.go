package core_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/lynx/agent/core"
)

func TestNewSessionDefaultsTimestamps(t *testing.T) {
	before := time.Now()
	session := core.NewSession("s-1", "user-1", "demo-agent")
	after := time.Now()

	if session.ID != "s-1" || session.UserID != "user-1" || session.AgentName != "demo-agent" {
		t.Errorf("identity: %#v", session)
	}
	if session.StartedAt.Before(before) || session.StartedAt.After(after) {
		t.Errorf("StartedAt: outside [%v, %v]: %v", before, after, session.StartedAt)
	}
	if !session.StartedAt.Equal(session.UpdatedAt) {
		t.Errorf("StartedAt should equal UpdatedAt at creation; got %v vs %v",
			session.StartedAt, session.UpdatedAt)
	}
	if session.Metadata == nil {
		t.Error("Metadata should be allocated, not nil")
	}
}

func TestSessionTouchRefreshesUpdatedAt(t *testing.T) {
	session := core.NewSession("s-1", "u", "demo")
	before := time.Unix(1, 0).UTC()
	session.UpdatedAt = before
	session.Touch()
	if !session.UpdatedAt.After(before) {
		t.Errorf("Touch should advance UpdatedAt: before=%v after=%v", before, session.UpdatedAt)
	}
}

func TestSessionBindAgent(t *testing.T) {
	session := core.NewSession("s-1", "user-1", "")
	if err := session.BindAgent("demo"); err != nil {
		t.Fatalf("BindAgent: %v", err)
	}
	if session.AgentName != "demo" {
		t.Fatalf("AgentName = %q, want demo", session.AgentName)
	}
	if err := session.BindAgent("demo"); err != nil {
		t.Fatalf("BindAgent idempotent call: %v", err)
	}
	if err := session.BindAgent("other"); !errors.Is(err, core.ErrInvalidSession) {
		t.Fatalf("BindAgent conflict = %v, want ErrInvalidSession", err)
	}
}

func TestSessionValidate(t *testing.T) {
	valid := core.NewSession("s-1", "user-1", "demo")
	tests := []struct {
		name   string
		mutate func(*core.Session)
	}{
		{name: "empty ID", mutate: func(s *core.Session) { s.ID = "" }},
		{name: "padded ID", mutate: func(s *core.Session) { s.ID = " s-1" }},
		{name: "self parent", mutate: func(s *core.Session) { s.ParentID = s.ID }},
		{name: "padded parent", mutate: func(s *core.Session) { s.ParentID = " parent " }},
		{name: "padded user", mutate: func(s *core.Session) { s.UserID = " user " }},
		{name: "empty agent", mutate: func(s *core.Session) { s.AgentName = "" }},
		{name: "zero start", mutate: func(s *core.Session) { s.StartedAt = time.Time{} }},
		{name: "zero update", mutate: func(s *core.Session) { s.UpdatedAt = time.Time{} }},
		{name: "update before start", mutate: func(s *core.Session) { s.UpdatedAt = s.StartedAt.Add(-time.Second) }},
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid session: %v", err)
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			session := valid
			test.mutate(&session)
			if err := session.Validate(); !errors.Is(err, core.ErrInvalidSession) {
				t.Fatalf("Validate() error = %v, want ErrInvalidSession", err)
			}
		})
	}
}

func TestMemorySessionStoreSaveLoad(t *testing.T) {
	store := core.NewMemorySessionStore()
	ctx := context.Background()

	session := core.NewSession("s-1", "user-1", "demo")
	session.Metadata["channel"] = "web"

	if err := store.Save(ctx, session); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, err := store.Load(ctx, "s-1")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got.ID != "s-1" || got.Metadata["channel"] != "web" {
		t.Errorf("round-trip: %#v", got)
	}
}

func TestMemorySessionStoreOwnsNestedMetadataSnapshots(t *testing.T) {
	store := core.NewMemorySessionStore()
	session := core.NewSession("s-1", "user-1", "demo")
	nested := map[string]any{"labels": []any{"saved"}}
	session.Metadata["nested"] = nested

	if err := store.Save(t.Context(), session); err != nil {
		t.Fatalf("Save: %v", err)
	}
	nested["labels"].([]any)[0] = "caller-mutated"
	session.Metadata["new"] = true

	first, err := store.Load(t.Context(), session.ID)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	firstNested := first.Metadata["nested"].(map[string]any)
	if got := firstNested["labels"].([]any)[0]; got != "saved" {
		t.Fatalf("stored nested metadata = %v, want saved", got)
	}
	if _, leaked := first.Metadata["new"]; leaked {
		t.Fatal("stored metadata retained caller map mutation")
	}

	firstNested["labels"].([]any)[0] = "load-mutated"
	second, err := store.Load(t.Context(), session.ID)
	if err != nil {
		t.Fatalf("second Load: %v", err)
	}
	secondNested := second.Metadata["nested"].(map[string]any)
	if got := secondNested["labels"].([]any)[0]; got != "saved" {
		t.Fatalf("load result aliased stored metadata: got %v", got)
	}
}

func TestMemorySessionStoreRejectsNonJSONMetadata(t *testing.T) {
	store := core.NewMemorySessionStore()
	session := core.NewSession("s-1", "user-1", "demo")
	session.Metadata["callback"] = func() {}

	if err := store.Save(t.Context(), session); err == nil || !strings.Contains(err.Error(), "session metadata") {
		t.Fatalf("Save error = %v, want metadata encoding error", err)
	}
}

func TestMemorySessionStoreSeparatesConcurrentCallerMutation(t *testing.T) {
	store := core.NewMemorySessionStore()
	session := core.NewSession("s-1", "user-1", "demo")
	labels := []any{"stored"}
	session.Metadata["labels"] = labels
	if err := store.Save(t.Context(), session); err != nil {
		t.Fatalf("Save: %v", err)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		for index := range 1_000 {
			labels[0] = index
		}
	}()
	for range 1_000 {
		loaded, err := store.Load(t.Context(), session.ID)
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if got := loaded.Metadata["labels"].([]any)[0]; got != "stored" {
			t.Fatalf("stored metadata changed with caller mutation: %v", got)
		}
	}
	<-done
}

func TestMemorySessionStoreNotFound(t *testing.T) {
	store := core.NewMemorySessionStore()
	_, err := store.Load(context.Background(), "ghost")
	if !errors.Is(err, core.ErrSessionNotFound) {
		t.Errorf("want ErrSessionNotFound, got %v", err)
	}
}

func TestMemorySessionStoreDeleteIdempotent(t *testing.T) {
	store := core.NewMemorySessionStore()
	ctx := context.Background()

	_ = store.Save(ctx, core.NewSession("s-1", "u", "x"))
	if err := store.Delete(ctx, "s-1"); err != nil {
		t.Fatal(err)
	}
	if err := store.Delete(ctx, "s-1"); err != nil {
		t.Errorf("redelete: %v", err)
	}
	if err := store.Delete(ctx, "never-existed"); err != nil {
		t.Errorf("delete unknown: %v", err)
	}
}

func TestMemorySessionStoreRejectsEmptyID(t *testing.T) {
	if err := core.NewMemorySessionStore().Save(t.Context(), core.Session{}); !errors.Is(err, core.ErrInvalidSession) {
		t.Fatalf("Save error = %v, want ErrInvalidSession", err)
	}
}

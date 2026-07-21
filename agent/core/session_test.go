package core_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/storetest"
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
	if !session.Metadata.IsZero() {
		t.Errorf("Metadata = %#v, want empty", session.Metadata)
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

func TestSessionCloneOwnsMetadata(t *testing.T) {
	session := core.NewSession("s-1", "user-1", "demo")
	setSessionMetadata(t, &session, "channel", "desktop")
	clone := session.Clone()
	if err := clone.Metadata.Set("channel", "web"); err != nil {
		t.Fatal(err)
	}
	if got := decodeSessionMetadata[string](t, session.Metadata, "channel"); got != "desktop" {
		t.Fatalf("source metadata = %q, want desktop", got)
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
	store := storetest.NewMemorySessionStore()
	ctx := context.Background()

	session := core.NewSession("s-1", "user-1", "demo")
	setSessionMetadata(t, &session, "channel", "web")

	if err := store.Save(ctx, session); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, err := store.Load(ctx, "s-1")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got.ID != "s-1" || decodeSessionMetadata[string](t, got.Metadata, "channel") != "web" {
		t.Errorf("round-trip: %#v", got)
	}
}

func TestMemorySessionStoreOwnsNestedMetadataSnapshots(t *testing.T) {
	store := storetest.NewMemorySessionStore()
	session := core.NewSession("s-1", "user-1", "demo")
	nested := map[string]any{"labels": []any{"saved"}}
	setSessionMetadata(t, &session, "nested", nested)

	if err := store.Save(t.Context(), session); err != nil {
		t.Fatalf("Save: %v", err)
	}
	nested["labels"].([]any)[0] = "caller-mutated"
	setSessionMetadata(t, &session, "new", true)

	first, err := store.Load(t.Context(), session.ID)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	firstNested := decodeSessionMetadata[map[string]any](t, first.Metadata, "nested")
	if got := firstNested["labels"].([]any)[0]; got != "saved" {
		t.Fatalf("stored nested metadata = %v, want saved", got)
	}
	var added bool
	if leaked, err := first.Metadata.Decode("new", &added); err != nil || leaked {
		t.Fatal("stored metadata retained caller map mutation")
	}

	firstNested["labels"].([]any)[0] = "load-mutated"
	second, err := store.Load(t.Context(), session.ID)
	if err != nil {
		t.Fatalf("second Load: %v", err)
	}
	secondNested := decodeSessionMetadata[map[string]any](t, second.Metadata, "nested")
	if got := secondNested["labels"].([]any)[0]; got != "saved" {
		t.Fatalf("load result aliased stored metadata: got %v", got)
	}
}

func TestSessionMetadataRejectsNonJSONValueAtWriteBoundary(t *testing.T) {
	var metadata core.SessionMetadata
	if err := metadata.Set("callback", func() {}); !errors.Is(err, core.ErrInvalidSessionMetadata) {
		t.Fatalf("Set error = %v, want ErrInvalidSessionMetadata", err)
	}
}

func TestSessionMetadataOwnsCanonicalJSONObject(t *testing.T) {
	source := []byte(`{"z":[1,true],"a":{"name":"agent"}}`)
	metadata, err := core.ParseSessionMetadata(source)
	if err != nil {
		t.Fatalf("ParseSessionMetadata: %v", err)
	}
	source[2] = 'x'

	encoded, err := json.Marshal(metadata)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if got, want := string(encoded), `{"a":{"name":"agent"},"z":[1,true]}`; got != want {
		t.Fatalf("JSON = %s, want %s", got, want)
	}

	clone := metadata.Clone()
	if err := clone.Set("new", true); err != nil {
		t.Fatal(err)
	}
	var added bool
	if found, err := metadata.Decode("new", &added); err != nil || found {
		t.Fatalf("clone mutation leaked into source: found=%t err=%v", found, err)
	}
}

func TestSessionMetadataRejectsNonObjects(t *testing.T) {
	for _, input := range []string{"null", "[]", `"value"`, "42", "{"} {
		t.Run(input, func(t *testing.T) {
			if _, err := core.ParseSessionMetadata([]byte(input)); !errors.Is(err, core.ErrInvalidSessionMetadata) {
				t.Fatalf("ParseSessionMetadata(%q) error = %v, want ErrInvalidSessionMetadata", input, err)
			}
		})
	}
}

func TestMemorySessionStoreSeparatesConcurrentCallerMutation(t *testing.T) {
	store := storetest.NewMemorySessionStore()
	session := core.NewSession("s-1", "user-1", "demo")
	labels := []any{"stored"}
	setSessionMetadata(t, &session, "labels", labels)
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
		if got := decodeSessionMetadata[[]any](t, loaded.Metadata, "labels")[0]; got != "stored" {
			t.Fatalf("stored metadata changed with caller mutation: %v", got)
		}
	}
	<-done
}

func setSessionMetadata(t *testing.T, session *core.Session, name string, value any) {
	t.Helper()
	if err := session.Metadata.Set(name, value); err != nil {
		t.Fatalf("Metadata.Set(%q): %v", name, err)
	}
}

func decodeSessionMetadata[T any](t *testing.T, metadata core.SessionMetadata, name string) T {
	t.Helper()
	var value T
	found, err := metadata.Decode(name, &value)
	if err != nil {
		t.Fatalf("Metadata.Decode(%q): %v", name, err)
	}
	if !found {
		t.Fatalf("Metadata.Decode(%q): field not found", name)
	}
	return value
}

func TestMemorySessionStoreNotFound(t *testing.T) {
	store := storetest.NewMemorySessionStore()
	_, err := store.Load(context.Background(), "ghost")
	if !errors.Is(err, core.ErrSessionNotFound) {
		t.Errorf("want ErrSessionNotFound, got %v", err)
	}
}

func TestMemorySessionStoreDeleteIdempotent(t *testing.T) {
	store := storetest.NewMemorySessionStore()
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
	if err := storetest.NewMemorySessionStore().Save(t.Context(), core.Session{}); !errors.Is(err, core.ErrInvalidSession) {
		t.Fatalf("Save error = %v, want ErrInvalidSession", err)
	}
}

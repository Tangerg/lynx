package agent2

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"
)

func TestSnapshotStrictlyRejectsUnknownFields(t *testing.T) {
	snapshot := completedEngineTestSnapshot(t)
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(snapshot.JSON(), &fields); err != nil {
		t.Fatal(err)
	}
	fields["application_revision"] = json.RawMessage(`1`)
	data, err := json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseSnapshot(data); err == nil {
		t.Fatal("ParseSnapshot accepted an unknown application field")
	}
}

func TestSnapshotRejectsPriorSchemaVersion(t *testing.T) {
	snapshot := completedEngineTestSnapshot(t)
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(snapshot.JSON(), &fields); err != nil {
		t.Fatal(err)
	}
	fields["schema_version"] = json.RawMessage(`3`)
	data, err := json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseSnapshot(data); !errors.Is(err, ErrInvalidSnapshot) {
		t.Fatalf("prior schema error = %v, want ErrInvalidSnapshot", err)
	}
}

func FuzzSnapshotJSONRoundTrip(f *testing.F) {
	snapshot := completedEngineTestSnapshot(f)
	f.Add([]byte(snapshot.JSON()))
	f.Fuzz(func(t *testing.T, data []byte) {
		parsed, err := ParseSnapshot(data)
		if err != nil {
			return
		}
		encoded, err := json.Marshal(parsed)
		if err != nil {
			t.Fatal(err)
		}
		reparsed, err := ParseSnapshot(encoded)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(parsed.JSON(), reparsed.JSON()) {
			t.Fatal("Snapshot changed across a strict JSON round trip")
		}
	})
}

func completedEngineTestSnapshot(t testing.TB) Snapshot {
	t.Helper()
	definition := newEngineTestDefinition(t, "engine.effect", "effect")
	deployment := engineTestDeployment(t, definition, &engineTestDispatcher{policy: ReplayPolicyNever})
	engine, err := NewEngine(EngineConfig{})
	if err != nil {
		t.Fatal(err)
	}
	input, err := EncodeInput(engineTestInput{Value: "snapshot"})
	if err != nil {
		t.Fatal(err)
	}
	process, err := engine.Start(context.Background(), deployment, input)
	if err != nil {
		t.Fatal(err)
	}
	result, err := process.Await(context.Background())
	if err != nil || result.Status() != StatusCompleted {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	snapshot, err := process.Capture(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := engine.Close(); err != nil {
		t.Fatal(err)
	}
	return snapshot
}

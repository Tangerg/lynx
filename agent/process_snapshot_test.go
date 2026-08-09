package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"math"
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
	fields["schema_version"] = json.RawMessage(`5`)
	data, err := json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseSnapshot(data); !errors.Is(err, ErrInvalidSnapshot) {
		t.Fatalf("prior schema error = %v, want ErrInvalidSnapshot", err)
	}
}

func TestSnapshotRejectsAcceptedSignalCountThatDisagreesWithMailbox(t *testing.T) {
	snapshot := completedEngineTestSnapshot(t)
	wire, err := snapshot.wire()
	if err != nil {
		t.Fatal(err)
	}
	if wire.Usage.AcceptedSignals == 0 {
		t.Fatal("test fixture contains no accepted Signal")
	}
	wire.Usage.AcceptedSignals--
	data, err := json.Marshal(wire)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseSnapshot(data); !errors.Is(err, ErrInvalidSnapshot) {
		t.Fatalf("accepted Signal mismatch error = %v, want ErrInvalidSnapshot", err)
	}
}

func TestSnapshotRejectsPreparedStepSequenceOverflow(t *testing.T) {
	snapshot := preparedEngineTestSnapshot(t)
	wire, err := snapshot.wire()
	if err != nil {
		t.Fatal(err)
	}
	if wire.Prepared == nil || len(wire.Prepared.Effects) != 1 {
		t.Fatalf("prepared fixture = %#v", wire.Prepared)
	}
	wire.CommittedSteps = math.MaxUint64
	wire.Usage.CommittedSteps = math.MaxUint64
	wire.Limits.MaxSteps = math.MaxUint64
	wire.Budget.Steps = math.MaxUint64
	wire.Prepared.StepSequence = 0
	wire.Prepared.Effects[0].ID = deriveEffectID(wire.ProcessID, 0, 0)
	data, err := json.Marshal(wire)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseSnapshot(data); !errors.Is(err, ErrInvalidSnapshot) {
		t.Fatalf("prepared Step overflow error = %v, want ErrInvalidSnapshot", err)
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

func preparedEngineTestSnapshot(t testing.TB) Snapshot {
	t.Helper()
	acknowledger := &engineTestAcknowledger{}
	definition := newEngineTestDefinition(t, "engine.effect", "effect")
	deployment := engineTestDeployment(t, definition, &engineTestDispatcher{policy: ReplayPolicyNever})
	engine, err := NewEngine(EngineConfig{PreparedStepAcknowledger: acknowledger})
	if err != nil {
		t.Fatal(err)
	}
	input, err := EncodeInput(engineTestInput{Value: "prepared snapshot"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := engine.Run(context.Background(), deployment, input); err != nil {
		t.Fatal(err)
	}
	snapshot := acknowledger.captured()
	if err := engine.Close(); err != nil {
		t.Fatal(err)
	}
	return snapshot
}

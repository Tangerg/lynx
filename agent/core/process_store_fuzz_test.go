package core_test

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/Tangerg/lynx/agent/core"
)

func FuzzProcessSnapshotJSON(f *testing.F) {
	now := time.Unix(1_752_568_200, 123_000_000).UTC()
	valid, err := json.Marshal(core.ProcessSnapshot{
		SchemaVersion: core.ProcessSnapshotSchemaVersion,
		ID:            "process-1",
		Deployment:    core.DeploymentRef{Name: "researcher", Version: "1.0.0", Digest: "digest-1"},
		StartedAt:     now,
		CapturedAt:    now.Add(time.Second),
		Status:        core.StatusCompleted,
	})
	if err != nil {
		f.Fatal(err)
	}
	for _, seed := range [][]byte{valid, []byte(`{}`), []byte(`{"schema_version":999}`), []byte(`null`)} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		assertSnapshotJSONFixedPoint(t, data)
	})
}

func assertSnapshotJSONFixedPoint(t *testing.T, data []byte) {
	t.Helper()
	var first core.ProcessSnapshot
	if err := json.Unmarshal(data, &first); err != nil {
		return
	}
	if err := first.Validate(); err != nil {
		t.Fatalf("successful Unmarshal produced invalid ProcessSnapshot: %v", err)
	}
	firstWire, err := json.Marshal(first)
	if err != nil {
		t.Fatalf("Marshal after successful Unmarshal: %v", err)
	}
	var second core.ProcessSnapshot
	if err := json.Unmarshal(firstWire, &second); err != nil {
		t.Fatalf("Unmarshal canonical ProcessSnapshot: %v", err)
	}
	secondWire, err := json.Marshal(second)
	if err != nil {
		t.Fatalf("Marshal second ProcessSnapshot: %v", err)
	}
	if !bytes.Equal(firstWire, secondWire) {
		t.Fatalf("ProcessSnapshot wire did not reach fixed point: first=%s second=%s", firstWire, secondWire)
	}
}

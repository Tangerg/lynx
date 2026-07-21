package skillauthoring_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/infra/skillauthoring"
)

func TestRecordUseAccumulatesUsage(t *testing.T) {
	root := t.TempDir()
	store := skillauthoring.NewStore(root)
	base := time.Unix(1_000_000, 0)
	if err := store.RecordUse(t.Context(), "run-tests", base); err != nil {
		t.Fatalf("RecordUse: %v", err)
	}
	if err := store.RecordUse(t.Context(), "run-tests", base.Add(time.Hour)); err != nil {
		t.Fatalf("RecordUse: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(root, ".usage.json"))
	if err != nil {
		t.Fatalf("usage file not written: %v", err)
	}
	var usage map[string]map[string]any
	if err := json.Unmarshal(data, &usage); err != nil {
		t.Fatalf("usage file not valid JSON: %v", err)
	}
	record, ok := usage["run-tests"]
	if !ok {
		t.Fatalf("no usage record for run-tests: %s", data)
	}
	if last, _ := record["lastUsed"].(float64); int64(last) != base.Add(time.Hour).Unix() {
		t.Fatalf("lastUsed = %v, want %d", record["lastUsed"], base.Add(time.Hour).Unix())
	}
	if first, _ := record["firstSeen"].(float64); int64(first) != base.Unix() {
		t.Fatalf("firstSeen = %v, want %d (anchored at first use)", record["firstSeen"], base.Unix())
	}
}

func TestRecordUseDisabledStoreNoOps(t *testing.T) {
	store := skillauthoring.NewStore("")
	if err := store.RecordUse(t.Context(), "x", time.Unix(1, 0)); err != nil {
		t.Fatalf("disabled RecordUse: %v", err)
	}
}

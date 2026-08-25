package skillauthoring_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/skills"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/skillauthoring"
)

const (
	governedUsageBytes = 64 << 10
)

func writeActiveSkillFixture(t *testing.T, root, name string) {
	t.Helper()
	directory := filepath.Join(root, name)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatalf("create active skill %q: %v", name, err)
	}
	document := fmt.Sprintf("---\nname: %s\ndescription: A valid managed Skill used by the capacity counterexample.\n---\ninstructions", name)
	if err := os.WriteFile(filepath.Join(directory, "SKILL.md"), []byte(document), 0o644); err != nil {
		t.Fatalf("write active skill %q: %v", name, err)
	}
}

func TestRecordUseAccumulatesUsage(t *testing.T) {
	root := t.TempDir()
	store := skillauthoring.NewStore(root, skills.ScopeUser)
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
	store := skillauthoring.NewStore("", skills.ScopeUser)
	if err := store.RecordUse(t.Context(), "x", time.Unix(1, 0)); err != nil {
		t.Fatalf("disabled RecordUse: %v", err)
	}
}

func TestRecordUseRejectsOversizedUsageMetadata(t *testing.T) {
	root := t.TempDir()
	oversized := `{"` + strings.Repeat("x", governedUsageBytes) + `":{"firstSeen":1}}`
	if err := os.WriteFile(filepath.Join(root, ".usage.json"), []byte(oversized), 0o644); err != nil {
		t.Fatal(err)
	}

	store := skillauthoring.NewStore(root, skills.ScopeUser)
	if err := store.RecordUse(t.Context(), "run-tests", time.Unix(2, 0)); !errors.Is(err, skills.ErrUsageTooLarge) {
		t.Fatalf("RecordUse error = %v, want ErrUsageTooLarge beyond %d bytes", err, governedUsageBytes)
	}
}

func TestRecordUseRejectsOverCapacityUsageMap(t *testing.T) {
	root := t.TempDir()
	usage := make(map[string]map[string]int64, skills.MaxSkillsPerSource+1)
	for index := range skills.MaxSkillsPerSource + 1 {
		usage[fmt.Sprintf("skill-%03d", index)] = map[string]int64{"firstSeen": 1}
	}
	data, err := json.Marshal(usage)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".usage.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	store := skillauthoring.NewStore(root, skills.ScopeUser)
	if err := store.RecordUse(t.Context(), "run-tests", time.Unix(2, 0)); !errors.Is(err, skills.ErrLibraryCapacity) {
		t.Fatalf("RecordUse error = %v, want ErrLibraryCapacity beyond %d records", err, skills.MaxSkillsPerSource)
	}
}

package sqlite

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestContractBaselineTracksSchemaEpoch(t *testing.T) {
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate contract baseline test source")
	}
	baselinePath := filepath.Join(filepath.Dir(source), "..", "..", "..", "..", "doc", "CONTRACT_BASELINE.md")
	baseline, err := os.ReadFile(baselinePath)
	if err != nil {
		t.Fatalf("read contract baseline: %v", err)
	}
	want := []byte(fmt.Sprintf("`schemaEpoch = %d`", schemaEpoch))
	if !bytes.Contains(baseline, want) {
		t.Fatalf("contract baseline does not contain current schema epoch %q", want)
	}
}

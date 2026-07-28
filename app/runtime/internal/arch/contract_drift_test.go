package arch

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestGeneratedContractHasNoDrift is contract §11.4 gate 1: rerun the generator
// and the worktree must be unchanged.
//
// It is the only mechanism that notices when the code and the published contract
// stop agreeing. Every other check in this package guards a structural rule; this
// one guards a FACT — that the method surface, capability policy, error registry
// and shape specs a client reads are the ones the dispatcher actually implements.
// Without it, "generated" degrades into "generated once".
func TestGeneratedContractHasNoDrift(t *testing.T) {
	root := moduleRoot(t)
	manifest := filepath.Join(root, "contract", "manifest.json")

	committed, err := os.ReadFile(manifest)
	if err != nil {
		t.Fatalf("read the committed manifest: %v — run `go generate ./...`", err)
	}

	regenerated := t.TempDir()
	cmd := exec.Command("go", "run", "github.com/Tangerg/lynx/app/runtime/cmd/contractgen", "-out", regenerated)
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("run contractgen: %v\n%s", err, out)
	}
	fresh, err := os.ReadFile(filepath.Join(regenerated, "manifest.json"))
	if err != nil {
		t.Fatalf("read the regenerated manifest: %v", err)
	}

	if !bytes.Equal(committed, fresh) {
		t.Fatal("contract/manifest.json is stale — run `go generate ./...` and commit the result")
	}
}

// TestGeneratedContractIsSubstantive stops the drift gate from passing
// vacuously: an empty manifest would compare equal to an empty manifest forever.
func TestGeneratedContractIsSubstantive(t *testing.T) {
	root := moduleRoot(t)
	raw, err := os.ReadFile(filepath.Join(root, "contract", "manifest.json"))
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	var manifest struct {
		Protocol         map[string]string `json:"protocol"`
		Methods          []struct{}        `json:"methods"`
		StreamingMethods []string          `json:"streamingMethods"`
		Errors           struct {
			Codes map[string]int `json:"codes"`
		} `json:"errors"`
		CapabilityPolicy []struct{} `json:"capabilityPolicy"`
		RunEventPolicy   []struct{} `json:"runEventPolicy"`
		StatePolicy      []struct{} `json:"statePolicy"`
		Unions           []struct{} `json:"unions"`
		Constraints      []struct{} `json:"objectConstraints"`
		SystemInvariants []struct{} `json:"systemInvariants"`
	}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatalf("decode manifest: %v", err)
	}
	sections := map[string]int{
		"protocol":          len(manifest.Protocol),
		"methods":           len(manifest.Methods),
		"streamingMethods":  len(manifest.StreamingMethods),
		"errors.codes":      len(manifest.Errors.Codes),
		"capabilityPolicy":  len(manifest.CapabilityPolicy),
		"runEventPolicy":    len(manifest.RunEventPolicy),
		"statePolicy":       len(manifest.StatePolicy),
		"unions":            len(manifest.Unions),
		"objectConstraints": len(manifest.Constraints),
		"systemInvariants":  len(manifest.SystemInvariants),
	}
	for section, count := range sections {
		if count == 0 {
			t.Errorf("manifest section %q is empty; the drift gate would pass on nothing", section)
		}
	}
}

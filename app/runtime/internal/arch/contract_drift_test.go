package arch

import (
	"bytes"
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
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

	reference := filepath.Join(root, "contract", "API_REFERENCE.md")
	regenerated := t.TempDir()
	cmd := exec.Command("go", "run", "github.com/Tangerg/lynx/app/runtime/cmd/contractgen", "-out", regenerated)
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("run contractgen: %v\n%s", err, out)
	}
	for _, artifact := range []string{manifest, reference} {
		committed, err := os.ReadFile(artifact)
		if err != nil {
			t.Fatalf("read %s: %v — run `go generate ./...`", filepath.Base(artifact), err)
		}
		fresh, err := os.ReadFile(filepath.Join(regenerated, filepath.Base(artifact)))
		if err != nil {
			t.Fatalf("read the regenerated %s: %v", filepath.Base(artifact), err)
		}
		if !bytes.Equal(committed, fresh) {
			t.Errorf("contract/%s is stale — run `go generate ./...` and commit the result", filepath.Base(artifact))
		}
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

// TestRequestConstraintsStayPure is contract §11.4 gate 7: a DTO validator's
// dependency graph contains no store, dispatcher or executor.
//
// The whole reason a constraint may live on the request type is that checking it
// costs nothing and can never fail for an environmental reason. Give a Validate()
// a repository and two things break at once: "invalid_params" starts meaning
// "the database was slow", and the generated TS validator — which has no
// repository — stops being equivalent to the Go one.
func TestRequestConstraintsStayPure(t *testing.T) {
	root := moduleRoot(t)
	path := filepath.Join(root, "internal", "delivery", "protocol", "request_constraints.go")
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	for _, spec := range file.Imports {
		imported := strings.Trim(spec.Path.Value, `"`)
		if strings.Contains(imported, "/internal/") {
			t.Errorf("request constraints import %q; a shape constraint may only read the value it validates", imported)
		}
	}
	// Validate must also be reachable without a context: a check that needs one is
	// a check that does I/O.
	full, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	for _, decl := range full.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "Validate" || fn.Recv == nil {
			continue
		}
		if fn.Type.Params != nil && len(fn.Type.Params.List) != 0 {
			t.Errorf("%s.Validate takes parameters; a shape constraint reads only its own value", exprString(fn.Recv.List[0].Type))
		}
	}
}

// TestProtocolVersionAgreesEverywhere is contract §11.4 gate 12: the generated
// manifest, the canonical docs and the code state one protocol version.
//
// This is the drift A1 had to fix by hand, machine-enforced. C16 flips the version
// in ONE place — the constant — and this gate then names every document still
// claiming the old one, instead of a reader discovering the mismatch later from a
// client that negotiated against a stale header.
func TestProtocolVersionAgreesEverywhere(t *testing.T) {
	root := moduleRoot(t)

	var manifest struct {
		Protocol struct {
			Current      string `json:"current"`
			MinSupported string `json:"minSupported"`
		} `json:"protocol"`
	}
	raw, err := os.ReadFile(filepath.Join(root, "contract", "manifest.json"))
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatalf("decode manifest: %v", err)
	}
	if manifest.Protocol.Current != protocol.ProtocolVersion {
		t.Errorf("manifest protocol %q != code %q", manifest.Protocol.Current, protocol.ProtocolVersion)
	}
	if manifest.Protocol.MinSupported != protocol.MinProtocolVersion {
		t.Errorf("manifest minSupported %q != code %q", manifest.Protocol.MinSupported, protocol.MinProtocolVersion)
	}

	// The canonical docs each state the version in their own header. Any version
	// literal they carry must be one the code actually serves; a doc naming a
	// version the runtime would reject is worse than no doc, because a client
	// negotiates against it.
	dateLiteral := regexp.MustCompile(`\b20\d\d-\d\d-\d\d\b`)
	for _, name := range []string{"API.md", "AUX_API.md", "TRANSPORT.md"} {
		path := filepath.Join(root, "..", "desktop", "docs", "protocol", name)
		text, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		found := false
		for _, match := range dateLiteral.FindAllString(string(text), -1) {
			if !protocol.SupportsProtocolVersion(match) {
				t.Errorf("%s names protocol version %q, which this build does not serve", name, match)
				continue
			}
			found = true
		}
		if !found {
			t.Errorf("%s states no protocol version; a canonical doc must say which one it describes", name)
		}
	}
}

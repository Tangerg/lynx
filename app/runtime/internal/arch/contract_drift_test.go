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
	"slices"
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
	regenerated, regeneratedTS, regeneratedValidators := regenerateContract(t, root)

	// Two artifacts land outside contract/ — the TypeScript types in the frontend's
	// tree and the Go validator beside the shapes it checks — so they are compared
	// one by one, under the same rule: the generator reran and nothing moved.
	for _, outside := range []struct{ fresh, committed, name string }{
		{regeneratedTS, filepath.Join(root, tsWireDir), tsWireTypes},
		{regeneratedValidators, filepath.Join(root, validatorDir), validatorFile},
	} {
		if !bytes.Equal(readArtifact(t, outside.fresh, outside.name), readArtifact(t, outside.committed, outside.name)) {
			t.Errorf("%s is stale — run `go generate ./...` and commit the result", outside.name)
		}
	}

	fresh := artifactNames(t, regenerated)
	committed := artifactNames(t, filepath.Join(root, "contract"))
	for _, name := range fresh {
		if !slices.Contains(committed, name) {
			t.Errorf("the generator writes %s and the tree has no copy — run `go generate ./...` and commit it", name)
		}
	}
	for _, name := range committed {
		if !slices.Contains(fresh, name) {
			t.Errorf("contract/%s is no longer generated — delete it rather than leaving an artifact nothing authors", name)
		}
	}
	for _, name := range fresh {
		if !slices.Contains(committed, name) {
			continue
		}
		if !bytes.Equal(readArtifact(t, regenerated, name), readArtifact(t, filepath.Join(root, "contract"), name)) {
			t.Errorf("contract/%s is stale — run `go generate ./...` and commit the result", name)
		}
	}
}

// Where the generated TypeScript lands, mirroring the go:generate directive on
// dispatch's contract_methods.go. The frontend consumes the wire types from its own
// tree — a client that imported them across the module boundary would not build.
const (
	tsWireDir     = "../desktop/frontend/src/rpc"
	tsWireTypes   = "wire.generated.ts"
	validatorDir  = "internal/delivery/protocol"
	validatorFile = "request_constraints.generated.go"
)

func regenerateContract(t *testing.T, root string) (artifacts, typescript, validators string) {
	t.Helper()

	artifacts, typescript, validators = t.TempDir(), t.TempDir(), t.TempDir()
	cmd := exec.Command("go", "run", "github.com/Tangerg/lynx/app/runtime/cmd/contractgen",
		"-out", artifacts, "-ts", typescript, "-validators", validators)
	cmd.Dir = root
	if combined, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("run contractgen: %v\n%s", err, combined)
	}
	return artifacts, typescript, validators
}

func artifactNames(t *testing.T, dir string) []string {
	t.Helper()

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read %s: %v", dir, err)
	}
	var out []string
	for _, entry := range entries {
		if !entry.IsDir() {
			out = append(out, entry.Name())
		}
	}
	slices.Sort(out)
	return out
}

func readArtifact(t *testing.T, dir, name string) []byte {
	t.Helper()

	content, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatalf("read %s: %v", filepath.Join(dir, name), err)
	}
	return content
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
		CarriedShapes    []struct{} `json:"carriedShapes"`
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
		"carriedShapes":     len(manifest.CarriedShapes),
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
	for _, name := range []string{"request_constraints.go", validatorFile} {
		assertConstraintsArePure(t, filepath.Join(root, validatorDir, name))
	}
}

func assertConstraintsArePure(t *testing.T, path string) {
	t.Helper()

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

// TestGeneratedSchemasResolve is contract §11.4 gate 4: the OpenRPC document and
// the JSON Schema bundle parse, and every reference in them lands on a definition
// that exists.
//
// A dangling reference is the failure mode that matters here, because it is
// invisible to the drift gate: a stale generator produces a self-consistent pair
// of files and byte-compares clean forever. What it cannot survive is being
// resolved. The check is written here rather than delegated to a validator library
// so it holds on a machine with no network and no vendored schema tooling.
func TestGeneratedSchemasResolve(t *testing.T) {
	root := moduleRoot(t)
	dir := filepath.Join(root, "contract")

	var bundle struct {
		Schema string                     `json:"$schema"`
		Defs   map[string]json.RawMessage `json:"$defs"`
	}
	if err := json.Unmarshal(readArtifact(t, dir, "schema.json"), &bundle); err != nil {
		t.Fatalf("decode schema.json: %v", err)
	}
	if bundle.Schema == "" {
		t.Error("schema.json states no dialect; a schema whose dialect is a guess is not a contract")
	}
	if len(bundle.Defs) == 0 {
		t.Fatal("schema.json defines no types")
	}

	// Inside the bundle a reference is document-local; from the method document it
	// carries the bundle's file name, because the shapes have exactly one home.
	referenced := make(map[string]bool)
	for _, document := range []struct {
		name   string
		prefix string
	}{
		{"schema.json", "#/$defs/"},
		{"openrpc.json", "schema.json#/$defs/"},
		{"manifest.json", "schema.json#/$defs/"},
	} {
		var decoded any
		if err := json.Unmarshal(readArtifact(t, dir, document.name), &decoded); err != nil {
			t.Fatalf("decode %s: %v", document.name, err)
		}
		refs := collectRefs(decoded)
		if len(refs) == 0 {
			t.Errorf("%s references no shapes at all", document.name)
		}
		for _, ref := range refs {
			target, ok := strings.CutPrefix(ref, document.prefix)
			if !ok {
				t.Errorf("%s references %q, which does not point into the shape bundle", document.name, ref)
				continue
			}
			if _, ok := bundle.Defs[target]; !ok {
				t.Errorf("%s references %q, which schema.json does not define", document.name, ref)
				continue
			}
			referenced[target] = true
		}
	}

	// A definition nothing points at describes a frame no method can carry — either
	// a spec registered against an unreachable type, or a shape that outlived its
	// method.
	for name := range bundle.Defs {
		if !referenced[name] {
			t.Errorf("schema.json defines %q and nothing references it", name)
		}
	}
}

// TestOpenRPCDescribesEveryMethod pins the method document to the registry: the
// two artifacts are generated from one source, so a gap here means a method was
// projected into one artifact and not the other.
func TestOpenRPCDescribesEveryMethod(t *testing.T) {
	root := moduleRoot(t)
	dir := filepath.Join(root, "contract")

	var document struct {
		OpenRPC string `json:"openrpc"`
		Info    struct {
			Version string `json:"version"`
		} `json:"info"`
		Methods []struct {
			Name           string `json:"name"`
			ParamStructure string `json:"paramStructure"`
			Kind           string `json:"x-lyra-kind"`
			Stability      string `json:"x-lyra-stability"`
		} `json:"methods"`
	}
	if err := json.Unmarshal(readArtifact(t, dir, "openrpc.json"), &document); err != nil {
		t.Fatalf("decode openrpc.json: %v", err)
	}
	if document.OpenRPC == "" {
		t.Error("openrpc.json states no spec version")
	}
	if document.Info.Version != protocol.ProtocolVersion {
		t.Errorf("openrpc.json is version %q, the code serves %q", document.Info.Version, protocol.ProtocolVersion)
	}

	var manifest struct {
		Methods []struct {
			Name      string `json:"name"`
			Kind      string `json:"kind"`
			Stability string `json:"stability"`
		} `json:"methods"`
	}
	if err := json.Unmarshal(readArtifact(t, dir, "manifest.json"), &manifest); err != nil {
		t.Fatalf("decode manifest.json: %v", err)
	}
	if len(document.Methods) != len(manifest.Methods) {
		t.Fatalf("openrpc.json has %d methods, the manifest has %d", len(document.Methods), len(manifest.Methods))
	}
	for index, method := range manifest.Methods {
		described := document.Methods[index]
		switch {
		case described.Name != method.Name:
			t.Errorf("method %d: openrpc says %q, the manifest says %q", index, described.Name, method.Name)
		case described.Kind != method.Kind:
			t.Errorf("%s: openrpc says %q, the manifest says %q", method.Name, described.Kind, method.Kind)
		case described.Stability != method.Stability:
			t.Errorf("%s: openrpc says %q, the manifest says %q", method.Name, described.Stability, method.Stability)
		case described.ParamStructure != "by-name":
			t.Errorf("%s: params are %q; the wire passes one object keyed by field name", method.Name, described.ParamStructure)
		}
	}
}

// collectRefs walks decoded JSON and returns every $ref value it finds.
func collectRefs(node any) []string {
	switch typed := node.(type) {
	case map[string]any:
		var out []string
		for key, value := range typed {
			if key == "$ref" {
				if ref, ok := value.(string); ok {
					out = append(out, ref)
				}
				continue
			}
			out = append(out, collectRefs(value)...)
		}
		return out
	case []any:
		var out []string
		for _, value := range typed {
			out = append(out, collectRefs(value)...)
		}
		return out
	default:
		return nil
	}
}

// notOnTheWire are the exported structs in the protocol package that the shape
// bundle is right to omit, each with the reason.
//
// It is a closed list on purpose. Everything else in that package IS the wire, so
// a new exported struct either appears in the bundle or has to be named here — the
// same discipline the union field-coverage check applies one level down. Without
// it, the way a shape goes unpublished is silently: nobody notices that no method
// reaches it.
var notOnTheWire = map[string]string{
	"ConstraintError": "the Go validator's error carrier; its wire projection is ProblemData.errors",
	"WireField":       "reflection over the wire types, not a wire type",
	"WorkspaceQuery":  "an embedded mixin — encoding/json inlines its fields, so the wire has no such object",
}

// TestEveryWireStructIsPublished checks the bundle against the protocol package.
//
// The walk starts from the registered methods, which means a shape reachable from
// nothing — a tool result's members, an envelope member — is absent from every
// artifact and nothing complains. That is how the contract quietly loses a type a
// client still has to render, so the declaration mechanism exists (carried shapes,
// state payloads) and this proves it was used.
func TestEveryWireStructIsPublished(t *testing.T) {
	root := moduleRoot(t)

	var bundle struct {
		Defs map[string]json.RawMessage `json:"$defs"`
	}
	if err := json.Unmarshal(readArtifact(t, filepath.Join(root, "contract"), "schema.json"), &bundle); err != nil {
		t.Fatalf("decode schema.json: %v", err)
	}
	// A generic publishes as one definition per instantiation (PageOfSession), so the
	// base name counts as published.
	published := make(map[string]bool, len(bundle.Defs))
	for name := range bundle.Defs {
		published[name] = true
		if base, _, generic := strings.Cut(name, "Of"); generic {
			published[base] = true
		}
	}

	for _, name := range exportedStructs(t, filepath.Join(root, "internal", "delivery", "protocol")) {
		reason, excused := notOnTheWire[name]
		switch {
		case published[name] && excused:
			t.Errorf("%s is published AND listed as not on the wire (%q) — one of the two is wrong", name, reason)
		case !published[name] && !excused:
			t.Errorf("%s is a wire struct no artifact describes; register how it is carried, or say why it is not on the wire", name)
		}
	}
	for name := range notOnTheWire {
		if !slices.Contains(exportedStructs(t, filepath.Join(root, "internal", "delivery", "protocol")), name) {
			t.Errorf("%s is excused from the bundle and no longer exists", name)
		}
	}
}

func exportedStructs(t *testing.T, dir string) []string {
	t.Helper()

	files, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		t.Fatalf("glob %s: %v", dir, err)
	}
	var out []string
	for _, path := range files {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		for _, decl := range file.Decls {
			group, ok := decl.(*ast.GenDecl)
			if !ok || group.Tok != token.TYPE {
				continue
			}
			for _, spec := range group.Specs {
				typeSpec, ok := spec.(*ast.TypeSpec)
				if !ok || !typeSpec.Name.IsExported() {
					continue
				}
				if _, ok := typeSpec.Type.(*ast.StructType); ok {
					out = append(out, typeSpec.Name.Name)
				}
			}
		}
	}
	slices.Sort(out)
	return out
}

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
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/contractshape"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/dispatch"
	"github.com/Tangerg/lynx/app/runtime/protocol"
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

	// Some artifacts land outside contract/'s root — the published TypeScript binding
	// in its own subdirectory and the Go validator beside the shapes it checks. Their
	// homes may hold other files, so the rule runs one way: every file the generator
	// writes there must match the copy in the tree.
	for _, outside := range []struct{ fresh, committed string }{
		{regeneratedTS, filepath.Join(root, tsWireDir)},
		{regeneratedValidators, filepath.Join(root, validatorDir)},
	} {
		for _, name := range artifactNames(t, outside.fresh) {
			if !bytes.Equal(readArtifact(t, outside.fresh, name), readArtifact(t, outside.committed, name)) {
				t.Errorf("%s is stale — run `go generate ./...` and commit the result", name)
			}
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

// Where the runtime publishes its generated TypeScript binding, mirroring the
// generation directive on dispatch's contract_methods.go. Client modules own how
// they consume or vendor this output; the runtime never writes into their trees.
const (
	tsWireDir       = "contract/typescript"
	tsWireValidator = "wire.validate.generated.ts"
	validatorDir    = "protocol"
	validatorFile   = "wire_constraints.generated.go"
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
		ProtocolVersion  string     `json:"protocolVersion"`
		Methods          []struct{} `json:"methods"`
		StreamingMethods []string   `json:"streamingMethods"`
		HTTPEndpoints    []struct{} `json:"httpEndpoints"`
		Errors           struct {
			Types []struct{} `json:"types"`
		} `json:"errors"`
		CapabilityPolicy    []struct{} `json:"capabilityPolicy"`
		RunEventPolicy      []struct{} `json:"runEventPolicy"`
		CarriedShapes       []struct{} `json:"carriedShapes"`
		ResultPresentations []struct{} `json:"toolResultPresentations"`
		Unions              []struct{} `json:"unions"`
		Constraints         []struct{} `json:"objectConstraints"`
		ValueConstraints    []struct{} `json:"valueConstraints"`
		SystemInvariants    []struct{} `json:"systemInvariants"`
	}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatalf("decode manifest: %v", err)
	}
	sections := map[string]int{
		"protocolVersion":         len(manifest.ProtocolVersion),
		"methods":                 len(manifest.Methods),
		"streamingMethods":        len(manifest.StreamingMethods),
		"httpEndpoints":           len(manifest.HTTPEndpoints),
		"errors.types":            len(manifest.Errors.Types),
		"capabilityPolicy":        len(manifest.CapabilityPolicy),
		"carriedShapes":           len(manifest.CarriedShapes),
		"toolResultPresentations": len(manifest.ResultPresentations),
		"runEventPolicy":          len(manifest.RunEventPolicy),
		"unions":                  len(manifest.Unions),
		"objectConstraints":       len(manifest.Constraints),
		"valueConstraints":        len(manifest.ValueConstraints),
		"systemInvariants":        len(manifest.SystemInvariants),
	}
	for section, count := range sections {
		if count == 0 {
			t.Errorf("manifest section %q is empty; the drift gate would pass on nothing", section)
		}
	}
}

// TestWireConstraintsStayPure is contract §11.4 gate 7: a DTO validator's
// dependency graph contains no store, dispatcher or executor. The private,
// reflection-only contractshape helper is the sole permitted internal import.
//
// A shape constraint is safe on either wire direction precisely because checking
// it costs nothing and cannot fail for an environmental reason. Give ValidateWire
// a repository and request "invalid_params" starts meaning "the database was slow"
// while output validity starts depending on I/O; the generated client could not be
// equivalent to either.
func TestWireConstraintsStayPure(t *testing.T) {
	root := moduleRoot(t)
	for _, name := range []string{"wire_constraints.go", validatorFile} {
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
		if strings.Contains(imported, "/internal/") && !strings.HasSuffix(imported, "/internal/contractshape") {
			t.Errorf("wire constraints import %q; a shape constraint may only read the value it validates", imported)
		}
	}
	// ValidateWire must also be reachable without a context: a check that needs one is
	// a check that does I/O.
	full, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	for _, decl := range full.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "ValidateWire" || fn.Recv == nil {
			continue
		}
		if fn.Type.Params != nil && len(fn.Type.Params.List) != 0 {
			t.Errorf("%s.ValidateWire takes parameters; a shape constraint reads only its own value", exprString(fn.Recv.List[0].Type))
		}
	}
}

// TestRegistryShapeRulesReachTheGoValidator closes the server-side half of the
// three-way contract: every union, conditional object and value-constrained shape
// registered for schema/TypeScript also owns a generated ValidateWire method.
func TestRegistryShapeRulesReachTheGoValidator(t *testing.T) {
	root := moduleRoot(t)
	source := string(readArtifact(t, filepath.Join(root, validatorDir), validatorFile))
	shapes := dispatch.WireShapes()

	var registered []reflect.Type
	for _, spec := range shapes.Unions() {
		registered = append(registered, spec.GoType)
	}
	for _, spec := range shapes.Constraints() {
		registered = append(registered, spec.GoType)
	}
	for _, spec := range shapes.ValueConstraints() {
		registered = append(registered, spec.GoType)
	}
	for _, shape := range registered {
		// The generator names the receiver after its type, exactly as
		// hand-written methods do, so the expected signature is derived the
		// same way rather than assuming one shared name.
		receiver := strings.ToLower(shape.Name()[:1])
		signature := "func (" + receiver + " " + shape.Name() + ") ValidateWire() error"
		if !strings.Contains(source, signature) {
			t.Errorf("%s is registered as a wire-constrained shape but has no generated ValidateWire method", shape.Name())
		}
	}
}

// TestProtocolVersionAgreesEverywhere is contract §11.4 gate 12: the generated
// manifest, the Runtime-owned canonical docs and the code state one protocol
// version.
//
// This is the drift A1 had to fix by hand, machine-enforced. C16 flips the version
// in ONE place — the constant — and this gate then names every document still
// claiming the old one, instead of a reader discovering the mismatch later from a
// client that negotiated against a stale header.
func TestProtocolVersionAgreesEverywhere(t *testing.T) {
	root := moduleRoot(t)

	var manifest struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	raw, err := os.ReadFile(filepath.Join(root, "contract", "manifest.json"))
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	if unmarshalErr := json.Unmarshal(raw, &manifest); unmarshalErr != nil {
		t.Fatalf("decode manifest: %v", unmarshalErr)
	}
	if manifest.ProtocolVersion != protocol.ProtocolVersion {
		t.Errorf("manifest protocol %q != code %q", manifest.ProtocolVersion, protocol.ProtocolVersion)
	}

	// The canonical docs each state the version in their own header. Any version
	// literal they carry must be one the code actually serves; a doc naming a
	// version the runtime would reject is worse than no doc, because a client
	// negotiates against it.
	dateLiteral := regexp.MustCompile(`\b20\d\d-\d\d-\d\d\b`)
	for _, name := range []string{"API.md", "AUX_API.md", "TRANSPORT.md"} {
		path := filepath.Join(root, "doc", name)
		text, readFileErr := os.ReadFile(path)
		if readFileErr != nil {
			t.Fatalf("read %s: %v", name, readFileErr)
		}
		found := false
		for _, match := range dateLiteral.FindAllString(string(text), -1) {
			if match != protocol.ProtocolVersion {
				t.Errorf("%s names protocol version %q, which this build does not serve", name, match)
				continue
			}
			found = true
		}
		if !found {
			t.Errorf("%s states no protocol version; a canonical doc must say which one it describes", name)
		}
	}

	// The canonical samples are the third published statement. A client copies them
	// — that is what a canonical sample is for — so one naming a retired version
	// hands out a handshake the runtime refuses. The shape gate cannot see it: the
	// schema constrains the field's type, never which date is current.
	//
	// The client's own constant is NOT read here: it is generated from the same
	// code, so gate 1 already holds it. Test fixtures are not read either — a
	// refusal test has to be able to name a version this build rejects.
	samples, err := filepath.Glob(filepath.Join(root, tsWireDir, "samples", "*.json"))
	if err != nil {
		t.Fatalf("glob canonical samples: %v", err)
	}
	if len(samples) == 0 {
		t.Fatal("no canonical samples found; the sweep below would pass on nothing")
	}
	stated := 0
	for _, path := range samples {
		var decoded any
		if err := json.Unmarshal(readArtifact(t, filepath.Dir(path), filepath.Base(path)), &decoded); err != nil {
			t.Fatalf("decode %s: %v", filepath.Base(path), err)
		}
		for _, version := range collectProtocolVersions(decoded) {
			stated++
			if version != protocol.ProtocolVersion {
				t.Errorf("canonical sample %s states protocol version %q, which this build does not serve",
					filepath.Base(path), version)
			}
		}
	}
	if stated == 0 {
		t.Error("no canonical sample states a protocol version; the handshake fixtures stopped covering it")
	}
}

// TestProtocolContractIsRuntimeOwned prevents a consuming application from
// becoming the Runtime's contract author again. Clients may vendor generated
// bindings, but production code, generators and current architecture guidance
// must resolve the protocol entirely inside this module.
func TestProtocolContractIsRuntimeOwned(t *testing.T) {
	root := moduleRoot(t)
	leaks := []string{
		strings.Join([]string{"desktop", "docs", "protocol"}, "/"),
		strings.Join([]string{`filepath.Join(root, "`, `..", "`, `desktop"`}, ""),
	}
	for _, rel := range []string{"CLAUDE.md", "cmd", "internal", "doc/README.md", "doc/ARCHITECTURE.md"} {
		path := filepath.Join(root, rel)
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat %s: %v", rel, err)
		}
		paths := []string{path}
		if info.IsDir() {
			paths = nil
			err := filepath.WalkDir(path, func(candidate string, entry os.DirEntry, walkErr error) error {
				if walkErr != nil {
					return walkErr
				}
				if !entry.IsDir() && slices.Contains([]string{".go", ".md"}, filepath.Ext(candidate)) {
					paths = append(paths, candidate)
				}
				return nil
			})
			if err != nil {
				t.Fatalf("walk %s: %v", rel, err)
			}
		}
		for _, candidate := range paths {
			source, err := os.ReadFile(candidate)
			if err != nil {
				t.Fatalf("read %s: %v", candidate, err)
			}
			for _, leaked := range leaks {
				if bytes.Contains(source, []byte(leaked)) {
					t.Errorf("%s resolves the Runtime protocol through a client module (%q)", candidate, leaked)
				}
			}
		}
	}
}

// protocolVersionKeys are the wire's only spellings of a protocol version.
var protocolVersionKeys = []string{"protocolVersion"}

// collectProtocolVersions walks decoded JSON for values stated under those keys, so
// a sample added later is swept without anyone remembering to list it.
func collectProtocolVersions(node any) []string {
	switch typed := node.(type) {
	case map[string]any:
		var out []string
		for key, value := range typed {
			if version, ok := value.(string); ok && slices.Contains(protocolVersionKeys, key) {
				out = append(out, version)
				continue
			}
			out = append(out, collectProtocolVersions(value)...)
		}
		return out
	case []any:
		var out []string
		for _, value := range typed {
			out = append(out, collectProtocolVersions(value)...)
		}
		return out
	default:
		return nil
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
			Name string `json:"name"`
			Kind string `json:"kind"`
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
	"Feature":         "the published capability vocabulary's registry entry; its wire projection is FeatureCapability",
	"ConstraintError": "the Go validator's error carrier; its wire projection is ProblemData.errors",
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

	for _, name := range exportedStructs(t, filepath.Join(root, "protocol")) {
		reason, excused := notOnTheWire[name]
		switch {
		case published[name] && excused:
			t.Errorf("%s is published AND listed as not on the wire (%q) — one of the two is wrong", name, reason)
		case !published[name] && !excused:
			t.Errorf("%s is a wire struct no artifact describes; register how it is carried, or say why it is not on the wire", name)
		}
	}
	for name := range notOnTheWire {
		if !slices.Contains(exportedStructs(t, filepath.Join(root, "protocol")), name) {
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

// TestValueConstraintsAgreeAcrossArtifacts is contract §11.4 gate 6: every declared
// value constraint is stated by all THREE emitters.
//
// One declaration feeds three independent emitters — the Go validator writes a
// matching helper call, the schema writes its value keyword, and the TypeScript
// checks write the same keyword — while a nested path takes a fourth code path in two of them (an
// allOf branch, because the rule belongs to the owner and not to every carrier of
// the shared type). Construction does not make them agree; only reading the
// artifacts back does.
func TestValueConstraintsAgreeAcrossArtifacts(t *testing.T) {
	root := moduleRoot(t)
	dir := filepath.Join(root, "contract")

	var bundle struct {
		Defs map[string]json.RawMessage `json:"$defs"`
	}
	if err := json.Unmarshal(readArtifact(t, dir, "schema.json"), &bundle); err != nil {
		t.Fatalf("decode schema.json: %v", err)
	}
	validator := string(readArtifact(t, filepath.Join(root, validatorDir), validatorFile))
	checks := checkEntries(t, string(readArtifact(t, filepath.Join(root, tsWireDir), tsWireValidator)))

	shapes := dispatch.WireShapes()
	unions := make(map[reflect.Type]dispatch.UnionSpec)
	for _, union := range shapes.Unions() {
		unions[union.GoType] = union
	}
	for _, spec := range shapes.ValueConstraints() {
		shape := spec.GoType.Name()
		definition, ok := bundle.Defs[shape]
		if !ok {
			t.Errorf("%s carries value constraints and the bundle does not define it", shape)
			continue
		}
		for _, constraint := range spec.Constraints {
			expected := expectedCompiledConstraint(t, shape, spec.GoType, constraint, unions[spec.GoType])
			assertCompiledConstraint(t, shape, constraint, expected, definition, validator, checks)
		}
	}
}

type compiledConstraintExpectation struct {
	schemaKeyword string
	goHelper      string
}

func expectedCompiledConstraint(
	t *testing.T,
	shape string,
	goType reflect.Type,
	constraint dispatch.FieldConstraint,
	union dispatch.UnionSpec,
) compiledConstraintExpectation {
	t.Helper()
	leaf := func() contractshape.Field {
		_, field, found := contractshape.GoPath(goType, constraint.Field)
		if !found {
			t.Fatalf("%s has no field %q", shape, constraint.Field)
		}
		return field
	}
	switch constraint.Kind {
	case dispatch.ConstraintNonEmpty:
		field := leaf()
		if field.Optional && field.Type.Kind() == reflect.Pointer {
			return compiledConstraintExpectation{"minLength", "optionalText"}
		}
		if field.Optional && unionRequiresField(union, constraint.Field) {
			return compiledConstraintExpectation{"minLength", "requiredWhen"}
		}
		return compiledConstraintExpectation{"minLength", "requiredText"}
	case dispatch.ConstraintPositive:
		field := leaf()
		switch {
		case field.Type.Kind() == reflect.Pointer:
			return compiledConstraintExpectation{"minimum", "optionalPositiveNumber"}
		case field.Optional:
			return compiledConstraintExpectation{"minimum", "optionalPositiveScalarNumber"}
		default:
			return compiledConstraintExpectation{"minimum", "positiveNumber"}
		}
	case dispatch.ConstraintNonNegative:
		if leaf().Type.Kind() == reflect.Pointer {
			return compiledConstraintExpectation{"minimum", "optionalNonNegativeNumber"}
		}
		return compiledConstraintExpectation{"minimum", "nonNegativeNumber"}
	case dispatch.ConstraintNonEmptyItems:
		if leaf().Optional {
			return compiledConstraintExpectation{"minItems", "nonEmptyItems"}
		}
		return compiledConstraintExpectation{"minItems", "requiredItems"}
	case dispatch.ConstraintNonEmptyProperties:
		return compiledConstraintExpectation{"minProperties", "nonEmptyProperties"}
	case dispatch.ConstraintUniqueItems:
		if leaf().Type.Kind() == reflect.Pointer {
			return compiledConstraintExpectation{"uniqueItems", "optionalUniqueItems"}
		}
		return compiledConstraintExpectation{"uniqueItems", "uniqueItems"}
	case dispatch.ConstraintMinItems:
		if leaf().Optional {
			return compiledConstraintExpectation{"minItems", "optionalMinItems"}
		}
		return compiledConstraintExpectation{"minItems", "requiredMinItems"}
	case dispatch.ConstraintMaxLength:
		if leaf().Type.Kind() == reflect.Pointer {
			return compiledConstraintExpectation{"maxLength", "optionalMaxLength"}
		}
		return compiledConstraintExpectation{"maxLength", "maxLength"}
	case dispatch.ConstraintMinimum:
		if leaf().Type.Kind() == reflect.Pointer {
			return compiledConstraintExpectation{"minimum", "optionalMinimumNumber"}
		}
		return compiledConstraintExpectation{"minimum", "minimumNumber"}
	case dispatch.ConstraintMaximum:
		if leaf().Type.Kind() == reflect.Pointer {
			return compiledConstraintExpectation{"maximum", "optionalMaximumNumber"}
		}
		return compiledConstraintExpectation{"maximum", "maximumNumber"}
	default:
		t.Fatalf("%s.%s has unsupported constraint kind %s", shape, constraint.Field, constraint.Kind)
		return compiledConstraintExpectation{}
	}
}

func unionRequiresField(union dispatch.UnionSpec, field string) bool {
	for _, variant := range union.Variants {
		if slices.Contains(variant.Required, field) {
			return true
		}
	}
	return union.PatternVariant != nil && slices.Contains(union.PatternVariant.Required, field)
}

func assertCompiledConstraint(
	t *testing.T,
	shape string,
	constraint dispatch.FieldConstraint,
	expected compiledConstraintExpectation,
	definition json.RawMessage,
	validator string,
	checks map[string]string,
) {
	t.Helper()
	// The schema states a direct field on its property and a nested field in the
	// owning shape's allOf branch.
	if !constraintInSchema(t, definition, constraint.Field, expected.schemaKeyword, constraint.Limit) {
		t.Errorf("%s.%s is declared %s and schema.json states no %s for it",
			shape, constraint.Field, constraint, expected.schemaKeyword)
	}
	if !statesGoConstraint(validator, shape, expected.goHelper, constraint.Field, constraint.Limit) {
		t.Errorf("%s.%s is declared %s and the generated validator has no %s call",
			shape, constraint.Field, constraint, expected.goHelper)
	}
	entry, found := checks[shape]
	if !found {
		t.Errorf("%s carries value constraints and %s has no check for it", shape, tsWireValidator)
		return
	}
	if !statesCheck(entry, constraint.Field, expected.schemaKeyword, constraint.Limit) {
		t.Errorf("%s.%s is declared %s and its %s check states no %s",
			shape, constraint.Field, constraint, tsWireValidator, expected.schemaKeyword)
	}
}

// statesCheck reports whether one shape's checks constrain the last segment of a
// dotted path with the given keyword. A value constraint compiles to a single line
// either way it is stated — `sessionId: allOf([text(), minLength(1)])` beside a type
// keyword, `id: minLength(1)` alone in an allOf branch — so the line the field names
// is the whole rule.
func statesCheck(entry, path, keyword string, limit int) bool {
	segments := strings.Split(path, ".")
	field := segments[len(segments)-1] + ": "
	call := keyword + "("
	if limit > 0 {
		call += strconv.Itoa(limit) + ")"
	}
	for line := range strings.SplitSeq(entry, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, field) && strings.Contains(trimmed, call) {
			return true
		}
	}
	return false
}

func statesGoConstraint(source, shape, helper, field string, limit int) bool {
	// The generator names the receiver after its type, so both the method
	// marker and the receiver argument are derived from the shape name.
	receiver := strings.ToLower(shape[:1])
	marker := "func (" + receiver + " " + shape + ") ValidateWire() error {"
	start := strings.Index(source, marker)
	if start < 0 {
		return false
	}
	source = source[start:]
	if end := strings.Index(source[len(marker):], "\nfunc "); end >= 0 {
		source = source[:len(marker)+end]
	}
	if helper == "requiredWhen" {
		fieldArgument := `, "` + field + `", ` + receiver + `)`
		for line := range strings.SplitSeq(source, "\n") {
			if strings.Contains(line, "requiredWhen(") && strings.Contains(line, fieldArgument) {
				return true
			}
		}
		return false
	}
	prefix := helper + `("` + field + `"`
	for line := range strings.SplitSeq(source, "\n") {
		if !strings.Contains(line, prefix) {
			continue
		}
		return limit == 0 || strings.Contains(line, ", "+strconv.Itoa(limit)+")")
	}
	return false
}

// checkEntries splits the generated TypeScript checks into one entry per published
// shape. Every entry starts a line at the registry's own indentation, so the text
// between two such lines is one shape's whole rule set.
func checkEntries(t *testing.T, source string) map[string]string {
	t.Helper()

	entry := regexp.MustCompile(`(?m)^  ([A-Za-z0-9_]+): `)
	matches := entry.FindAllStringSubmatchIndex(source, -1)
	if len(matches) == 0 {
		t.Fatalf("%s publishes no checks", tsWireValidator)
	}
	out := make(map[string]string, len(matches))
	for index, match := range matches {
		end := len(source)
		if index+1 < len(matches) {
			end = matches[index+1][0]
		}
		out[source[match[2]:match[3]]] = source[match[0]:end]
	}
	return out
}

// constraintInSchema reports whether the definition constrains the last segment of
// a dotted path with the given keyword, at any depth.
func constraintInSchema(t *testing.T, definition json.RawMessage, path, keyword string, limit int) bool {
	t.Helper()

	var decoded any
	if err := json.Unmarshal(definition, &decoded); err != nil {
		t.Fatalf("decode definition: %v", err)
	}
	segments := strings.Split(path, ".")
	return statesKeyword(decoded, segments[len(segments)-1], keyword, limit)
}

func statesKeyword(node any, property, keyword string, limit int) bool {
	switch typed := node.(type) {
	case map[string]any:
		if mapStatesPropertyConstraint(typed, property, keyword, limit) {
			return true
		}
		for _, value := range typed {
			if statesKeyword(value, property, keyword, limit) {
				return true
			}
		}
	case []any:
		for _, value := range typed {
			if statesKeyword(value, property, keyword, limit) {
				return true
			}
		}
	}
	return false
}

func mapStatesPropertyConstraint(node map[string]any, property, keyword string, limit int) bool {
	properties, hasProperties := node["properties"].(map[string]any)
	if !hasProperties {
		return false
	}
	constrained, hasProperty := properties[property].(map[string]any)
	if !hasProperty {
		return false
	}
	value, stated := constrained[keyword]
	if !stated || limit == 0 {
		return stated
	}
	number, isNumber := value.(float64)
	return isNumber && number == float64(limit)
}

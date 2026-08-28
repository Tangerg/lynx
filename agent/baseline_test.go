package agent

import (
	"crypto/sha256"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"
)

const (
	currentAPIBaseline         = 35
	currentAPIBaselineFrozenOn = "2026-08-29"
)

var exportedAPIBaselines = []struct {
	name      string
	label     string
	directory string
	want      string
}{
	{name: "kernel", label: "root kernel", directory: ".", want: "4b7e02f98828bc4f2762eb32c92a0975567b0441c736726fb0b54901be0756d8"},
	{name: "agenttest", label: "agenttest", directory: "agenttest", want: "66c49a9dec9e33d416fbc66c925a5893d54b96335d4e09294ad87b40badc6ad9"},
	{name: "interaction", label: "interaction", directory: "interaction", want: "8d5b982f92cdbd457970f8e9678c058198a937ff9ca42740af26e5e4db383b75"},
	{name: "planning", label: "planning", directory: "planning", want: "4ca18318b81c7fc646ee7121c9f2e303df2cc2d28fcd8357e2c3b07cebd1a014"},
	{name: "goap", label: "planning/goap", directory: "planning/goap", want: "0f7376bbf8b1815315005984540bdc5d5aabb7bccea5147f957c2911a55e04e7"},
	{name: "workflow", label: "workflow", directory: "workflow", want: "f0f995aa9a543e0985d650ca9815d6baac1aa6367f6d75483b958489113fda16"},
	{name: "platform", label: "platform", directory: "platform", want: "f55a3f35ae808a709284bc828e0bd8dcdc8d258bf2a8185ebe2a88da39a97d1e"},
}

var frameworkPackageDirectories = []string{
	".", "agenttest", "interaction", "planning", "planning/goap", "workflow", "platform",
}

func TestPublicInterfacesAreDocumentedAndParametersNamed(t *testing.T) {
	for _, path := range frameworkProductionGoFiles(t) {
		assertPublicContractPolicyInFile(t, path)
	}
}

func TestManagedExecutionVocabularyIsUnambiguous(t *testing.T) {
	paths := append(frameworkProductionGoFiles(t),
		"doc/API_BASELINE.md",
		"doc/ARCHITECTURE.md",
		"doc/CAPABILITY_LEDGER.md",
		"doc/DECISIONS.md",
		"doc/ENGINEERING_STANDARDS.md",
		"doc/EXECUTION_PLAN.md",
	)
	forbidden := []*regexp.Regexp{
		regexp.MustCompile(`(?i)\bna` + `tive\b`),
		regexp.MustCompile(regexp.QuoteMeta("原" + "生")),
		regexp.MustCompile(`\bStep trans` + `actions?\b`),
		regexp.MustCompile(regexp.QuoteMeta("候选" + "事务")),
	}
	for _, path := range paths {
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, pattern := range forbidden {
			if pattern.Match(content) {
				t.Errorf("%s uses retired managed-execution terminology matching %q", path, pattern)
			}
		}
	}
}

func frameworkProductionGoFiles(t *testing.T) []string {
	t.Helper()
	var paths []string
	for _, directory := range frameworkPackageDirectories {
		entries, err := os.ReadDir(directory)
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			name := entry.Name()
			if !entry.IsDir() && strings.HasSuffix(name, ".go") && !strings.HasSuffix(name, "_test.go") {
				paths = append(paths, filepath.Join(directory, name))
			}
		}
	}
	return paths
}

func assertPublicContractPolicyInFile(t *testing.T, path string) {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	for _, declaration := range file.Decls {
		switch declaration := declaration.(type) {
		case *ast.FuncDecl:
			assertExportedFunctionParametersNamed(t, path, declaration)
		case *ast.GenDecl:
			assertExportedInterfaceContracts(t, path, declaration)
		}
	}
}

func assertExportedFunctionParametersNamed(t *testing.T, path string, declaration *ast.FuncDecl) {
	t.Helper()
	publicReceiver := declaration.Recv == nil || token.IsExported(receiverTypeName(declaration.Recv))
	if !declaration.Name.IsExported() || !publicReceiver {
		return
	}
	assertParametersAreNamed(t, path, declaration.Name.Name, declaration.Type.Params)
}

func assertExportedInterfaceContracts(t *testing.T, path string, declaration *ast.GenDecl) {
	t.Helper()
	for _, specification := range declaration.Specs {
		if specification, ok := specification.(*ast.TypeSpec); ok {
			if !specification.Name.IsExported() {
				continue
			}
			assertExportedTypeContract(t, path, declaration.Doc, specification)
		}
	}
}

func assertExportedTypeContract(
	t *testing.T,
	path string,
	declarationDoc *ast.CommentGroup,
	specification *ast.TypeSpec,
) {
	t.Helper()
	switch declaration := specification.Type.(type) {
	case *ast.InterfaceType:
		doc := specification.Doc
		if doc == nil {
			doc = declarationDoc
		}
		assertGoDocStartsWithName(t, path, specification.Name.Name, doc)
		for _, method := range declaration.Methods.List {
			function, ok := method.Type.(*ast.FuncType)
			if !ok {
				continue
			}
			name := specification.Name.Name
			if len(method.Names) > 0 {
				methodName := method.Names[0].Name
				name += "." + methodName
				assertGoDocStartsWithName(t, path, methodName, method.Doc)
			}
			assertParametersAreNamed(t, path, name, function.Params)
		}
	case *ast.FuncType:
		assertParametersAreNamed(t, path, specification.Name.Name, declaration.Params)
	}
}

func assertParametersAreNamed(t *testing.T, path, callable string, parameters *ast.FieldList) {
	t.Helper()
	if parameters == nil {
		return
	}
	for _, parameter := range parameters.List {
		if len(parameter.Names) == 0 {
			t.Errorf("%s: exported callable %s requires semantically named parameters", path, callable)
		}
	}
}

func assertGoDocStartsWithName(t *testing.T, path, name string, doc *ast.CommentGroup) {
	t.Helper()
	if doc == nil || !strings.HasPrefix(strings.TrimSpace(doc.Text()), name) {
		t.Errorf("%s: exported %s requires GoDoc beginning with its exact name", path, name)
	}
}

func TestExportedAPIBaseline(t *testing.T) {
	for _, test := range exportedAPIBaselines {
		t.Run(test.name, func(t *testing.T) {
			command := exec.CommandContext(t.Context(), "go", "doc", "-all", ".")
			command.Dir = test.directory
			command.Env = append(os.Environ(), "GOWORK=off")
			output, err := command.Output()
			if err != nil {
				t.Fatal(err)
			}
			got := fmt.Sprintf("%x", sha256.Sum256(output))
			if got != test.want {
				t.Fatalf(
					"exported API/GoDoc changed: got %s, want %s; audit the change and update the accepted baseline",
					got, test.want,
				)
			}
		})
	}
}

func TestAPIBaselineDocumentMatchesFrozenPublicContracts(t *testing.T) {
	document, err := os.ReadFile("doc/API_BASELINE.md")
	if err != nil {
		t.Fatal(err)
	}
	text := string(document)
	required := []string{
		fmt.Sprintf("> 状态：Baseline %d 已冻结", currentAPIBaseline),
		fmt.Sprintf("> 冻结日期：%s", currentAPIBaselineFrozenOn),
		fmt.Sprintf("Baseline %d 不是兼容承诺或发布版本。", currentAPIBaseline),
		fmt.Sprintf("Baseline %d public digest：", currentAPIBaseline),
		fmt.Sprintf("Baseline %d wire digest：", currentAPIBaseline),
		"`json.RawMessage` 使用任意合法 JSON value 合同",
		"`[]byte` 使用 null/base64 string 合同",
		"package DAG 继续禁止任何 `app/runtime` production import",
	}
	for _, baseline := range exportedAPIBaselines {
		required = append(required, fmt.Sprintf("- %s：`%s`", baseline.label, baseline.want))
	}
	for _, contract := range required {
		if !strings.Contains(text, contract) {
			t.Errorf("API baseline document is missing current contract %q", contract)
		}
	}
}

func TestErrorCausesAreWrapped(t *testing.T) {
	for _, path := range frameworkProductionGoFiles(t) {
		assertErrorCausesWrappedInFile(t, path)
	}
}

func assertErrorCausesWrappedInFile(t *testing.T, path string) {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	ast.Inspect(file, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok || len(call.Args) < 2 || !isFmtErrorf(call.Fun) {
			return true
		}
		format, ok := stringLiteral(call.Args[0])
		if !ok || !strings.Contains(format, "%v") {
			return true
		}
		assertErrorArgumentsWrapped(t, path, call.Args[1:])
		return true
	})
}

func assertErrorArgumentsWrapped(t *testing.T, path string, arguments []ast.Expr) {
	t.Helper()
	for _, argument := range arguments {
		identifier, ok := argument.(*ast.Ident)
		if ok && isErrorCauseName(identifier.Name) {
			t.Errorf(
				"%s: fmt.Errorf formats cause %s with %%v; preserve errors.Is/As with %%w",
				path, identifier.Name,
			)
		}
	}
}

func isFmtErrorf(expression ast.Expr) bool {
	selector, ok := expression.(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != "Errorf" {
		return false
	}
	packageName, ok := selector.X.(*ast.Ident)
	return ok && packageName.Name == "fmt"
}

func stringLiteral(expression ast.Expr) (string, bool) {
	literal, ok := expression.(*ast.BasicLit)
	if !ok || literal.Kind != token.STRING {
		return "", false
	}
	return strings.Trim(literal.Value, "`\""), true
}

func isErrorCauseName(name string) bool {
	return name == "err" || name == "cause" ||
		strings.HasSuffix(name, "Err") || strings.HasSuffix(name, "Error")
}

func TestSnapshotWireBaseline(t *testing.T) {
	shape := snapshotWireShape()
	got := fmt.Sprintf("%x", sha256.Sum256([]byte(shape)))
	const want = "41b91a73b202a4654f3e5248a5b01a56e313a5c5ff70d39af65655df667aebc6"
	if got != want {
		t.Fatalf("snapshot wire changed: got %s, want %s\n%s", got, want, shape)
	}
}

func TestWireBaselinesCoverEveryProductionWireType(t *testing.T) {
	covered := make(map[string]struct{})
	for _, wireType := range append(snapshotWireTypes(), observationWireTypes()...) {
		covered[wireType.Name()] = struct{}{}
	}
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, declaration := range file.Decls {
			general, ok := declaration.(*ast.GenDecl)
			if !ok {
				continue
			}
			for _, specification := range general.Specs {
				typeSpec, ok := specification.(*ast.TypeSpec)
				if !ok {
					continue
				}
				_, isStruct := typeSpec.Type.(*ast.StructType)
				baselineOwned := strings.HasSuffix(typeSpec.Name.Name, "Wire") ||
					strings.HasSuffix(typeSpec.Name.Name, "EventPayload")
				if !isStruct || !baselineOwned {
					continue
				}
				if _, found := covered[typeSpec.Name.Name]; !found {
					t.Errorf("%s: production wire type %s is absent from a wire baseline", name, typeSpec.Name.Name)
				}
			}
		}
	}
}

func TestObservationWireBaseline(t *testing.T) {
	shape := observationWireShape()
	got := fmt.Sprintf("%x", sha256.Sum256([]byte(shape)))
	const want = "a087b14412eddf40b5bddcc051a4a02f0620b150daa94977816882c80f3ccf1f"
	if got != want {
		t.Fatalf("observation wire changed: got %s, want %s\n%s", got, want, shape)
	}
}

func observationWireShape() string {
	types := observationWireTypes()
	slices.SortFunc(types, func(left, right reflect.Type) int {
		return strings.Compare(left.Name(), right.Name())
	})
	var shape strings.Builder
	for _, wireType := range types {
		fmt.Fprintf(&shape, "%s\n", wireType.Name())
		for field := range wireType.Fields() {
			fmt.Fprintf(
				&shape, "  %s %s json=%q\n",
				field.Name, field.Type.String(), field.Tag.Get("json"),
			)
		}
	}
	return shape.String()
}

func observationWireTypes() []reflect.Type {
	return []reflect.Type{
		reflect.TypeFor[deltaDroppedEventPayload](),
		reflect.TypeFor[deltaWire](),
		reflect.TypeFor[effectFinishedEventPayload](),
		reflect.TypeFor[effectStartedEventPayload](),
		reflect.TypeFor[eventWire](),
		reflect.TypeFor[processFinishedEventPayload](),
		reflect.TypeFor[signalAcceptedEventPayload](),
		reflect.TypeFor[stepCommittedEventPayload](),
		reflect.TypeFor[stepFinishedEventPayload](),
	}
}

func snapshotWireShape() string {
	types := snapshotWireTypes()
	slices.SortFunc(types, func(left, right reflect.Type) int {
		return strings.Compare(left.Name(), right.Name())
	})
	var shape strings.Builder
	fmt.Fprintf(
		&shape, "process=%d tree=%d child=%d framework_effect=%d\n",
		processSnapshotSchemaVersion, treeSnapshotSchemaVersion,
		childProtocolSchemaVersion, frameworkEffectSchemaVersion,
	)
	for _, wireType := range types {
		fmt.Fprintf(&shape, "%s\n", wireType.Name())
		for field := range wireType.Fields() {
			fmt.Fprintf(
				&shape, "  %s %s json=%q\n",
				field.Name, field.Type.String(), field.Tag.Get("json"),
			)
		}
	}
	return shape.String()
}

func snapshotWireTypes() []reflect.Type {
	return []reflect.Type{
		reflect.TypeFor[descriptorContractWire](),
		reflect.TypeFor[descriptorWire](),
		reflect.TypeFor[processSnapshotWire](),
		reflect.TypeFor[processRelationWire](),
		reflect.TypeFor[preparedStepWire](),
		reflect.TypeFor[preparedEffectWire](),
		reflect.TypeFor[pendingControlWire](),
		reflect.TypeFor[mailboxWire](),
		reflect.TypeFor[signalRecordWire](),
		reflect.TypeFor[waitRecordWire](),
		reflect.TypeFor[treeSnapshotWire](),
		reflect.TypeFor[childWaitSnapshotWire](),
		reflect.TypeFor[executionStateWire](),
		reflect.TypeFor[transitionWire](),
		reflect.TypeFor[effectWire](),
		reflect.TypeFor[settlementWire](),
		reflect.TypeFor[signalWire](),
		reflect.TypeFor[deploymentIdentityWire](),
		reflect.TypeFor[deploymentRefWire](),
		reflect.TypeFor[terminationWire](),
		reflect.TypeFor[failureWire](),
		reflect.TypeFor[childWaitConditionWire](),
		reflect.TypeFor[childWaitSpecWire](),
		reflect.TypeFor[childOutcomeWire](),
		reflect.TypeFor[childWaitEffectWire](),
		reflect.TypeFor[childWaitOpenedWire](),
		reflect.TypeFor[childrenCompletedWire](),
		reflect.TypeFor[childStartEffectWire](),
		reflect.TypeFor[childStartResultWire](),
		reflect.TypeFor[waitRequestWire](),
		reflect.TypeFor[resultWire](),
		reflect.TypeFor[Budget](),
		reflect.TypeFor[Limits](),
		reflect.TypeFor[TreeLimits](),
		reflect.TypeFor[Usage](),
	}
}

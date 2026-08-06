package agent2

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
	"slices"
	"strings"
	"testing"
)

func TestExportedContractsAreDocumentedAndNamed(t *testing.T) {
	directories := []string{".", "interaction", "planning", "planning/goap", "workflow", "otel", "platform"}
	for _, directory := range directories {
		entries, err := os.ReadDir(directory)
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			name := entry.Name()
			if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
				continue
			}
			path := filepath.Join(directory, name)
			file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ParseComments)
			if err != nil {
				t.Fatal(err)
			}
			for _, declaration := range file.Decls {
				switch declaration := declaration.(type) {
				case *ast.FuncDecl:
					publicReceiver := declaration.Recv == nil || token.IsExported(receiverTypeName(declaration.Recv))
					if declaration.Name.IsExported() && publicReceiver {
						assertGoDocStartsWithName(t, path, declaration.Name.Name, declaration.Doc)
						assertParametersAreNamed(t, path, declaration.Name.Name, declaration.Type.Params)
					}
				case *ast.GenDecl:
					for _, specification := range declaration.Specs {
						switch specification := specification.(type) {
						case *ast.TypeSpec:
							if specification.Name.IsExported() {
								doc := specification.Doc
								if doc == nil {
									doc = declaration.Doc
								}
								assertGoDocStartsWithName(t, path, specification.Name.Name, doc)
								assertExportedTypeContract(t, path, specification)
							}
						case *ast.ValueSpec:
							for _, identifier := range specification.Names {
								if !identifier.IsExported() {
									continue
								}
								doc := specification.Doc
								if doc == nil {
									doc = declaration.Doc
								}
								assertGoDocStartsWithName(t, path, identifier.Name, doc)
							}
						}
					}
				}
			}
		}
	}
}

func assertExportedTypeContract(t *testing.T, path string, specification *ast.TypeSpec) {
	t.Helper()
	switch declaration := specification.Type.(type) {
	case *ast.StructType:
		for _, field := range declaration.Fields.List {
			for _, name := range field.Names {
				if name.IsExported() {
					assertGoDocStartsWithName(t, path, name.Name, field.Doc)
				}
			}
		}
	case *ast.InterfaceType:
		for _, method := range declaration.Methods.List {
			function, ok := method.Type.(*ast.FuncType)
			if !ok {
				continue
			}
			name := specification.Name.Name
			if len(method.Names) > 0 {
				name += "." + method.Names[0].Name
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
	tests := []struct {
		name      string
		directory string
		want      string
	}{
		{name: "kernel", directory: ".", want: "19339ca04489469a3c06f2c8f9a763a65eaf47ad2b31a67e4eb402290c44394c"},
		{name: "interaction", directory: "interaction", want: "b5fdabbea94d2b9aa346446a7cafb4accde928b23489012ba2763c77a517da91"},
		{name: "planning", directory: "planning", want: "15c48c52b7d4765ba86da2e5fd11822669c163c01e98dd4cb3668f71f7c5f30a"},
		{name: "goap", directory: "planning/goap", want: "dd5a007a20ddbeac2112bbed10718f5256fe2449376fd7dcc1400e25578253ec"},
		{name: "workflow", directory: "workflow", want: "a0e4815dc7c9bb69f215702434b8547e2abdb446400efc445d3fdb35ff752094"},
		{name: "otel", directory: "otel", want: "0725fbef9fbd28ba9b6999ab8b427dd9a4376f83aef9839a9f15f60c16422016"},
		{name: "platform", directory: "platform", want: "748f5ea1ef3b09c702a792ab6e16a3b4ae6be9776ef1a4ba51e856757abff078"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			command := exec.CommandContext(t.Context(), "go", "doc", "-all", ".")
			command.Dir = test.directory
			command.Env = append(os.Environ(), "GOWORK=off")
			output, err := command.Output()
			if err != nil {
				t.Fatal(err)
			}
			assertGoDocHasNoUndocumentedCallables(t, output)
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

func assertGoDocHasNoUndocumentedCallables(t *testing.T, output []byte) {
	t.Helper()
	lines := strings.Split(string(output), "\n")
	for index, line := range lines {
		if !strings.HasPrefix(line, "func ") || index+1 >= len(lines) {
			continue
		}
		if lines[index+1] == "" {
			t.Errorf("go doc exposes an undocumented callable: %s", line)
		}
	}
}

func TestErrorCausesAreWrapped(t *testing.T) {
	directories := []string{".", "interaction", "planning", "planning/goap", "workflow", "otel", "platform"}
	for _, directory := range directories {
		entries, err := os.ReadDir(directory)
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			name := entry.Name()
			if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
				continue
			}
			path := filepath.Join(directory, name)
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
				for _, argument := range call.Args[1:] {
					identifier, ok := argument.(*ast.Ident)
					if ok && isErrorCauseName(identifier.Name) {
						t.Errorf("%s: fmt.Errorf formats cause %s with %%v; preserve errors.Is/As with %%w", path, identifier.Name)
					}
				}
				return true
			})
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
	const want = "0e245506ccda1ef6c1697a782a67e3d5c01e8417bc8d7ff8686eca138b5a43c5"
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
	const want = "4006d3d24440922ba1e1ba9616bde3a88352fcc2625d918b839e1de283b631cb"
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
		for index := range wireType.NumField() {
			field := wireType.Field(index)
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
		reflect.TypeOf(deltaDroppedEventPayload{}),
		reflect.TypeOf(deltaWire{}),
		reflect.TypeOf(effectFinishedEventPayload{}),
		reflect.TypeOf(effectStartedEventPayload{}),
		reflect.TypeOf(eventWire{}),
		reflect.TypeOf(processFinishedEventPayload{}),
		reflect.TypeOf(signalAcceptedEventPayload{}),
		reflect.TypeOf(stepCommittedEventPayload{}),
		reflect.TypeOf(stepFinishedEventPayload{}),
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
		for index := range wireType.NumField() {
			field := wireType.Field(index)
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
		reflect.TypeOf(descriptorContractWire{}),
		reflect.TypeOf(descriptorWire{}),
		reflect.TypeOf(processSnapshotWire{}),
		reflect.TypeOf(processRelationWire{}),
		reflect.TypeOf(preparedStepWire{}),
		reflect.TypeOf(preparedEffectWire{}),
		reflect.TypeOf(pendingControlWire{}),
		reflect.TypeOf(mailboxWire{}),
		reflect.TypeOf(signalRecordWire{}),
		reflect.TypeOf(waitRecordWire{}),
		reflect.TypeOf(treeSnapshotWire{}),
		reflect.TypeOf(childWaitSnapshotWire{}),
		reflect.TypeOf(executionStateWire{}),
		reflect.TypeOf(transitionWire{}),
		reflect.TypeOf(effectWire{}),
		reflect.TypeOf(settlementWire{}),
		reflect.TypeOf(signalWire{}),
		reflect.TypeOf(deploymentIdentityWire{}),
		reflect.TypeOf(deploymentRefWire{}),
		reflect.TypeOf(terminationWire{}),
		reflect.TypeOf(failureWire{}),
		reflect.TypeOf(childWaitConditionWire{}),
		reflect.TypeOf(childWaitSpecWire{}),
		reflect.TypeOf(childOutcomeWire{}),
		reflect.TypeOf(childWaitEffectWire{}),
		reflect.TypeOf(childWaitOpenedWire{}),
		reflect.TypeOf(childrenCompletedWire{}),
		reflect.TypeOf(childStartEffectWire{}),
		reflect.TypeOf(childStartResultWire{}),
		reflect.TypeOf(waitRequestWire{}),
		reflect.TypeOf(resultWire{}),
		reflect.TypeOf(Budget{}),
		reflect.TypeOf(Limits{}),
		reflect.TypeOf(TreeLimits{}),
		reflect.TypeOf(Usage{}),
	}
}

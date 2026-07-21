package arch

import (
	"bytes"
	"flag"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

var updateAgentAPI = flag.Bool("update-api", false, "replace the reviewed Agent exported API baseline")

const agentExportedAPIBaseline = "testdata/exported_api.txt"

// agentRootFacadeBudget caps the curated root-package surface so the facade
// stays a discoverable common-path entry point, not a mirror of every
// sub-package. Raised from 50 to cover the common tool-using-agent authoring
// path end to end (Chat, RequireToolGroup + ToolGroup* declaration/resolution,
// Get, RequireType) so a typical agent needs only the `agent` import; advanced
// SPI (tool-group permissions, Objects/Last) deliberately stays in core.
const agentRootFacadeBudget = 58

// agentPublicPackages is the complete non-internal, non-example surface of the
// Agent module. One module has one SemVer contract: planners, workflows,
// policies, and routing are just as externally nameable as runtime/core and
// therefore cannot sit outside the release baseline.
var agentPublicPackages = map[string]struct{}{
	".":                 {},
	"core":              {},
	"event":             {},
	"hitl":              {},
	"interaction":       {},
	"planning":          {},
	"planning/goap":     {},
	"planning/htn":      {},
	"planning/reactive": {},
	"planning/utility":  {},
	"runtime":           {},
	"routing":           {},
	"storetest":         {},
	"toolloop":          {},
	"toolpolicy":        {},
	"workflow":          {},
}

// TestAllPublicPackagesAreAPIGuarded makes package discovery part of the
// release contract. Adding a new public package without explicitly deciding to
// freeze it is a failure, as is retaining a baseline entry after a package is
// removed.
func TestAllPublicPackagesAreAPIGuarded(t *testing.T) {
	root := moduleRoot(t)
	discovered := make(map[string]struct{})
	for _, path := range productionGoFiles(t) {
		dir, err := filepath.Rel(root, filepath.Dir(path))
		if err != nil {
			t.Fatalf("make %s relative to Agent: %v", path, err)
		}
		packagePath := filepath.ToSlash(dir)
		if packagePath == "internal" || strings.HasPrefix(packagePath, "internal/") {
			continue
		}
		discovered[packagePath] = struct{}{}
	}
	for packagePath := range discovered {
		if _, guarded := agentPublicPackages[packagePath]; !guarded {
			t.Errorf("public package %q is not covered by the Agent API baseline", packagePath)
		}
	}
	for packagePath := range agentPublicPackages {
		if _, exists := discovered[packagePath]; !exists {
			t.Errorf("Agent API baseline lists missing public package %q", packagePath)
		}
	}
}

// TestExportedAPIMatchesBaseline is the release guard for the Agent
// framework's complete public Go surface. Any difference must be reviewed as an
// API decision and accepted by updating the checked-in baseline with:
//
//	go test ./internal/arch -run TestExportedAPIMatchesBaseline -update-api
//
// The snapshot keeps complete const/var groups when one member is exported so
// inferred types and iota ordering cannot change silently. Function bodies and
// comments are deliberately omitted.
func TestExportedAPIMatchesBaseline(t *testing.T) {
	got := agentExportedAPISnapshot(t)
	path := filepath.Join(moduleRoot(t), "internal", "arch", agentExportedAPIBaseline)
	if *updateAgentAPI {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("create Agent API baseline directory: %v", err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatalf("write Agent API baseline: %v", err)
		}
		return
	}

	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read Agent API baseline: %v", err)
	}
	if got == string(want) {
		return
	}
	t.Fatalf("Agent exported API changed without a reviewed baseline update:\n%s\nreview the diff, update migration/release notes, then run: go test ./internal/arch -run TestExportedAPIMatchesBaseline -update-api", agentAPIDelta(string(want), got))
}

// TestRootFacadeBudget keeps the convenience package useful without letting it
// become a mirror of core/runtime. The required set is the reviewed standard
// definition + lifecycle path; advanced protocols stay in their owner package.
func TestRootFacadeBudget(t *testing.T) {
	snapshot := agentExportedAPISnapshot(t)
	declarations := 0
	for line := range strings.SplitSeq(snapshot, "\n") {
		if strings.HasPrefix(line, ".: ") {
			declarations++
		}
	}
	if declarations > agentRootFacadeBudget {
		t.Fatalf("root agent façade has %d exported declarations, budget is %d", declarations, agentRootFacadeBudget)
	}

	names := rootExportedNames(t)
	for _, required := range []string{
		"Action", "ActionConfig", "Agent", "Process", "DeploymentRef",
		"Deployment", "Engine", "EngineConfig", "Goal", "MustNewEngine", "New", "NewAction",
		"NewEngine", "ProcessContext", "PromptConfig", "PromptJSON", "ChatCapability",
		"ProcessOptions", "Result",
	} {
		if _, ok := names[required]; !ok {
			t.Errorf("root agent façade is missing standard-path symbol %s", required)
		}
	}
	for _, forbidden := range []string{
		"ActionQoS", "ChatClientProvider", "AgentProcess", "AgentProcessStatus",
		"AgentRef", "ComputedCondition", "Determination", "GoalExport", "IOBinding",
		"Platform", "PlatformConfig", "ProcessType", "ServiceProvider", "Config",
	} {
		if _, ok := names[forbidden]; ok {
			t.Errorf("removed or overly broad symbol %s returned to root agent façade", forbidden)
		}
	}
}

func TestAPISnapshotOmitsPrivateRepresentation(t *testing.T) {
	snapshot := agentExportedAPISnapshot(t)
	for _, leaked := range []string{
		"core: type Agent struct { config",
		"core: type Goal struct { name",
		"planning: type Plan struct { actions",
		"planning: type Domain struct { actions",
		"routing: type Candidate struct { deployment",
	} {
		if strings.Contains(snapshot, leaked) {
			t.Errorf("exported API baseline includes private representation %q", leaked)
		}
	}
}

func rootExportedNames(t *testing.T) map[string]struct{} {
	t.Helper()
	root := moduleRoot(t)
	fset := token.NewFileSet()
	names := make(map[string]struct{})
	for _, path := range productionGoFiles(t) {
		if filepath.Dir(path) != root {
			continue
		}
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("parse root façade file %s: %v", path, err)
		}
		for _, declaration := range file.Decls {
			switch typed := declaration.(type) {
			case *ast.FuncDecl:
				if typed.Recv == nil && ast.IsExported(typed.Name.Name) {
					names[typed.Name.Name] = struct{}{}
				}
			case *ast.GenDecl:
				for _, specification := range typed.Specs {
					switch spec := specification.(type) {
					case *ast.TypeSpec:
						if ast.IsExported(spec.Name.Name) {
							names[spec.Name.Name] = struct{}{}
						}
					case *ast.ValueSpec:
						for _, name := range spec.Names {
							if ast.IsExported(name.Name) {
								names[name.Name] = struct{}{}
							}
						}
					}
				}
			}
		}
	}
	return names
}

func agentExportedAPISnapshot(t *testing.T) string {
	t.Helper()
	root := moduleRoot(t)
	fset := token.NewFileSet()
	var entries []string

	for _, path := range productionGoFiles(t) {
		dir, err := filepath.Rel(root, filepath.Dir(path))
		if err != nil {
			t.Fatalf("make %s relative to Agent: %v", path, err)
		}
		packagePath := filepath.ToSlash(dir)
		if _, public := agentPublicPackages[packagePath]; !public {
			continue
		}

		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s for API snapshot: %v", path, err)
		}
		for _, declaration := range file.Decls {
			for _, item := range agentPublicDeclarations(declaration) {
				entries = append(entries, packagePath+": "+agentCanonicalDeclaration(t, fset, item))
			}
		}
	}

	sort.Strings(entries)
	return "# Generated Agent exported API baseline. Review every diff; do not edit by hand.\n" +
		"# Packages: every non-internal, non-example package in the agent module.\n" +
		"# Regenerate: go test ./internal/arch -run TestExportedAPIMatchesBaseline -update-api\n\n" +
		strings.Join(entries, "\n") + "\n"
}

func agentPublicDeclarations(declaration ast.Decl) []ast.Node {
	switch typed := declaration.(type) {
	case *ast.FuncDecl:
		if !ast.IsExported(typed.Name.Name) {
			return nil
		}
		if typed.Recv != nil && !agentReceiverIsExported(typed.Recv) {
			return nil
		}
		copy := *typed
		copy.Doc = nil
		copy.Body = nil
		return []ast.Node{&copy}
	case *ast.GenDecl:
		switch typed.Tok {
		case token.TYPE:
			var nodes []ast.Node
			for _, specification := range typed.Specs {
				typeSpec := specification.(*ast.TypeSpec)
				if !ast.IsExported(typeSpec.Name.Name) {
					continue
				}
				copy := *typeSpec
				copy.Doc = nil
				copy.Comment = nil
				copy.Type = agentPublicType(typeSpec.Type)
				nodes = append(nodes, &ast.GenDecl{Tok: token.TYPE, Specs: []ast.Spec{&copy}})
			}
			return nodes
		case token.CONST, token.VAR:
			if !agentGeneralDeclarationExportsName(typed) {
				return nil
			}
			copy := &ast.GenDecl{Tok: typed.Tok, Lparen: typed.Lparen}
			for _, specification := range typed.Specs {
				valueSpec := specification.(*ast.ValueSpec)
				valueCopy := *valueSpec
				valueCopy.Doc = nil
				valueCopy.Comment = nil
				copy.Specs = append(copy.Specs, &valueCopy)
			}
			return []ast.Node{copy}
		}
	}
	return nil
}

// agentPublicType removes unexported fields from exported struct snapshots.
// Private representation is intentionally free to evolve; only fields another
// package can name belong in the SemVer contract.
func agentPublicType(expression ast.Expr) ast.Expr {
	switch typed := expression.(type) {
	case *ast.StructType:
		cloned := *typed
		fields := &ast.FieldList{}
		if typed.Fields != nil {
			fields.Opening = typed.Fields.Opening
			fields.Closing = typed.Fields.Closing
			for _, field := range typed.Fields.List {
				fieldCopy := *field
				if len(field.Names) == 0 {
					name, ok := agentReceiverBaseName(field.Type)
					if !ok || !ast.IsExported(name) {
						continue
					}
				} else {
					fieldCopy.Names = nil
					for _, name := range field.Names {
						if ast.IsExported(name.Name) {
							fieldCopy.Names = append(fieldCopy.Names, name)
						}
					}
					if len(fieldCopy.Names) == 0 {
						continue
					}
				}
				fieldCopy.Type = agentPublicType(field.Type)
				fields.List = append(fields.List, &fieldCopy)
			}
		}
		cloned.Fields = fields
		return &cloned
	case *ast.ArrayType:
		cloned := *typed
		cloned.Elt = agentPublicType(typed.Elt)
		return &cloned
	case *ast.MapType:
		cloned := *typed
		cloned.Key = agentPublicType(typed.Key)
		cloned.Value = agentPublicType(typed.Value)
		return &cloned
	case *ast.StarExpr:
		cloned := *typed
		cloned.X = agentPublicType(typed.X)
		return &cloned
	case *ast.ChanType:
		cloned := *typed
		cloned.Value = agentPublicType(typed.Value)
		return &cloned
	default:
		return expression
	}
}

// agentReceiverIsExported reports whether a method's receiver type can be
// named by another package. Exported methods on implementation-only receiver
// types satisfy internal interfaces, but they are not package API and must not
// make an internal refactor look like a public API change.
func agentReceiverIsExported(receiver *ast.FieldList) bool {
	if receiver == nil || len(receiver.List) != 1 {
		return false
	}
	name, ok := agentReceiverBaseName(receiver.List[0].Type)
	return ok && ast.IsExported(name)
}

func agentReceiverBaseName(expression ast.Expr) (string, bool) {
	switch typed := expression.(type) {
	case *ast.Ident:
		return typed.Name, true
	case *ast.StarExpr:
		return agentReceiverBaseName(typed.X)
	case *ast.IndexExpr:
		return agentReceiverBaseName(typed.X)
	case *ast.IndexListExpr:
		return agentReceiverBaseName(typed.X)
	case *ast.ParenExpr:
		return agentReceiverBaseName(typed.X)
	case *ast.SelectorExpr:
		return typed.Sel.Name, true
	default:
		return "", false
	}
}

func agentGeneralDeclarationExportsName(declaration *ast.GenDecl) bool {
	for _, specification := range declaration.Specs {
		for _, name := range specification.(*ast.ValueSpec).Names {
			if ast.IsExported(name.Name) {
				return true
			}
		}
	}
	return false
}

func agentCanonicalDeclaration(t *testing.T, fset *token.FileSet, declaration ast.Node) string {
	t.Helper()
	var output bytes.Buffer
	if err := format.Node(&output, fset, declaration); err != nil {
		t.Fatalf("format Agent API declaration: %v", err)
	}
	return strings.Join(strings.Fields(output.String()), " ")
}

func agentAPIDelta(want, got string) string {
	wantLines := agentLineCounts(want)
	gotLines := agentLineCounts(got)
	var removed, added []string
	for line, count := range wantLines {
		for range max(0, count-gotLines[line]) {
			removed = append(removed, "- "+line)
		}
	}
	for line, count := range gotLines {
		for range max(0, count-wantLines[line]) {
			added = append(added, "+ "+line)
		}
	}
	sort.Strings(removed)
	sort.Strings(added)
	changes := append(removed, added...)
	const limit = 80
	if len(changes) > limit {
		changes = append(changes[:limit], fmt.Sprintf("... %d additional changed lines", len(changes)-limit))
	}
	return strings.Join(changes, "\n")
}

func agentLineCounts(value string) map[string]int {
	counts := make(map[string]int)
	for _, line := range strings.Split(strings.TrimSpace(value), "\n") {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		counts[line]++
	}
	return counts
}

func productionGoFiles(t *testing.T) []string {
	t.Helper()
	root := moduleRoot(t)
	var files []string
	if err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			name := entry.Name()
			if name == "vendor" || name == "examples" || (strings.HasPrefix(name, ".") && path != root) {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go") {
			files = append(files, path)
		}
		return nil
	}); err != nil {
		t.Fatalf("walk Agent production files: %v", err)
	}
	sort.Strings(files)
	return files
}

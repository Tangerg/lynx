// Package arch holds the agent module's architecture-fitness tests. It contains
// no production code — only tests that mechanically enforce structural
// invariants the compiler can't, so the framework's layering can't quietly rot
// during refactors.
//
// agent is an embeddable FRAMEWORK MODULE, not an application, so this is NOT a
// Clean-Architecture concentric-ring rule (delivery/use-case/domain/infra). Its
// internal shape is a dependency LADDER: framework kernel → strategy plug-ins →
// engine → combinators. The rule below encodes that ladder. See docs/README.md,
// docs/EXTENSION_DESIGN.md, and the root Agent Framework execution plan.
package arch

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var forbiddenFrameworkModulePrefixes = []string{
	"github.com/Tangerg/lynx/app",
	"github.com/Tangerg/lynx/a2a",
	"github.com/Tangerg/lynx/mcp",
	"github.com/a2aproject/a2a-go",
	"github.com/modelcontextprotocol/go-sdk",
}

// TestDependencyRule enforces the framework's dependency ladder: an inner rung
// must not import an outer one.
//
// Rungs (inner → outer):
//
//	core         core/, interaction/        pure primitives + public SPI (Action/Goal/Condition/Blackboard/Extension)
//	strategy     planning/, event/,             strategy/protocol plug-ins that depend on core
//	             hitl/, toolloop/
//	engine       runtime/, routing/              state machine and dispatch; consumes core + strategy
//	combinator   ./, workflow/                    public convenience and high-level combinators
//
// Forbidden edges (an inner rung learning about an outer one):
//
//	core       ↛ strategy, engine, combinator   primitives depend on nothing above them
//	strategy   ↛ engine, combinator             a plug-in must not reach the engine
//	engine     ↛ combinator                     the engine must not depend on the combinators built atop it
//
// Intentionally allowed (correct ladder edges, and the documented preference
// for concrete internal dependencies — agent/CLAUDE.md): strategy → core; engine → core + strategy;
// combinator → core + engine (workflow holds *runtime.Engine by concrete type,
// no SubprocessSpawner interface — that would be a YAGNI ceremony). event → planning
// is a same-rung edge (event types describe planning), so it is not forbidden.
func TestDependencyRule(t *testing.T) {
	const modulePath = "github.com/Tangerg/lynx/agent"
	root := moduleRoot(t)
	fset := token.NewFileSet()

	violations := 0
	walkErr := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			// examples/ is demo code, not on the production path (agent/CLAUDE.md);
			// skip vendored + hidden dirs too.
			if name == "vendor" || name == "examples" || (strings.HasPrefix(name, ".") && path != root) {
				return filepath.SkipDir
			}
			return nil
		}
		// Test files may import across rungs (stubs, fixtures); only production
		// dependencies are constrained.
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		rel, err := filepath.Rel(root, filepath.Dir(path))
		if err != nil {
			return err
		}
		from := rungOf(filepath.ToSlash(rel))
		if from == "" {
			return nil // unclassified (module root / examples) — unconstrained
		}

		f, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, imp := range f.Imports {
			ip := strings.Trim(imp.Path.Value, `"`)
			rest, ok := moduleImportRel(ip, modulePath)
			if !ok {
				continue
			}
			to := rungOf(rest)
			if to != "" && forbidden(from, to) {
				violations++
				t.Errorf("dependency-rule violation: %s (%s) imports %s (%s)", rel, from, rest, to)
			}
		}
		return nil
	})
	if walkErr != nil {
		t.Fatalf("walk module: %v", walkErr)
	}
	if violations == 0 {
		t.Log("dependency ladder holds: no inner rung imports an outer one")
	}
}

// TestEveryPublicProductionPackageIsClassified prevents a new package from
// silently bypassing the dependency ladder. Internal test infrastructure and
// examples are outside the published framework DAG; every other production Go
// package must occupy a reviewed rung.
func TestEveryPublicProductionPackageIsClassified(t *testing.T) {
	root := moduleRoot(t)
	seen := make(map[string]struct{})
	walkErr := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			if name == "vendor" || name == "examples" || name == "internal" || (strings.HasPrefix(name, ".") && path != root) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel, err := filepath.Rel(root, filepath.Dir(path))
		if err != nil {
			return err
		}
		packagePath := filepath.ToSlash(rel)
		if _, checked := seen[packagePath]; checked {
			return nil
		}
		seen[packagePath] = struct{}{}
		if rungOf(packagePath) == "" {
			t.Errorf("public production package %q is not classified in the Agent dependency ladder", packagePath)
		}
		return nil
	})
	if walkErr != nil {
		t.Fatalf("walk public Agent packages: %v", walkErr)
	}
}

func TestFrameworkDoesNotImportApplicationOrTransportModules(t *testing.T) {
	root := moduleRoot(t)
	fset := token.NewFileSet()
	walkErr := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			if name == "vendor" || name == "examples" || (strings.HasPrefix(name, ".") && path != root) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, imported := range file.Imports {
			importPath := strings.Trim(imported.Path.Value, `"`)
			for _, prefix := range forbiddenFrameworkModulePrefixes {
				if hasModulePrefix(importPath, prefix) {
					rel, _ := filepath.Rel(root, path)
					t.Errorf("Agent Framework production package imports application or transport module %q: %s", importPath, rel)
				}
			}
		}
		return nil
	})
	if walkErr != nil {
		t.Fatalf("walk Agent Framework: %v", walkErr)
	}
}

func TestFrameworkModuleDoesNotRequireApplicationOrTransportModules(t *testing.T) {
	path := filepath.Join(moduleRoot(t), "go.mod")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read Agent go.mod: %v", err)
	}
	for _, field := range strings.Fields(string(data)) {
		for _, prefix := range forbiddenFrameworkModulePrefixes {
			if hasModulePrefix(field, prefix) {
				t.Errorf("Agent module depends on application or transport module %q", field)
			}
		}
	}
}

func hasModulePrefix(path, prefix string) bool {
	return path == prefix || strings.HasPrefix(path, prefix+"/")
}

func TestFrameworkDoesNotImportStorageBackends(t *testing.T) {
	forbiddenPrefixes := []string{
		"database/sql",
		"github.com/Tangerg/lynx/chathistory",
		"github.com/Tangerg/lynx/vectorstores",
		"github.com/jackc/pgx",
		"github.com/mattn/go-sqlite3",
		"github.com/redis/go-redis",
		"go.mongodb.org/mongo-driver",
		"gorm.io",
		"modernc.org/sqlite",
	}
	root := moduleRoot(t)
	fset := token.NewFileSet()
	walkErr := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
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
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, imported := range file.Imports {
			importPath := strings.Trim(imported.Path.Value, `"`)
			for _, prefix := range forbiddenPrefixes {
				if importPath == prefix || strings.HasPrefix(importPath, prefix+"/") {
					relativePath, _ := filepath.Rel(root, path)
					t.Errorf("Agent Framework production package imports storage backend %q: %s", importPath, relativePath)
				}
			}
		}
		return nil
	})
	if walkErr != nil {
		t.Fatalf("walk Agent Framework: %v", walkErr)
	}
}

// TestRuntimeExportsNoHostTransactionProtocol prevents the concrete regression
// removed by P25: a framework plan must not grow settlement or persistence
// methods that ask a consumer to coordinate external state while runtime keeps
// ownership. The exported API golden remains the review gate for other changes;
// this test intentionally avoids a broad vocabulary denylist.
func TestRuntimeExportsNoHostTransactionProtocol(t *testing.T) {
	root := filepath.Join(moduleRoot(t), "runtime")
	fset := token.NewFileSet()
	removedDeclarations := map[string]struct{}{
		"PreparedWaitingSubtreeCancellation": {},
		"PrepareWaitingSubtreeCancellation":  {},
	}
	forbiddenPlanMethods := map[string]struct{}{
		"Prepare":           {},
		"Commit":            {},
		"Abort":             {},
		"Persist":           {},
		"PersistCheckpoint": {},
	}
	walkErr := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return err
		}
		for _, declaration := range file.Decls {
			switch value := declaration.(type) {
			case *ast.FuncDecl:
				if !value.Name.IsExported() {
					continue
				}
				if _, removed := removedDeclarations[value.Name.Name]; removed {
					t.Errorf("Agent runtime restores removed host transaction operation %q: %s", value.Name.Name, filepath.Base(path))
				}
				if receiverTypeName(value) == "WaitingSubtreeCancellationPlan" {
					if _, forbidden := forbiddenPlanMethods[value.Name.Name]; forbidden {
						t.Errorf("Agent runtime transition plan exports host settlement method %q: %s", value.Name.Name, filepath.Base(path))
					}
				}
			case *ast.GenDecl:
				for _, spec := range value.Specs {
					typeSpec, ok := spec.(*ast.TypeSpec)
					if !ok || !typeSpec.Name.IsExported() {
						continue
					}
					if _, removed := removedDeclarations[typeSpec.Name.Name]; removed {
						t.Errorf("Agent runtime restores removed host transaction type %q: %s", typeSpec.Name.Name, filepath.Base(path))
					}
				}
			}
		}
		return nil
	})
	if walkErr != nil {
		t.Fatalf("walk Agent runtime: %v", walkErr)
	}
}

func TestRuntimeNamedJSONStructsAreExecutionState(t *testing.T) {
	allowed := map[string]struct{}{
		"canonicalAction":      {},
		"canonicalBinding":     {},
		"canonicalCondition":   {},
		"canonicalDefinition":  {},
		"canonicalGoal":        {},
		"nestedChildRelation":  {},
		"suspensionCheckpoint": {},
	}
	seen := make(map[string]struct{}, len(allowed))
	root := filepath.Join(moduleRoot(t), "runtime")
	fset := token.NewFileSet()
	walkErr := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return err
		}
		for _, declaration := range file.Decls {
			general, ok := declaration.(*ast.GenDecl)
			if !ok || general.Tok != token.TYPE {
				continue
			}
			for _, specification := range general.Specs {
				typeSpec := specification.(*ast.TypeSpec)
				structure, ok := typeSpec.Type.(*ast.StructType)
				if !ok || !agentHasJSONTag(structure) {
					continue
				}
				if _, reviewed := allowed[typeSpec.Name.Name]; !reviewed {
					rel, _ := filepath.Rel(root, path)
					t.Errorf("runtime named JSON struct %q is not framework execution state: %s", typeSpec.Name.Name, rel)
					continue
				}
				seen[typeSpec.Name.Name] = struct{}{}
			}
		}
		return nil
	})
	if walkErr != nil {
		t.Fatalf("walk Agent runtime JSON structs: %v", walkErr)
	}
	for name := range allowed {
		if _, present := seen[name]; !present {
			t.Errorf("runtime JSON struct allowlist contains stale entry %q", name)
		}
	}
}

func TestRuntimeEventsStayInMemory(t *testing.T) {
	fset := token.NewFileSet()
	for _, packageName := range []string{"event", "interaction", "toolloop"} {
		root := filepath.Join(moduleRoot(t), packageName)
		walkErr := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			file, err := parser.ParseFile(fset, path, nil, 0)
			if err != nil {
				return err
			}
			if packageName == "event" {
				for _, imported := range file.Imports {
					if imported.Path.Value == `"encoding/json"` {
						t.Errorf("Agent lifecycle events own an external JSON projection: %s", filepath.Base(path))
					}
				}
			}
			for _, declaration := range file.Decls {
				function, ok := declaration.(*ast.FuncDecl)
				if !ok || (function.Name.Name != "MarshalJSON" && function.Name.Name != "UnmarshalJSON") {
					continue
				}
				if packageName == "event" || receiverTypeName(function) == "Event" {
					t.Errorf("Agent runtime events own JSON method %s: %s", function.Name.Name, filepath.Base(path))
				}
			}
			return nil
		})
		if walkErr != nil {
			t.Fatalf("walk Agent %s events: %v", packageName, walkErr)
		}
	}
}

func receiverTypeName(function *ast.FuncDecl) string {
	if function == nil || function.Recv == nil || len(function.Recv.List) != 1 {
		return ""
	}
	receiver := function.Recv.List[0].Type
	if pointer, ok := receiver.(*ast.StarExpr); ok {
		receiver = pointer.X
	}
	if identifier, ok := receiver.(*ast.Ident); ok {
		return identifier.Name
	}
	return ""
}

func TestToolLoopDoesNotImportLegacyProtocol(t *testing.T) {
	root := filepath.Join(moduleRoot(t), "toolloop")
	fset := token.NewFileSet()
	walkErr := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, imported := range file.Imports {
			importPath := strings.Trim(imported.Path.Value, `"`)
			if importPath == "github.com/Tangerg/lynx/chatclient" ||
				strings.HasPrefix(importPath, "github.com/Tangerg/lynx/chatclient/") {
				rel, _ := filepath.Rel(root, path)
				t.Errorf("tool-loop file %s imports frozen runtime %q", rel, importPath)
			}
		}
		return nil
	})
	if walkErr != nil {
		t.Fatalf("walk tool-loop package: %v", walkErr)
	}
}

const (
	rungCore       = "core"
	rungStrategy   = "strategy"
	rungEngine     = "engine"
	rungCombinator = "combinator"
)

// rungOf classifies a module-relative package dir (e.g. "planning/goap")
// into its rung, or "" when the path is outside the rungs under test.
func rungOf(rel string) string {
	if rel == "." {
		return rungCombinator
	}
	first, _, _ := strings.Cut(rel, "/")
	switch first {
	case "core", "interaction":
		return rungCore
	case "planning", "event", "hitl", "toolloop":
		return rungStrategy
	case "runtime", "routing":
		return rungEngine
	case "workflow":
		return rungCombinator
	default:
		return ""
	}
}

func moduleImportRel(importPath, modulePath string) (string, bool) {
	if importPath == modulePath {
		return ".", true
	}
	return strings.CutPrefix(importPath, modulePath+"/")
}

// forbidden reports whether a package on rung "from" may NOT import one on "to".
func forbidden(from, to string) bool {
	switch from {
	case rungCore:
		return to == rungStrategy || to == rungEngine || to == rungCombinator
	case rungStrategy:
		return to == rungEngine || to == rungCombinator
	case rungEngine:
		return to == rungCombinator
	default:
		return false
	}
}

// moduleRoot walks up from the test's working directory to the directory
// holding go.mod (the agent module root).
func moduleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found walking up from test dir")
		}
		dir = parent
	}
}

// TestFrameworkWritesNoModelFacingCopy keeps product wording out of the
// framework. It rejects framework-owned literals at the two actual model-facing
// boundaries: chat message construction and definition descriptions. It does
// not guess from identifier names or generic multi-line strings.
func TestFrameworkWritesNoModelFacingCopy(t *testing.T) {
	messageConstructors := map[string]struct{}{
		"NewSystemMessage":    {},
		"NewUserMessage":      {},
		"NewAssistantMessage": {},
		"NewTextPart":         {},
	}
	descriptionOwners := map[string]struct{}{
		"ActionConfig":   {},
		"AgentConfig":    {},
		"GoalConfig":     {},
		"ToolDefinition": {},
	}
	root := moduleRoot(t)
	fset := token.NewFileSet()
	walkErr := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
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
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return err
		}
		relativePath, _ := filepath.Rel(root, path)
		ast.Inspect(file, func(node ast.Node) bool {
			switch value := node.(type) {
			case *ast.CompositeLit:
				if _, guarded := descriptionOwners[compositeTypeName(value.Type)]; !guarded {
					return true
				}
				for _, element := range value.Elts {
					field, ok := element.(*ast.KeyValueExpr)
					if !ok {
						continue
					}
					key, ok := field.Key.(*ast.Ident)
					if ok && key.Name == "Description" && containsStringLiteral(field.Value) {
						t.Errorf("Agent production code owns definition description text: %s (%s)", relativePath, compositeTypeName(value.Type))
					}
				}
			case *ast.CallExpr:
				selector, ok := value.Fun.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				if _, ok := messageConstructors[selector.Sel.Name]; !ok {
					return true
				}
				for _, argument := range value.Args {
					if containsStringLiteral(argument) {
						t.Errorf("Agent production code builds a model message from its own text: %s (%s)", relativePath, selector.Sel.Name)
					}
				}
			}
			return true
		})
		return nil
	})
	if walkErr != nil {
		t.Fatalf("walk Agent production code: %v", walkErr)
	}
}

func compositeTypeName(expression ast.Expr) string {
	switch value := expression.(type) {
	case *ast.Ident:
		return value.Name
	case *ast.SelectorExpr:
		return value.Sel.Name
	case *ast.IndexExpr:
		return compositeTypeName(value.X)
	case *ast.IndexListExpr:
		return compositeTypeName(value.X)
	default:
		return ""
	}
}

// containsStringLiteral reports whether expression is a string literal or a
// concatenation containing one — the shape a prompt takes when it is spliced
// into caller text.
func containsStringLiteral(expression ast.Expr) bool {
	switch value := expression.(type) {
	case *ast.BasicLit:
		return value.Kind == token.STRING
	case *ast.BinaryExpr:
		return containsStringLiteral(value.X) || containsStringLiteral(value.Y)
	case *ast.ParenExpr:
		return containsStringLiteral(value.X)
	default:
		return false
	}
}

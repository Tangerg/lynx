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
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestDependencyRule enforces the framework's dependency ladder: an inner rung
// must not import an outer one.
//
// Rungs (inner → outer):
//
//	core         core/, interaction/        pure primitives + public SPI (Action/Goal/Condition/Blackboard/Extension)
//	strategy     planning/, event/,             strategy/protocol plug-ins that depend on core
//	             hitl/, toolpolicy/, toolloop/
//	engine       runtime/, routing/              state machine and dispatch; consumes core + strategy
//	combinator   ./, workflow/, storetest/       public convenience, high-level combinators,
//	                                              and reusable store conformance tests
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

func TestAgentDoesNotImportApplicationModules(t *testing.T) {
	const appPrefix = "github.com/Tangerg/lynx/app/"
	root := moduleRoot(t)
	fset := token.NewFileSet()

	violations := 0
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

		f, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, imp := range f.Imports {
			ip := strings.Trim(imp.Path.Value, `"`)
			if strings.HasPrefix(ip, appPrefix) {
				violations++
				rel, _ := filepath.Rel(root, path)
				t.Errorf("agent must not import application module %q: %s", ip, rel)
			}
		}
		return nil
	})
	if walkErr != nil {
		t.Fatalf("walk agent: %v", walkErr)
	}
	if violations == 0 {
		t.Log("agent import boundary holds: no app/* imports found")
	}
}

func TestFrameworkDoesNotImportTransportSDKs(t *testing.T) {
	forbiddenPrefixes := []string{
		"github.com/Tangerg/lynx/a2a",
		"github.com/Tangerg/lynx/mcp",
		"github.com/a2aproject/a2a-go",
		"github.com/modelcontextprotocol/go-sdk",
	}
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
			for _, prefix := range forbiddenPrefixes {
				if strings.HasPrefix(importPath, prefix) {
					rel, _ := filepath.Rel(root, path)
					t.Errorf("Agent Framework production package imports transport SDK %q: %s", importPath, rel)
				}
			}
		}
		return nil
	})
	if walkErr != nil {
		t.Fatalf("walk Agent Framework: %v", walkErr)
	}
}

func TestFrameworkDoesNotImportStorageBackends(t *testing.T) {
	forbiddenPrefixes := []string{
		"database/sql",
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

func TestStoretestIsOnlyImportedByTests(t *testing.T) {
	const storetestPath = "github.com/Tangerg/lynx/agent/storetest"
	root := moduleRoot(t)
	fset := token.NewFileSet()
	walkErr := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if entry.Name() == "vendor" || strings.HasPrefix(entry.Name(), ".") && path != root {
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
			if strings.Trim(imported.Path.Value, `"`) == storetestPath {
				rel, _ := filepath.Rel(root, path)
				t.Errorf("test support package storetest imported by production file %s", rel)
			}
		}
		return nil
	})
	if walkErr != nil {
		t.Fatalf("walk Agent Framework: %v", walkErr)
	}
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
	case "planning", "event", "hitl", "toolpolicy", "toolloop":
		return rungStrategy
	case "runtime", "routing":
		return rungEngine
	case "workflow", "storetest":
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

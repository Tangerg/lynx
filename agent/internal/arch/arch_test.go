// Package arch holds the agent module's architecture-fitness tests. It contains
// no production code — only tests that mechanically enforce structural
// invariants the compiler can't, so the library's layering can't quietly rot
// during refactors.
//
// agent is a LIBRARY, not an application, so this is NOT a Clean-Architecture
// concentric-ring rule (delivery/use-case/domain/infra). The natural shape of a
// library is a dependency LADDER: primitives → strategy plug-ins → engine →
// combinators. The rule below encodes that ladder. See docs/README.md and
// docs/EXTENSION_DESIGN.md for the maintained architecture description.
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

// TestDependencyRule enforces the library's dependency ladder: an inner rung
// must not import an outer one.
//
// Rungs (inner → outer):
//
//	core         core/                     pure primitives + public SPI (Action/Goal/Condition/Blackboard/Extension)
//	strategy     planning/, event/,        策略/原语 plug-ins that depend on core but never on the engine
//	             hitl/, toolpolicy/, toolloop/
//	engine       runtime/, runtime/autonomy/   the state machine + dispatch; consumes core + strategy
//	combinator   ./, workflow/             public convenience surface and high-level builders
//	                                      that produce *core.Agent; consume core + engine
//
// Forbidden edges (an inner rung learning about an outer one):
//
//	core       ↛ strategy, engine, combinator   primitives depend on nothing above them
//	strategy   ↛ engine, combinator             a plug-in must not reach the engine
//	engine     ↛ combinator                     the engine must not depend on the combinators built atop it
//
// Intentionally allowed (correct ladder edges, and the documented "库内部用具体
// 类型" stance — agent/CLAUDE.md): strategy → core; engine → core + strategy;
// combinator → core + engine (workflow holds *runtime.Platform by concrete type,
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
		from := layerOf(filepath.ToSlash(rel))
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
			to := layerOf(rest)
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

func TestTargetToolLoopDoesNotImportLegacyProtocol(t *testing.T) {
	root := moduleRoot(t)
	fset := token.NewFileSet()
	for _, name := range []string{
		"checkpoint.go",
		"control.go",
		"event.go",
		"invocation.go",
		"runner.go",
		"runtime_policy.go",
	} {
		path := filepath.Join(root, "toolloop", name)
		file, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse target tool-loop file %s: %v", name, err)
		}
		for _, imported := range file.Imports {
			importPath := strings.Trim(imported.Path.Value, `"`)
			if importPath == "github.com/Tangerg/lynx/core/model" ||
				strings.HasPrefix(importPath, "github.com/Tangerg/lynx/core/model/") ||
				importPath == "github.com/Tangerg/lynx/chatclient" ||
				strings.HasPrefix(importPath, "github.com/Tangerg/lynx/chatclient/") {
				t.Errorf("target tool-loop file %s imports frozen runtime %q", name, importPath)
			}
		}
	}
}

func TestAgentDoesNotImportLegacyCoreModel(t *testing.T) {
	root := moduleRoot(t)
	fset := token.NewFileSet()
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if entry.Name() == "vendor" || (strings.HasPrefix(entry.Name(), ".") && path != root) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		file, parseErr := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if parseErr != nil {
			return parseErr
		}
		for _, imported := range file.Imports {
			importPath := strings.Trim(imported.Path.Value, `"`)
			if importPath == "github.com/Tangerg/lynx/core/model" ||
				strings.HasPrefix(importPath, "github.com/Tangerg/lynx/core/model/") {
				relative, _ := filepath.Rel(root, path)
				t.Errorf("agent file %s imports removed Core model path %q", relative, importPath)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

const (
	rungCore       = "core"
	rungStrategy   = "strategy"
	rungEngine     = "engine"
	rungCombinator = "combinator"
)

// layerOf classifies a module-relative package dir (e.g. "planning/planner/goap")
// into its rung, or "" when the path is outside the rungs under test.
func layerOf(rel string) string {
	if rel == "." {
		return rungCombinator
	}
	first, _, _ := strings.Cut(rel, "/")
	switch first {
	case "core":
		return rungCore
	case "planning", "event", "hitl", "toolpolicy", "toolloop":
		return rungStrategy
	case "runtime":
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

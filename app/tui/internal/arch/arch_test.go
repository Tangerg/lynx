// Package arch holds the tests that keep the library's layering true.
//
// The layering is what the library is: three rings, each more general than the one
// above it, and a dependency edge that only ever points down. A ring nothing checks is
// a ring that drifts — the boundary held by discipline alone is the one somebody
// crosses once, quietly, in a hurry.
//
// Each ring declares the rings it must never import, rather than the ones it may. A
// list of wrong directions stays short and stays true; a matrix of allowed edges has to
// be edited every time a legitimate one appears, which teaches people to edit the test
// instead of the code.
package arch

import (
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const modulePath = "github.com/Tangerg/lynx/app/tui"

// The rings, longest prefix first so the first match wins.
var rings = []struct {
	prefix string
	name   string
}{
	{"primitives/", "primitives"},
	{"atoms/", "atoms"},
	{"program/", "program"},
	{"internal/", "internal"},
}

// forbidden is, for each ring, the rings it may never import.
var forbidden = map[string][]string{
	// Cells, text, input and the terminal. The most general layer there is: it knows
	// what a terminal is made of and nothing about what anyone builds from it.
	"primitives": {"atoms", "program"},

	// Widgets with no meaning of their own: a list is a list whether it holds files or
	// sessions. Atoms may draw and answer input; they may not own a goroutine, a
	// terminal, or a program.
	"atoms": {"program"},

	// The loop. It is the outermost ring and the only one that owns a goroutine.
	"program": {},

	// The tests that guard the rings. They import nothing.
	"internal": {"primitives", "atoms", "program"},
}

// dependencies are the only third-party packages this library uses.
//
// The list is short on purpose, and it is a promise: a terminal interface library that
// drags a dependency tree behind it is one people work around instead of using. Adding
// to it is a decision, which is why it is written down where a test can fail.
var dependencies = []string{
	"github.com/rivo/uniseg",
	"github.com/mattn/go-runewidth",
	"golang.org/x/term",
	"golang.org/x/sys",
}

func TestEveryImportPointsDown(t *testing.T) {
	root := moduleRoot(t)
	fset := token.NewFileSet()

	checked := 0
	walk(t, root, func(dir, path string) {
		from := ringOf(dir)
		if from == "" {
			return
		}
		checked++
		for _, imported := range imports(t, fset, path) {
			rest, ok := strings.CutPrefix(imported, modulePath+"/")
			if !ok {
				continue
			}
			to := ringOf(rest)
			if to == "" || to == from {
				continue
			}
			if slices.Contains(forbidden[from], to) {
				t.Errorf("%s (%s) imports %s (%s): %s must never depend on %s",
					dir, from, rest, to, from, to)
			}
		}
	})
	if checked == 0 {
		t.Fatal("no files were checked, so this test proves nothing")
	}
}

// TestTheLibraryDependsOnNothingElse is the promise that makes this library usable.
//
// A terminal interface library that pulls in a framework, a logger and a colour
// package is one people wrap rather than adopt. The list it is checked against is
// short, and lengthening it is a decision somebody has to make on purpose.
func TestTheLibraryDependsOnNothingElse(t *testing.T) {
	root := moduleRoot(t)
	fset := token.NewFileSet()

	walk(t, root, func(dir, path string) {
		for _, imported := range imports(t, fset, path) {
			if !strings.Contains(imported, ".") {
				continue // a standard library package
			}
			if strings.HasPrefix(imported, modulePath) {
				continue
			}
			allowed := false
			for _, dep := range dependencies {
				if imported == dep || strings.HasPrefix(imported, dep+"/") {
					allowed = true
					break
				}
			}
			if !allowed {
				t.Errorf("%s imports %s, which is not one of this library's dependencies",
					dir, imported)
			}
		}
	})
}

// TestOnlyPrimitivesKnowWhatDrawsTheTerminal keeps the rendering substrate replaceable.
//
// The day it is worth changing what draws — a different width table, a different
// terminal package — the work is one ring's rather than the whole library's. That stays
// true only while nothing above that ring has quietly reached for it.
func TestOnlyPrimitivesKnowWhatDrawsTheTerminal(t *testing.T) {
	root := moduleRoot(t)
	fset := token.NewFileSet()

	walk(t, root, func(dir, path string) {
		if ring := ringOf(dir); ring == "primitives" || ring == "" {
			return
		}
		for _, imported := range imports(t, fset, path) {
			for _, dep := range dependencies {
				if imported == dep || strings.HasPrefix(imported, dep+"/") {
					t.Errorf("%s imports %s: only primitives may know what draws the terminal",
						dir, imported)
				}
			}
		}
	})
}

// TestEveryDirectoryBelongsToARing catches a directory added without a rule to govern
// it, which would be unguarded from the day it appeared — the way an unguarded boundary
// always starts.
func TestEveryDirectoryBelongsToARing(t *testing.T) {
	root := moduleRoot(t)
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("read the module: %v", err)
	}
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		ring := ringOf(entry.Name() + "/")
		if ring == "" {
			t.Errorf("%s belongs to no ring: add it to the rules, or put its code in a "+
				"ring that exists", entry.Name())
			continue
		}
		if _, ruled := forbidden[ring]; !ruled {
			t.Errorf("ring %q has no rule saying what it must not import", ring)
		}
	}
}

// TestTheRulesWouldActuallyRefuseSomething is the counter-example. A guard never shown
// to fail is a guard nobody knows is wired up: were the ring table empty, or its names
// out of step with what ringOf produces, every check above would pass by finding
// nothing.
func TestTheRulesWouldActuallyRefuseSomething(t *testing.T) {
	for _, tc := range []struct {
		from, to string
		refused  bool
	}{
		// The edges that make the rings rings.
		{"primitives/grid", "atoms", true},
		{"primitives/text", "program", true},
		{"atoms", "program", true},
		{"atoms/theme", "program", true},

		// The edges the rings are made of.
		{"atoms", "primitives/grid", false},
		{"program", "atoms", false},
		{"program", "primitives/term", false},
		{"primitives/text", "primitives/grid", false},
	} {
		from, to := ringOf(tc.from), ringOf(tc.to)
		if from == "" {
			t.Fatalf("%s belongs to no ring, so nothing about it is guarded", tc.from)
		}
		if to == "" {
			t.Fatalf("%s belongs to no ring, so importing it is unguarded", tc.to)
		}
		if got := slices.Contains(forbidden[from], to); got != tc.refused {
			verb := map[bool]string{true: "refused", false: "allowed"}
			t.Errorf("%s -> %s is %s, want it %s", from, to, verb[got], verb[tc.refused])
		}
	}
}

// ringOf names the ring a module-relative directory belongs to.
func ringOf(dir string) string {
	dir = filepath.ToSlash(dir)
	if !strings.HasSuffix(dir, "/") {
		dir += "/"
	}
	for _, r := range rings {
		if strings.HasPrefix(dir, r.prefix) {
			return r.name
		}
	}
	return ""
}

// walk visits every production Go file, reporting its directory relative to the module
// root. Test files are skipped: a test may reach across rings for a fixture, and
// constraining that would buy nothing.
func walk(t *testing.T, root string, visit func(dir, path string)) {
	t.Helper()
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if name := d.Name(); name == "vendor" || (strings.HasPrefix(name, ".") && path != root) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		dir, err := filepath.Rel(root, filepath.Dir(path))
		if err != nil {
			return err
		}
		visit(filepath.ToSlash(dir), path)
		return nil
	})
	if err != nil {
		t.Fatalf("walk the module: %v", err)
	}
}

// imports reads one file's import paths.
func imports(t *testing.T, fset *token.FileSet, path string) []string {
	t.Helper()
	file, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	out := make([]string, 0, len(file.Imports))
	for _, spec := range file.Imports {
		out = append(out, strings.Trim(spec.Path.Value, `"`))
	}
	return out
}

// moduleRoot is the directory holding go.mod.
func moduleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("working directory: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("no go.mod above the working directory")
		}
		dir = parent
	}
}

// Package arch holds the tests that keep this program's layering true.
//
// The interface library lives in its own module and guards its own rings. What is
// guarded here is the product: where its data comes from, how it is folded, how it is
// shown, and the rule that keeps the library a library.
package arch

import (
	"go/parser"
	"go/token"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const (
	modulePath  = "github.com/Tangerg/lynx/app/cli"
	libraryPath = "github.com/Tangerg/oolong"
)

// The layers, longest prefix first so the first match wins.
var layers = []struct {
	prefix string
	name   string
}{
	{"internal/agent/mock/", "mock"},
	{"internal/sideload/", "sideload"},
	{"internal/terminal/", "terminal"},
	{"internal/attachment/", "attachment"},
	{"internal/promptqueue/", "promptqueue"},
	{"internal/reconnect/", "reconnect"},
	{"internal/requestid/", "requestid"},
	{"internal/session/", "session"},
	{"internal/oneshot/", "oneshot"},
	{"internal/agent/", "agent"},
	{"internal/settings/", "settings"},
	{"internal/extensions/", "extensions"},
	{"internal/render/", "render"},
	{"internal/cmd/", "cmd"},
	{"internal/arch/", "arch"},
}

// allowed names every inward or same-ring dependency. An allowlist makes a new
// dependency fail closed instead of silently weakening the architecture.
var allowed = map[string][]string{
	// Domain policy and generic infrastructure are the center.
	"agent":       nil,
	"settings":    {"agent"},
	"requestid":   nil,
	"session":     {"agent"},
	"oneshot":     {"agent", "reconnect", "requestid"},
	"extensions":  nil,
	"promptqueue": {"agent"},

	// Outbound adapters share domain contracts, not one another.
	"attachment": {"agent"},
	"reconnect":  {"agent"},
	"mock":       {"agent"},
	"render":     {"agent"},

	// Delivery adapters compose inward abstractions. Sideloading is the outer trust
	// boundary around terminal contributions; cmd is the application composition root.
	"terminal": {"agent", "attachment", "extensions", "promptqueue", "reconnect", "requestid", "session", "settings"},
	"sideload": {"extensions", "terminal"},
	"cmd":      {"agent", "attachment", "extensions", "oneshot", "render", "session", "settings", "sideload", "terminal"},
	"arch":     nil,
}

func TestLayeringHoldsInTheImports(t *testing.T) {
	root := moduleRoot(t)
	fset := token.NewFileSet()

	checked := 0
	walk(t, root, func(dir, path string) {
		from := layerOf(dir)
		if from == "" {
			return
		}
		checked++
		for _, imported := range imports(t, fset, path) {
			rest, ok := strings.CutPrefix(imported, modulePath+"/")
			if !ok {
				continue
			}
			to := layerOf(rest)
			if to == "" || to == from {
				continue
			}
			if !slices.Contains(allowed[from], to) {
				t.Errorf("%s (%s) imports %s (%s): %s must never depend on %s",
					dir, from, rest, to, from, to)
			}
		}
	})
	if checked == 0 {
		t.Fatal("no files were checked, so this test proves nothing")
	}
}

func TestEveryInternalPackageBelongsToALayer(t *testing.T) {
	root := moduleRoot(t)
	seen := make(map[string]struct{})
	walk(t, root, func(dir, _ string) {
		if !strings.HasPrefix(dir, "internal/") || layerOf(dir) != "" {
			return
		}
		seen[dir] = struct{}{}
	})
	for _, dir := range slices.Sorted(maps.Keys(seen)) {
		t.Errorf("%s belongs to no architecture layer", dir)
	}
}

// TestTheLibraryStaysALibrary is the rule that keeps the interface library extractable.
//
// It is in its own module, so a reference from the library back into this program will
// not compile — that much is guaranteed by Go. What this test guards is the other
// direction: which of this program's layers are allowed to know that a terminal exists
// at all. The data and the renderers must not, or the interface stops being one choice
// among several and becomes the only one.
func TestTheLibraryStaysALibrary(t *testing.T) {
	root := moduleRoot(t)
	fset := token.NewFileSet()

	terminalFree := []string{"agent", "settings", "mock", "attachment", "promptqueue", "reconnect", "requestid", "session", "oneshot", "extensions", "render"}
	walk(t, root, func(dir, path string) {
		layer := layerOf(dir)
		if !slices.Contains(terminalFree, layer) {
			return
		}
		for _, imported := range imports(t, fset, path) {
			if strings.HasPrefix(imported, libraryPath) {
				t.Errorf("%s (%s) imports %s: this layer must not know there is a terminal",
					dir, layer, imported)
			}
		}
	})
}

// TestTheRulesWouldActuallyRefuseSomething is the counter-example: a guard never shown
// to fail is a guard nobody knows is wired up.
func TestTheRulesWouldActuallyRefuseSomething(t *testing.T) {
	for _, tc := range []struct {
		from, to string
		refused  bool
	}{
		{"internal/agent", "internal/agent/mock", true},
		{"internal/agent", "internal/terminal", true},
		{"internal/extensions", "internal/agent", true},
		{"internal/agent/mock", "internal/render", true},
		{"internal/attachment", "internal/terminal", true},
		{"internal/reconnect", "internal/cmd", true},
		{"internal/requestid", "internal/agent", true},
		{"internal/session", "internal/terminal", true},
		{"internal/oneshot", "internal/cmd", true},
		{"internal/settings", "internal/terminal", true},
		{"internal/promptqueue", "internal/terminal", true},
		{"internal/render", "internal/terminal", true},
		{"internal/terminal", "internal/cmd", true},
		{"internal/sideload", "internal/cmd", true},

		{"internal/agent/mock", "internal/agent", false},
		{"internal/terminal", "internal/agent", false},
		{"internal/terminal", "internal/extensions", false},
		{"internal/cmd", "internal/terminal", false},
		{"internal/sideload", "internal/extensions", false},
		{"internal/render", "internal/agent", false},
		{"internal/attachment", "internal/agent", false},
		{"internal/reconnect", "internal/agent", false},
		{"internal/cmd", "internal/requestid", true},
		{"internal/cmd", "internal/session", false},
		{"internal/cmd", "internal/oneshot", false},
		{"internal/settings", "internal/agent", false},
		{"internal/session", "internal/agent", false},
		{"internal/oneshot", "internal/agent", false},
		{"internal/promptqueue", "internal/agent", false},
	} {
		from, to := layerOf(tc.from), layerOf(tc.to)
		if from == "" {
			t.Fatalf("%s belongs to no layer, so nothing about it is guarded", tc.from)
		}
		if to == "" {
			t.Fatalf("%s belongs to no layer, so importing it is unguarded", tc.to)
		}
		got := from != to && !slices.Contains(allowed[from], to)
		if got != tc.refused {
			verb := map[bool]string{true: "refused", false: "allowed"}
			t.Errorf("%s -> %s is %s, want it %s", from, to, verb[got], verb[tc.refused])
		}
	}
}

// layerOf names the layer a module-relative directory belongs to.
func layerOf(dir string) string {
	dir = filepath.ToSlash(dir)
	if !strings.HasSuffix(dir, "/") {
		dir += "/"
	}
	for _, l := range layers {
		if strings.HasPrefix(dir, l.prefix) {
			return l.name
		}
	}
	return ""
}

// walk visits every production Go file, reporting its directory relative to the
// module root. Test files are skipped: a test may reach across layers for a
// fixture, and constraining that would buy nothing.
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

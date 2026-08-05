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
	{"internal/ui/store/", "store"},
	{"internal/ui/parts/", "parts"},
	{"internal/ui/views/", "views"},
	{"internal/ui/session/", "session"},
	{"internal/ui/", "ui"},
	{"internal/client/", "client"},
	{"internal/render/", "render"},
	{"internal/cmd/", "cmd"},
	{"internal/arch/", "arch"},
}

// forbidden is, for each layer, the layers it may never import.
var forbidden = map[string][]string{
	// The fold from a run's events into what a screen shows. It knows the data and
	// nothing about how it looks.
	"store": {"parts", "views", "session", "cmd", "render"},

	// Widgets that mean something here — a transcript, an approval prompt. They read the
	// view models; they do not arrange screens or fetch anything.
	"parts": {"views", "session", "cmd", "render"},

	// Screens: a layout, and who gets the keyboard.
	"views": {"session", "cmd", "render"},

	// The seam between the library and this program: it turns what the user does into
	// calls, and what the runtime says into what the screen shows. The only layer here
	// that owns a goroutine.
	"session": {"cmd", "render"},

	// Where the data comes from. Consumed by the interface and by the headless
	// renderers, and knows about neither.
	"client": {"ui", "store", "parts", "views", "session", "render", "cmd"},

	// The headless renderers. They share the client with the interface and nothing else:
	// both turn events into bytes, but one is a contract for a pipe and the other an
	// interface for a person, and they change for different reasons. Merging them would
	// be a false economy.
	"render": {"ui", "store", "parts", "views", "session", "cmd"},
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

	terminalFree := []string{"client", "render"}
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
		{"internal/ui/parts/x", "internal/ui/views/x", true},
		{"internal/ui/store", "internal/ui/parts/x", true},
		{"internal/ui/views/x", "internal/ui/session", true},
		{"internal/client", "internal/ui/views/x", true},
		{"internal/render", "internal/ui/parts/x", true},
		{"internal/ui/session", "internal/cmd", true},

		{"internal/ui/parts/x", "internal/ui/store", false},
		{"internal/ui/views/x", "internal/ui/parts/x", false},
		{"internal/ui/session", "internal/ui/views/x", false},
		{"internal/ui/session", "internal/client", false},
		{"internal/cmd", "internal/ui/session", false},
		{"internal/render", "internal/client", false},
	} {
		from, to := layerOf(tc.from), layerOf(tc.to)
		if from == "" {
			t.Fatalf("%s belongs to no layer, so nothing about it is guarded", tc.from)
		}
		if to == "" {
			t.Fatalf("%s belongs to no layer, so importing it is unguarded", tc.to)
		}
		if got := slices.Contains(forbidden[from], to); got != tc.refused {
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

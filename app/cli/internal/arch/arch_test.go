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
	modulePath  = "github.com/Tangerg/scope/app/cli"
	libraryPath = "github.com/Tangerg/oolong"
	runtimePath = "github.com/Tangerg/scope/app/runtime"
	cobraPath   = "github.com/spf13/cobra"
	viperPath   = "github.com/spf13/viper"
)

// The layers, longest prefix first so the first match wins.
var layers = []struct {
	prefix string
	name   string
}{
	{"internal/agent/mock/", "mock"},
	{"internal/runtimeembedded/", "runtimeembedded"},
	{"internal/authoringcontext/", "authoringcontext"},
	{"internal/agentmemory/", "agentmemory"},
	{"internal/diagnostictool/", "diagnostictool"},
	{"internal/hookpolicy/", "hookpolicy"},
	{"internal/feedback/", "feedback"},
	{"internal/failure/", "failure"},
	{"internal/changefeed/", "changefeed"},
	{"internal/workspace/", "workspace"},
	{"internal/usage/", "usage"},
	{"internal/modelconfig/", "modelconfig"},
	{"internal/goal/", "goal"},
	{"internal/knowledge/", "knowledge"},
	{"internal/skills/", "skills"},
	{"internal/mcp/", "mcp"},
	{"internal/schedule/", "schedule"},
	{"internal/backend/", "backend"},
	{"internal/sideload/", "sideload"},
	{"internal/terminal/", "terminal"},
	{"internal/attachment/", "attachment"},
	{"internal/promptqueue/", "promptqueue"},
	{"internal/mutation/", "mutation"},
	{"internal/reconnect/", "reconnect"},
	{"internal/retry/", "retry"},
	{"internal/runrecovery/", "runrecovery"},
	{"internal/runtimeprofile/", "runtimeprofile"},
	{"internal/session/", "session"},
	{"internal/sessionartifact/", "sessionartifact"},
	{"internal/sessiontransfer/", "sessiontransfer"},
	{"internal/sessiondeletion/", "sessiondeletion"},
	{"internal/sessionrollback/", "sessionrollback"},
	{"internal/steering/", "steering"},
	{"internal/workbench/", "workbench"},
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
	"failure":          nil,
	"runtimeprofile":   nil,
	"agent":            {"failure", "workspace"},
	"agentmemory":      nil,
	"authoringcontext": nil,
	"diagnostictool":   nil,
	"hookpolicy":       nil,
	"feedback":         nil,
	"changefeed":       nil,
	"workspace":        nil,
	"usage":            nil,
	"modelconfig":      {"failure"},
	"goal":             nil,
	"knowledge":        nil,
	"skills":           nil,
	"mcp":              {"failure"},
	"schedule":         nil,
	"backend":          {"agent", "agentmemory", "authoringcontext", "changefeed", "diagnostictool", "feedback", "goal", "hookpolicy", "knowledge", "mcp", "modelconfig", "runtimeprofile", "schedule", "sessiontransfer", "skills", "usage", "workspace"},
	"settings":         {"agent"},
	"session":          {"agent"},
	"sessiondeletion":  {"agent", "mutation", "retry", "workbench"},
	"sessionrollback":  {"agent", "mutation", "retry", "workbench"},
	"steering":         {"agent", "mutation", "retry", "workbench"},
	"mutation":         {"agent", "retry"},
	"retry":            nil,
	"oneshot":          {"agent", "mutation", "reconnect", "retry", "runrecovery"},
	"extensions":       nil,
	"promptqueue":      {"agent"},
	"sessiontransfer":  {"agent"},
	"sessionartifact":  {"sessiontransfer"},
	"workbench":        {"agent"},

	// Outbound adapters share domain contracts, not one another.
	"attachment":      {"agent"},
	"reconnect":       {"agent"},
	"runrecovery":     {"agent"},
	"mock":            {"agent", "failure", "workspace"},
	"runtimeembedded": {"agent", "agentmemory", "authoringcontext", "backend", "changefeed", "diagnostictool", "failure", "feedback", "goal", "hookpolicy", "knowledge", "mcp", "modelconfig", "runtimeprofile", "schedule", "sessiontransfer", "skills", "usage", "workspace"},
	"render":          {"agent", "failure"},

	// Delivery adapters compose inward abstractions. Sideloading is the outer trust
	// boundary around terminal contributions; cmd is the application composition root.
	"terminal": {"agent", "agentmemory", "attachment", "authoringcontext", "changefeed", "diagnostictool", "extensions", "failure", "feedback", "goal", "hookpolicy", "knowledge", "mcp", "modelconfig", "mutation", "promptqueue", "reconnect", "retry", "runrecovery", "runtimeprofile", "schedule", "session", "sessionartifact", "sessiondeletion", "sessionrollback", "sessiontransfer", "settings", "skills", "steering", "usage", "workbench", "workspace"},
	"sideload": {"extensions", "terminal"},
	"cmd":      {"agent", "attachment", "backend", "extensions", "failure", "mutation", "oneshot", "render", "retry", "runtimeprofile", "session", "sessiondeletion", "settings", "sideload", "terminal", "workbench"},
	"arch":     nil,
}

var adapterDependencies = []struct {
	path          string
	allowedLayers []string
}{
	{path: runtimePath, allowedLayers: []string{"runtimeembedded"}},
	{path: cobraPath, allowedLayers: []string{"cmd"}},
	{path: viperPath, allowedLayers: []string{"cmd"}},
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

	terminalFree := []string{"agent", "agentmemory", "authoringcontext", "backend", "changefeed", "diagnostictool", "failure", "feedback", "goal", "hookpolicy", "knowledge", "mcp", "modelconfig", "mutation", "retry", "runtimeprofile", "schedule", "skills", "usage", "workspace", "settings", "mock", "runtimeembedded", "attachment", "promptqueue", "reconnect", "runrecovery", "session", "sessionartifact", "sessiondeletion", "sessionrollback", "sessiontransfer", "steering", "workbench", "oneshot", "extensions", "render"}
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

func TestAdapterDependenciesStayAtTheirBoundaries(t *testing.T) {
	root := moduleRoot(t)
	fset := token.NewFileSet()
	walk(t, root, func(dir, path string) {
		layer := layerOf(dir)
		if layer == "" {
			return
		}
		for _, imported := range imports(t, fset, path) {
			for _, dependency := range adapterDependencies {
				if importsPath(imported, dependency.path) && !slices.Contains(dependency.allowedLayers, layer) {
					t.Errorf("%s (%s) imports %s: %s is confined to %s",
						dir, layer, imported, dependency.path, strings.Join(dependency.allowedLayers, ", "))
				}
			}
		}
	})
}

func TestAdapterBoundaryRulesRefuseInwardLeaks(t *testing.T) {
	for _, test := range []struct {
		layer, imported string
	}{
		{layer: "agent", imported: runtimePath + "/protocol"},
		{layer: "settings", imported: cobraPath},
		{layer: "oneshot", imported: viperPath},
	} {
		if adapterDependencyAllowed(test.layer, test.imported) {
			t.Errorf("%s unexpectedly accepts %s", test.layer, test.imported)
		}
	}
}

func adapterDependencyAllowed(layer, imported string) bool {
	for _, dependency := range adapterDependencies {
		if importsPath(imported, dependency.path) {
			return slices.Contains(dependency.allowedLayers, layer)
		}
	}
	return true
}

func importsPath(imported, dependency string) bool {
	return imported == dependency || strings.HasPrefix(imported, dependency+"/")
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
		{"internal/agent", "internal/runtimeembedded", true},
		{"internal/extensions", "internal/agent", true},
		{"internal/agent/mock", "internal/render", true},
		{"internal/attachment", "internal/terminal", true},
		{"internal/reconnect", "internal/cmd", true},
		{"internal/runrecovery", "internal/cmd", true},
		{"internal/session", "internal/terminal", true},
		{"internal/sessionartifact", "internal/terminal", true},
		{"internal/sessiontransfer", "internal/terminal", true},
		{"internal/workbench", "internal/terminal", true},
		{"internal/oneshot", "internal/cmd", true},
		{"internal/settings", "internal/terminal", true},
		{"internal/promptqueue", "internal/terminal", true},
		{"internal/render", "internal/terminal", true},
		{"internal/terminal", "internal/cmd", true},
		{"internal/sideload", "internal/cmd", true},

		{"internal/agent/mock", "internal/agent", false},
		{"internal/runtimeembedded", "internal/agent", false},
		{"internal/runtimeembedded", "internal/terminal", true},
		{"internal/terminal", "internal/agent", false},
		{"internal/terminal", "internal/sessionartifact", false},
		{"internal/terminal", "internal/sessiontransfer", false},
		{"internal/terminal", "internal/workbench", false},
		{"internal/terminal", "internal/extensions", false},
		{"internal/cmd", "internal/terminal", false},
		{"internal/sideload", "internal/extensions", false},
		{"internal/render", "internal/agent", false},
		{"internal/attachment", "internal/agent", false},
		{"internal/reconnect", "internal/agent", false},
		{"internal/runrecovery", "internal/agent", false},
		{"internal/cmd", "internal/runrecovery", true},
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

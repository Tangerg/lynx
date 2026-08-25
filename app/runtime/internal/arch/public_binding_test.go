package arch

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestPublicPackageSetIsExact(t *testing.T) {
	root := moduleRoot(t)
	want := []string{"embedded", "localruntime", "protocol"}
	var got []string
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("read Runtime module root: %v", err)
	}
	rootPackageFound := false
	for _, entry := range entries {
		if !entry.IsDir() {
			if !rootPackageFound && strings.HasSuffix(entry.Name(), ".go") && !strings.HasSuffix(entry.Name(), "_test.go") {
				got = append(got, ".")
				rootPackageFound = true
			}
			continue
		}
		if entry.Name() == "cmd" || entry.Name() == "internal" {
			continue
		}
		hasProductionGo, err := directoryHasProductionGo(filepath.Join(root, entry.Name()))
		if err != nil {
			t.Fatalf("inspect %s: %v", entry.Name(), err)
		}
		if hasProductionGo {
			got = append(got, entry.Name())
		}
	}
	slices.Sort(got)
	if !slices.Equal(got, want) {
		t.Fatalf("public Runtime packages = %v, want %v; change the public Go baseline deliberately", got, want)
	}
}

func directoryHasProductionGo(directory string) (bool, error) {
	errFound := fmt.Errorf("production Go file found")
	err := filepath.WalkDir(directory, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if path != directory && (entry.Name() == "testdata" || strings.HasPrefix(entry.Name(), ".")) {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(entry.Name(), ".go") && !strings.HasSuffix(entry.Name(), "_test.go") {
			return errFound
		}
		return nil
	})
	if err == errFound {
		return true, nil
	}
	return false, err
}

// TestPublicBindingsCompileForAnExternalModule proves the public Go bindings
// are usable without the Runtime module's internal-package privilege. Exact
// operation coverage is enforced by embedded's own surface test.
func TestPublicBindingsCompileForAnExternalModule(t *testing.T) {
	directory := t.TempDir()
	goMod := fmt.Sprintf(`module example.com/runtimeconsumer

go 1.27.0

require (
	github.com/Tangerg/lynx/app/runtime v0.0.0
	github.com/Tangerg/lynx/app/runtime/localruntime v0.0.0
)

replace github.com/Tangerg/lynx/app/runtime => %s
replace github.com/Tangerg/lynx/app/runtime/localruntime => %s
`, filepath.ToSlash(moduleRoot(t)), filepath.ToSlash(filepath.Join(moduleRoot(t), "localruntime")))
	source := `package runtimeconsumer

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/embedded"
	"github.com/Tangerg/lynx/app/runtime/localruntime"
	"github.com/Tangerg/lynx/app/runtime/protocol"
)

var _ = embedded.Open
var _ *embedded.Runtime
var _ = localruntime.ReadToken
var _ protocol.RunEvent

func consume(ctx context.Context, runtime *embedded.Runtime) error {
	if _, err := runtime.Discover(ctx, embedded.CallOptions{}); err != nil {
		return err
	}
	_, events, err := runtime.StartRun(ctx, protocol.StartRunRequest{}, embedded.RunCommandOptions{})
	if err != nil {
		return err
	}
	for _, err := range events {
		if err != nil {
			return err
		}
	}
	return runtime.Close()
}
`
	writeConsumerFile(t, filepath.Join(directory, "go.mod"), goMod)
	writeConsumerFile(t, filepath.Join(directory, "consumer.go"), source)

	command := exec.CommandContext(t.Context(), "go", "test", "-mod=mod", "./...")
	command.Dir = directory
	command.Env = append(os.Environ(), "GOWORK=off")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("compile external embedded consumer: %v\n%s", err, output)
	}
}

func writeConsumerFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write external consumer fixture %s: %v", path, err)
	}
}

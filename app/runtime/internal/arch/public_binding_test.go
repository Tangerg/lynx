package arch

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestEmbeddedBindingCompilesForAnExternalModule proves the public Go binding
// is usable without the Runtime module's internal-package privilege. Exact
// operation coverage is enforced by embedded's own surface test.
func TestEmbeddedBindingCompilesForAnExternalModule(t *testing.T) {
	directory := t.TempDir()
	goMod := fmt.Sprintf(`module example.com/runtimeconsumer

go 1.26.5

require github.com/Tangerg/lynx/app/runtime v0.0.0

replace github.com/Tangerg/lynx/app/runtime => %s
`, filepath.ToSlash(moduleRoot(t)))
	source := `package runtimeconsumer

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/embedded"
	"github.com/Tangerg/lynx/app/runtime/protocol"
)

var _ = embedded.Open
var _ *embedded.Runtime
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

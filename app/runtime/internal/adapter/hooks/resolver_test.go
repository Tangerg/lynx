package hooks

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	domainhooks "github.com/Tangerg/lynx/app/runtime/internal/domain/hooks"
)

func TestResolver_ForRechecksTrust(t *testing.T) {
	home := t.TempDir()
	cwd := t.TempDir()
	if err := os.MkdirAll(filepath.Join(cwd, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeHooks(t, home, `{"hooks":[{"event":"SessionStart","inject":"global"}]}`)
	writeHooks(t, cwd, `{"hooks":[{"event":"SessionStart","inject":"project"}]}`)

	trusted := false
	resolver := NewResolver(home, func(projectRoot string) bool {
		return projectRoot == cwd && trusted
	}, nil)

	before := resolver.For(context.Background(), cwd).Run(context.Background(), domainhooks.Input{Event: domainhooks.SessionStart})
	if before.InjectContext != "global" {
		t.Fatalf("InjectContext before trust = %q, want global only", before.InjectContext)
	}

	trusted = true
	after := resolver.For(context.Background(), cwd).Run(context.Background(), domainhooks.Input{Event: domainhooks.SessionStart})
	if after.InjectContext != "global\nproject" {
		t.Fatalf("InjectContext after trust = %q, want global and project", after.InjectContext)
	}
}

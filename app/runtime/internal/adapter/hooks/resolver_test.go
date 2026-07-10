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
	resolver := NewResolver(home, func(_ context.Context, projectRoot string) bool {
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

func TestResolver_TrustUsesCallContext(t *testing.T) {
	home := t.TempDir()
	cwd := t.TempDir()
	if err := os.MkdirAll(filepath.Join(cwd, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeHooks(t, cwd, `{"hooks":[{"event":"SessionStart","inject":"project"}]}`)

	type key struct{}
	ctx := context.WithValue(context.Background(), key{}, "turn")
	resolver := NewResolver(home, func(ctx context.Context, projectRoot string) bool {
		return projectRoot == cwd && ctx.Value(key{}) == "turn"
	}, nil)

	got := resolver.For(ctx, cwd).Run(context.Background(), domainhooks.Input{Event: domainhooks.SessionStart})
	if got.InjectContext != "project" {
		t.Fatalf("InjectContext = %q, want project hook trusted by call context", got.InjectContext)
	}
}

func TestResolver_InspectReturnsSnapshot(t *testing.T) {
	home := t.TempDir()
	cwd := t.TempDir()
	writeHooks(t, cwd, `{"hooks":[{"event":"SessionStart","inject":"original"}]}`)
	resolver := NewResolver(home, nil, nil)

	first := resolver.Inspect(context.Background(), cwd)
	if len(first.Hooks) != 1 {
		t.Fatalf("hooks = %v, want one", first.Hooks)
	}
	first.Hooks[0].Inject = "mutated"
	second := resolver.Inspect(context.Background(), cwd)
	if second.Hooks[0].Inject != "original" {
		t.Fatalf("cached hook was mutated through inspection: %+v", second.Hooks[0])
	}
}

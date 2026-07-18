package hooks

import (
	"context"
	"errors"
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
	resolver := NewResolver(home, func(_ context.Context, projectRoot string) (bool, error) {
		return projectRoot == cwd && trusted, nil
	}, nil)

	beforeHooks, err := resolver.For(context.Background(), cwd)
	if err != nil {
		t.Fatalf("For before trust: %v", err)
	}
	before := beforeHooks.Run(context.Background(), domainhooks.Input{Event: domainhooks.SessionStart})
	if before.InjectContext != "global" {
		t.Fatalf("InjectContext before trust = %q, want global only", before.InjectContext)
	}

	trusted = true
	afterHooks, err := resolver.For(context.Background(), cwd)
	if err != nil {
		t.Fatalf("For after trust: %v", err)
	}
	after := afterHooks.Run(context.Background(), domainhooks.Input{Event: domainhooks.SessionStart})
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
	resolver := NewResolver(home, func(ctx context.Context, projectRoot string) (bool, error) {
		return projectRoot == cwd && ctx.Value(key{}) == "turn", nil
	}, nil)

	bound, err := resolver.For(ctx, cwd)
	if err != nil {
		t.Fatalf("For: %v", err)
	}
	got := bound.Run(context.Background(), domainhooks.Input{Event: domainhooks.SessionStart})
	if got.InjectContext != "project" {
		t.Fatalf("InjectContext = %q, want project hook trusted by call context", got.InjectContext)
	}
}

func TestResolver_InspectReturnsSnapshot(t *testing.T) {
	home := t.TempDir()
	cwd := t.TempDir()
	writeHooks(t, cwd, `{"hooks":[{"event":"SessionStart","inject":"original"}]}`)
	resolver := NewResolver(home, func(context.Context, string) (bool, error) {
		return true, nil
	}, nil)

	first, err := resolver.Inspect(context.Background(), cwd)
	if err != nil {
		t.Fatalf("Inspect first: %v", err)
	}
	if len(first.Hooks) != 1 {
		t.Fatalf("hooks = %v, want one", first.Hooks)
	}
	first.Hooks[0].Inject = "mutated"
	second, err := resolver.Inspect(context.Background(), cwd)
	if err != nil {
		t.Fatalf("Inspect second: %v", err)
	}
	if second.Hooks[0].Inject != "original" {
		t.Fatalf("resolved hook was mutated through prior inspection: %+v", second.Hooks[0])
	}
}

func TestResolverReloadsEditedHooks(t *testing.T) {
	home := t.TempDir()
	cwd := t.TempDir()
	writeHooks(t, cwd, `{"hooks":[{"event":"SessionStart","inject":"before"}]}`)
	resolver := NewResolver(home, func(context.Context, string) (bool, error) {
		return true, nil
	}, nil)

	before, err := resolver.For(context.Background(), cwd)
	if err != nil {
		t.Fatalf("For before edit: %v", err)
	}
	writeHooks(t, cwd, `{"hooks":[{"event":"SessionStart","inject":"after"}]}`)
	after, err := resolver.For(context.Background(), cwd)
	if err != nil {
		t.Fatalf("For after edit: %v", err)
	}
	if got := before.Run(context.Background(), domainhooks.Input{Event: domainhooks.SessionStart}).InjectContext; got != "before" {
		t.Fatalf("before edit hook = %q", got)
	}
	if got := after.Run(context.Background(), domainhooks.Input{Event: domainhooks.SessionStart}).InjectContext; got != "after" {
		t.Fatalf("after edit hook = %q, want after", got)
	}
}

func TestResolverPreservesTrustFailure(t *testing.T) {
	wantErr := errors.New("trust store unavailable")
	resolver := NewResolver(t.TempDir(), func(context.Context, string) (bool, error) {
		return false, wantErr
	}, nil)

	if _, err := resolver.For(context.Background(), t.TempDir()); !errors.Is(err, wantErr) {
		t.Fatalf("For error = %v, want %v", err, wantErr)
	}
}

func TestResolverIgnoresInvalidUntrustedProjectHooks(t *testing.T) {
	home := t.TempDir()
	cwd := t.TempDir()
	writeHooks(t, home, `{"hooks":[{"event":"SessionStart","inject":"global"}]}`)
	writeHooks(t, cwd, `{ invalid project config `)
	resolver := NewResolver(home, func(context.Context, string) (bool, error) {
		return false, nil
	}, nil)

	bound, err := resolver.For(context.Background(), cwd)
	if err != nil {
		t.Fatalf("For: %v", err)
	}
	if got := bound.Run(context.Background(), domainhooks.Input{Event: domainhooks.SessionStart}).InjectContext; got != "global" {
		t.Fatalf("InjectContext = %q, want global", got)
	}
	if _, err := resolver.Inspect(context.Background(), cwd); err == nil {
		t.Fatal("Inspect invalid project config error = nil")
	}
}

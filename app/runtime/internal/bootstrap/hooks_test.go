package bootstrap

import (
	"context"
	"errors"
	"testing"
)

type failingHookTrust struct{ err error }

func (f failingHookTrust) IsTrusted(context.Context, string) (bool, error) {
	return false, f.err
}

func TestNewHookResolverPreservesTrustStoreFailure(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	wantErr := errors.New("trust store unavailable")
	resolver := NewHookResolver(t.TempDir(), failingHookTrust{err: wantErr})

	if _, err := resolver.For(context.Background(), t.TempDir()); !errors.Is(err, wantErr) {
		t.Fatalf("For error = %v, want %v", err, wantErr)
	}
}

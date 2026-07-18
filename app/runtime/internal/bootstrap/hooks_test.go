package bootstrap

import (
	"context"
	"errors"
	"testing"
)

type failingHookTrust struct{ err error }

func (t failingHookTrust) IsTrusted(context.Context, string) (bool, error) {
	return false, t.err
}

func TestNewHookResolverPreservesTrustStoreFailure(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	wantErr := errors.New("trust store unavailable")
	resolver := NewHookResolver(failingHookTrust{err: wantErr})

	if _, err := resolver.For(context.Background(), t.TempDir()); !errors.Is(err, wantErr) {
		t.Fatalf("For error = %v, want %v", err, wantErr)
	}
}

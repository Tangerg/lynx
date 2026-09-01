package modeltest

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

// WithTimeout bounds one integration call. A provider that hangs would
// otherwise consume the whole package deadline and report the timeout against
// an unrelated test; deriving from t.Context keeps the bound tied to this test
// so cancellation still propagates when the test fails first.
func WithTimeout(t *testing.T, duration time.Duration) (context.Context, context.CancelFunc) {
	t.Helper()
	return context.WithTimeout(t.Context(), duration)
}

const envKeyPrefix = "SCOPE_TEST_"

// RequireKey skips rather than fails when a provider credential is absent, so
// the same suite is runnable both in CI without secrets and locally with them.
// Deriving the variable name from the provider keeps credentials discoverable
// without a per-provider convention to look up.
func RequireKey(t *testing.T, provider string) string {
	t.Helper()
	name := envKeyPrefix + strings.ToUpper(provider) + "_KEY"
	v := os.Getenv(name)
	if v == "" {
		t.Skipf("set %s to run this integration test", name)
	}
	return v
}

// RequireEnv carries the same skip semantics as [RequireKey] for the settings
// that are not credentials: an endpoint, region, or deployment name a provider
// cannot infer. It is separate because those names are vendor-specific and
// cannot be derived from the provider alone.
func RequireEnv(t *testing.T, name string) string {
	t.Helper()
	v := os.Getenv(name)
	if v == "" {
		t.Skipf("set %s to run this integration test", name)
	}
	return v
}

// LookupEnv reads an optional setting whose absence is a valid configuration
// rather than a reason to skip, so the caller decides what a missing value
// means instead of losing the test to an unconditional skip.
func LookupEnv(name string) (string, bool) {
	v := os.Getenv(name)
	return v, v != ""
}

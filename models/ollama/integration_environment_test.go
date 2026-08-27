//go:build integration

package ollama_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

// withTimeout returns a child context of t.Context() with the given
// deadline. Use this in integration tests to bound real API calls.
func withTimeout(t *testing.T, d time.Duration) (context.Context, context.CancelFunc) {
	t.Helper()
	return context.WithTimeout(t.Context(), d)
}

const envKeyPrefix = "SCOPE_TEST_"

// requireKey returns the value of SCOPE_TEST_<provider>_KEY or skips the
// test when unset. The provider string is upper-cased; e.g.
// requireKey(t, "openai") looks up SCOPE_TEST_OPENAI_KEY.
//
// Integration tests should use this as their first line — it provides a
// uniform skip message and consistent env var naming across all vendors.
func requireKey(t *testing.T, provider string) string {
	t.Helper()
	name := envKeyPrefix + strings.ToUpper(provider) + "_KEY"
	v := os.Getenv(name)
	if v == "" {
		t.Skipf("set %s to run this integration test", name)
	}
	return v
}

// requireEnv returns the value of the named env var or skips the test
// when unset. Use this for non-key configuration (BaseURL overrides,
// model ids, regions) that integration tests might want to read.
func requireEnv(t *testing.T, name string) string {
	t.Helper()
	v := os.Getenv(name)
	if v == "" {
		t.Skipf("set %s to run this integration test", name)
	}
	return v
}

// lookupEnv returns the env var value plus an ok flag. No skip — for
// optional integration knobs (model id override etc.).
func lookupEnv(name string) (string, bool) {
	v := os.Getenv(name)
	return v, v != ""
}

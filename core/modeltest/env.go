package modeltest

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

func WithTimeout(t *testing.T, duration time.Duration) (context.Context, context.CancelFunc) {
	t.Helper()
	return context.WithTimeout(t.Context(), duration)
}

const envKeyPrefix = "SCOPE_TEST_"

func RequireKey(t *testing.T, provider string) string {
	t.Helper()
	name := envKeyPrefix + strings.ToUpper(provider) + "_KEY"
	v := os.Getenv(name)
	if v == "" {
		t.Skipf("set %s to run this integration test", name)
	}
	return v
}

func RequireEnv(t *testing.T, name string) string {
	t.Helper()
	v := os.Getenv(name)
	if v == "" {
		t.Skipf("set %s to run this integration test", name)
	}
	return v
}

func LookupEnv(name string) (string, bool) {
	v := os.Getenv(name)
	return v, v != ""
}

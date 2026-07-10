package mcpserver

import (
	"maps"
	"testing"
)

func TestServerSafeEnv(t *testing.T) {
	s := Server{Env: map[string]string{
		"API_KEY":               "x",
		"LD_PRELOAD":            "/evil.so",
		"dyld_insert_libraries": "/evil.dylib", // lower-case still dropped
		"PATH":                  "/usr/bin",    // benign, kept
	}}

	got := s.SafeEnv()
	want := map[string]string{"API_KEY": "x", "PATH": "/usr/bin"}
	if !maps.Equal(got, want) {
		t.Fatalf("SafeEnv() = %v, want %v", got, want)
	}

	// Must not mutate the original entry.
	if _, ok := s.Env["LD_PRELOAD"]; !ok {
		t.Fatal("SafeEnv mutated Server.Env")
	}

	// Empty stays empty (nil in, nil out).
	if (Server{}).SafeEnv() != nil {
		t.Fatal("nil Env should yield nil")
	}
	empty := Server{Env: map[string]string{}}
	gotEmpty := empty.SafeEnv()
	gotEmpty["NEW"] = "value"
	if len(empty.Env) != 0 {
		t.Fatal("non-nil empty Env shared storage with SafeEnv result")
	}
}

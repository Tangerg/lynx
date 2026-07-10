package main

import "testing"

func TestResolvedVersionPrefersExplicitLinkValue(t *testing.T) {
	original := version
	version = "v1.2.3"
	t.Cleanup(func() { version = original })

	if got := resolvedVersion(); got != "v1.2.3" {
		t.Fatalf("resolvedVersion = %q, want explicit link value", got)
	}
}

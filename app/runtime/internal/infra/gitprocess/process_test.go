package gitprocess

import (
	"slices"
	"strings"
	"testing"
)

func TestEnvironmentOwnsGitControlsAndExplicitOverrides(t *testing.T) {
	t.Setenv("GIT_DIR", "/foreign/repository")
	t.Setenv("GIT_INDEX_FILE", "/foreign/index")
	t.Setenv("GIT_CONFIG_KEY_0", "core.hooksPath")
	t.Setenv("GIT_LITERAL_PATHSPECS", "1")
	t.Setenv("git_optional_locks", "0")
	t.Setenv("SCOPE_GITPROCESS_SENTINEL", "preserved")
	t.Setenv("SCOPE_GITPROCESS_OVERRIDE", "ambient")

	environment := Environment(
		"GIT_DIR=/owned/repository",
		"SCOPE_GITPROCESS_OVERRIDE=explicit",
	)
	if !slices.Contains(environment, "GIT_DIR=/owned/repository") {
		t.Fatal("command-owned GIT_DIR override was not installed")
	}
	if !slices.Contains(environment, "SCOPE_GITPROCESS_SENTINEL=preserved") {
		t.Fatal("unrelated host environment was not preserved")
	}
	if !slices.Contains(environment, "SCOPE_GITPROCESS_OVERRIDE=explicit") {
		t.Fatal("explicit non-Git override was not installed")
	}
	for _, entry := range environment {
		if hasGitPrefix(strings.SplitN(entry, "=", 2)[0]) && entry != "GIT_DIR=/owned/repository" {
			t.Fatalf("inherited Git control survived: %q", entry)
		}
		if entry == "SCOPE_GITPROCESS_OVERRIDE=ambient" {
			t.Fatal("ambient value survived an explicit override")
		}
	}
}

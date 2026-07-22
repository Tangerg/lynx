//go:build darwin

package sandbox

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func runConfined(t *testing.T, c Command, dir string) ([]byte, error) {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), c.Name, c.Args...)
	cmd.Dir = dir
	cmd.Env = c.Env
	return cmd.CombinedOutput()
}

func TestConfinerJailsInPlace(t *testing.T) {
	// The confiner roots writes at the real working tree in place (no copy): a
	// write inside the root succeeds, a write outside fails, the home is hidden,
	// and the environment is scrubbed of inherited credentials.
	workspace := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.txt")
	home := t.TempDir()
	secret := filepath.Join(home, "secret.txt")
	if err := os.WriteFile(secret, []byte("private"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("LYRA_SANDBOX_SECRET", "must-not-leak")

	confiner, err := NewConfiner(nil)
	if err != nil {
		t.Fatal(err)
	}

	inside, err := confiner.Confine(workspace,
		"printf inside > inside.txt; printf %s \"${LYRA_SANDBOX_SECRET-unset}\"; test ! -r "+strconv.Quote(secret))
	if err != nil {
		t.Fatal(err)
	}
	out, err := runConfined(t, inside, workspace)
	if err != nil {
		t.Fatalf("confined command failed: %v (output %q)", err, out)
	}
	content, err := os.ReadFile(filepath.Join(workspace, "inside.txt"))
	if err != nil || string(content) != "inside" {
		t.Fatalf("inside write = (%q, %v); output %q", content, err, out)
	}
	if strings.Contains(string(out), "must-not-leak") {
		t.Fatal("jail inherited a credential-like environment variable")
	}

	outsideCmd, err := confiner.Confine(workspace, "printf outside > "+strconv.Quote(outside))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runConfined(t, outsideCmd, workspace); err == nil {
		t.Fatal("write outside the workspace unexpectedly succeeded")
	}
	if _, err := os.Stat(outside); !os.IsNotExist(err) {
		t.Fatalf("outside write created a file: %v", err)
	}
}

func TestConfineRejectsEmptyCommand(t *testing.T) {
	confiner, err := NewConfiner(nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := confiner.Confine(t.TempDir(), ""); err == nil {
		t.Fatal("expected an error for an empty command")
	}
}

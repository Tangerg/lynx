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

func TestConfineShellCommandJailsInPlace(t *testing.T) {
	// The jail roots writes at the real working tree in place (no copy): a write
	// inside cwd succeeds, a write outside fails, the home is hidden, and the
	// environment is scrubbed of inherited credentials.
	workspace := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.txt")
	home := t.TempDir()
	secret := filepath.Join(home, "secret.txt")
	if err := os.WriteFile(secret, []byte("private"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("LYRA_SANDBOX_SECRET", "must-not-leak")

	name, args, env, err := ConfineShellCommand(workspace, nil,
		"printf inside > inside.txt; printf %s \"${LYRA_SANDBOX_SECRET-unset}\"; test ! -r "+strconv.Quote(secret))
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(t.Context(), name, args...)
	cmd.Dir = workspace
	cmd.Env = env
	out, err := cmd.CombinedOutput()
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

	name, args, env, err = ConfineShellCommand(workspace, nil, "printf outside > "+strconv.Quote(outside))
	if err != nil {
		t.Fatal(err)
	}
	cmd = exec.CommandContext(t.Context(), name, args...)
	cmd.Dir = workspace
	cmd.Env = env
	if err := cmd.Run(); err == nil {
		t.Fatal("write outside the workspace unexpectedly succeeded")
	}
	if _, err := os.Stat(outside); !os.IsNotExist(err) {
		t.Fatalf("outside write created a file: %v", err)
	}
}

func TestConfineShellCommandRejectsEmpty(t *testing.T) {
	if _, _, _, err := ConfineShellCommand(t.TempDir(), nil, ""); err == nil {
		t.Fatal("expected an error for an empty command")
	}
}

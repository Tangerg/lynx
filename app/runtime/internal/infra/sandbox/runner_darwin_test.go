//go:build darwin

package sandbox

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	toolshell "github.com/Tangerg/lynx/tools/shell"
)

func TestSeatbeltRunnerConfinesWritesAndEnvironment(t *testing.T) {
	workspace := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.txt")
	home := t.TempDir()
	secret := filepath.Join(home, "secret.txt")
	if err := os.WriteFile(secret, []byte("private"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	runner, err := platformRunner(nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("LYRA_SANDBOX_SECRET", "must-not-leak")
	out, err := runner.Run(t.Context(), workspace, toolshell.Input{
		Cmd: "printf inside > inside.txt; printf %s \"${LYRA_SANDBOX_SECRET-unset}\"; test ! -r " + strconv.Quote(secret),
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.ExitCode != 0 {
		t.Fatalf("inside command failed: exit=%d stdout=%q stderr=%q", out.ExitCode, out.Stdout, out.Stderr)
	}
	content, err := os.ReadFile(filepath.Join(workspace, "inside.txt"))
	if err != nil || string(content) != "inside" {
		t.Fatalf("inside write = (%q, %v); exit=%d stdout=%q stderr=%q", content, err, out.ExitCode, out.Stdout, out.Stderr)
	}
	if strings.Contains(string(out.Stdout), "must-not-leak") {
		t.Fatal("sandbox inherited a credential-like environment variable")
	}

	out, err = runner.Run(t.Context(), workspace, toolshell.Input{Cmd: "printf outside > " + strconv.Quote(outside)})
	if err != nil {
		t.Fatal(err)
	}
	if out.ExitCode == 0 {
		t.Fatalf("outside write unexpectedly succeeded: stdout=%q stderr=%q", out.Stdout, out.Stderr)
	}
	if _, err := os.Stat(outside); !os.IsNotExist(err) {
		t.Fatalf("outside write created a file: %v", err)
	}
}

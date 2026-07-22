//go:build darwin

package exec

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/infra/sandbox"
)

// TestLaunchSandboxConfinesWrites proves the opt-in jail is actually wired onto
// the live shell: a sandboxed Launch confines a command's writes to its cwd.
func TestLaunchSandboxConfinesWrites(t *testing.T) {
	workspace := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.txt")

	confiner, err := sandbox.NewConfiner(nil)
	if err != nil {
		t.Fatalf("new confiner: %v", err)
	}
	shells := NewShells(confiner)
	t.Cleanup(func() { _ = shells.KillAll() })

	run := func(command string) *Shell {
		id, err := shells.Launch(t.Context(), "s1", workspace, command, 10*time.Second)
		if err != nil {
			t.Fatalf("launch: %v", err)
		}
		sh, ok := shells.Get(id)
		if !ok {
			t.Fatal("launched shell vanished")
		}
		<-sh.Done()
		return sh
	}

	sh := run("printf inside > inside.txt")
	if code, _, _ := sh.Outcome(); code != 0 {
		out, _ := sh.Read()
		t.Fatalf("inside write exited %d: %q", code, out)
	}
	content, err := os.ReadFile(filepath.Join(workspace, "inside.txt"))
	if err != nil || string(content) != "inside" {
		t.Fatalf("inside write = (%q, %v)", content, err)
	}

	sh = run("printf outside > " + strconv.Quote(outside))
	if code, _, _ := sh.Outcome(); code == 0 {
		t.Fatal("write outside the workspace unexpectedly succeeded")
	}
	if _, err := os.Stat(outside); !os.IsNotExist(err) {
		t.Fatalf("outside write created a file: %v", err)
	}
}

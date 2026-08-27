//go:build unix

package exec

import (
	"errors"
	"os"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

const shellDescendantPIDEnv = "SCOPE_EXEC_DESCENDANT_PID_FILE"

func TestShellKillReclaimsDescendants(t *testing.T) {
	pidFile := t.TempDir() + "/descendant.pid"
	t.Setenv(shellDescendantPIDEnv, pidFile)
	shells := NewShells(nil, false)
	t.Cleanup(func() { _ = shells.KillAll() })

	id, err := shells.Launch(
		t.Context(),
		"",
		"",
		`sleep 30 & echo $! > "$SCOPE_EXEC_DESCENDANT_PID_FILE"; wait`,
		0,
		false,
	)
	if err != nil {
		t.Fatalf("launch shell: %v", err)
	}
	pid := waitForShellDescendantPID(t, pidFile)
	t.Cleanup(func() {
		if err := syscall.Kill(pid, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
			t.Errorf("clean leaked descendant %d: %v", pid, err)
		}
	})

	if running, err := shells.Kill(id); err != nil || !running {
		t.Fatalf("Kill = (running=%v, err=%v), want stopped running shell", running, err)
	}
	waitDone(t, shells, id)
	if waitForShellProcessExit(pid, 2*time.Second) {
		return
	}
	t.Fatalf("shell descendant %d survived Kill", pid)
}

func TestShellCompletionReclaimsDescendantsWithoutRewritingLeaderExit(t *testing.T) {
	pidFile := t.TempDir() + "/descendant.pid"
	t.Setenv(shellDescendantPIDEnv, pidFile)
	shells := NewShells(nil, false)
	t.Cleanup(func() { _ = shells.KillAll() })

	id, err := shells.Launch(
		t.Context(),
		"",
		"",
		`sleep 30 & echo $! > "$SCOPE_EXEC_DESCENDANT_PID_FILE"`,
		0,
		false,
	)
	if err != nil {
		t.Fatalf("launch shell: %v", err)
	}
	pid := waitForShellDescendantPID(t, pidFile)
	t.Cleanup(func() {
		if err := syscall.Kill(pid, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
			t.Errorf("clean leaked descendant %d: %v", pid, err)
		}
	})
	waitDone(t, shells, id)

	code, killed, _, cleanupErr := mustShell(t, shells, id).Outcome()
	if code != 0 || killed || cleanupErr != nil {
		t.Fatalf("Outcome = (code=%d, killed=%v, cleanup=%v), want successful leader", code, killed, cleanupErr)
	}
	if !waitForShellProcessExit(pid, 2*time.Second) {
		t.Fatalf("shell descendant %d survived successful leader exit", pid)
	}
}

func waitForShellDescendantPID(t *testing.T, path string) int {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(path)
		if err == nil {
			pid, parseErr := strconv.Atoi(strings.TrimSpace(string(data)))
			if parseErr != nil {
				t.Fatalf("parse descendant PID: %v", parseErr)
			}
			return pid
		}
		if !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("read descendant PID: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("shell did not publish its descendant PID")
	return 0
}

func waitForShellProcessExit(pid int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if err := syscall.Kill(pid, 0); errors.Is(err, syscall.ESRCH) {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return false
}

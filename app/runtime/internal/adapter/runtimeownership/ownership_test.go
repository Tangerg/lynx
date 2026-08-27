package runtimeownership

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestManagersShareSessionAndGoalDriveOwnership(t *testing.T) {
	data := t.TempDir()
	first, err := New(data)
	if err != nil {
		t.Fatalf("New first: %v", err)
	}
	second, err := New(data)
	if err != nil {
		t.Fatalf("New second: %v", err)
	}

	sessionLease, ok := first.TrySession("session-1")
	if !ok {
		t.Fatal("first Session lease was refused")
	}
	if _, trySessionOk := second.TrySession("session-1"); trySessionOk {
		t.Fatal("second manager acquired the same Session writer")
	}
	if other, trySessionOk := second.TrySession("session-2"); !trySessionOk {
		t.Fatal("unrelated Session was blocked")
	} else {
		other.Release()
	}
	sessionLease.Release()
	if next, trySessionOk := second.TrySession("session-1"); !trySessionOk {
		t.Fatal("Session writer did not transfer after release")
	} else {
		next.Release()
	}

	drive, ok := first.TryGoalDrive("session-1")
	if !ok {
		t.Fatal("first Goal drive lease was refused")
	}
	if _, tryGoalDriveOk := second.TryGoalDrive("session-1"); tryGoalDriveOk {
		t.Fatal("second manager acquired the same Goal drive")
	}
	drive.Release()

	sweep, ok := first.TryRecoverySweep()
	if !ok {
		t.Fatal("first recovery sweep lease was refused")
	}
	if _, ok := second.TryRecoverySweep(); ok {
		t.Fatal("second manager acquired the same recovery sweep")
	}
	sweep.Release()
}

func TestWorkingTreeRunsSharePhysicalIdentityAndExcludeMutation(t *testing.T) {
	data := t.TempDir()
	first, err := New(data)
	if err != nil {
		t.Fatalf("New first: %v", err)
	}
	second, err := New(data)
	if err != nil {
		t.Fatalf("New second: %v", err)
	}
	cwd := t.TempDir()
	alias := filepath.Join(t.TempDir(), "workspace")
	if err := os.Symlink(cwd, alias); err != nil {
		t.Fatalf("create workspace alias: %v", err)
	}

	runA, ok := first.TryWorkingTree(cwd, true)
	if !ok {
		t.Fatal("first shared working-tree lease was refused")
	}
	runB, ok := second.TryWorkingTree(alias, true)
	if !ok {
		t.Fatal("second shared working-tree lease through alias was refused")
	}
	if _, tryWorkingTreeOk := second.TryWorkingTree(cwd, false); tryWorkingTreeOk {
		t.Fatal("destructive mutation crossed active Run leases")
	}
	runA.Release()
	if _, tryWorkingTreeOk := first.TryWorkingTree(cwd, false); tryWorkingTreeOk {
		t.Fatal("destructive mutation crossed the remaining Run lease")
	}
	runB.Release()
	mutation, ok := second.TryWorkingTree(alias, false)
	if !ok {
		t.Fatal("destructive mutation did not acquire after Runs released")
	}
	if _, ok := first.TryWorkingTree(cwd, true); ok {
		t.Fatal("Run crossed active destructive mutation")
	}
	mutation.Release()
}

func TestSessionOwnershipTransfersAfterProcessKill(t *testing.T) {
	const childEnvironment = "SCOPEAPP_TEST_RUNTIME_OWNERSHIP_CHILD"
	if os.Getenv(childEnvironment) == "1" {
		manager, err := New(os.Getenv("SCOPEAPP_TEST_RUNTIME_OWNERSHIP_DATA"))
		if err != nil {
			t.Fatal(err)
		}
		lease, ok := manager.TrySession("session-after-crash")
		if !ok {
			t.Fatal("child could not acquire Session ownership")
		}
		if err := os.WriteFile(os.Getenv("SCOPEAPP_TEST_RUNTIME_OWNERSHIP_READY"), nil, 0o600); err != nil {
			t.Fatal(err)
		}
		var oneByte [1]byte
		_, _ = os.Stdin.Read(oneByte[:])
		lease.Release()
		return
	}

	data := t.TempDir()
	ready := filepath.Join(t.TempDir(), "ready")
	command := exec.Command(os.Args[0], "-test.run=^TestSessionOwnershipTransfersAfterProcessKill$")
	command.Env = append(os.Environ(),
		childEnvironment+"=1",
		"SCOPEAPP_TEST_RUNTIME_OWNERSHIP_DATA="+data,
		"SCOPEAPP_TEST_RUNTIME_OWNERSHIP_READY="+ready,
	)
	stdin, err := command.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	defer stdin.Close()
	var output bytes.Buffer
	command.Stdout = &output
	command.Stderr = &output
	if startErr := command.Start(); startErr != nil {
		t.Fatal(startErr)
	}
	t.Cleanup(func() { _ = command.Process.Kill() })
	waitForFile(t, ready, &output)

	manager, err := New(data)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := manager.TrySession("session-after-crash"); ok {
		t.Fatal("parent acquired Session ownership while child was alive")
	}
	if err := command.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	if err := command.Wait(); err == nil {
		t.Fatal("killed child exited successfully")
	}

	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for {
		if lease, ok := manager.TrySession("session-after-crash"); ok {
			lease.Release()
			return
		}
		select {
		case <-ticker.C:
		case <-deadline.C:
			t.Fatalf("Session ownership did not transfer after process death: %s", output.String())
		}
	}
}

func waitForFile(t *testing.T, path string, childOutput *bytes.Buffer) {
	t.Helper()
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for {
		if _, err := os.Stat(path); err == nil {
			return
		} else if !errors.Is(err, os.ErrNotExist) {
			t.Fatal(err)
		}
		select {
		case <-ticker.C:
		case <-deadline.C:
			t.Fatalf("ownership child did not become ready: %s", childOutput.String())
		}
	}
}

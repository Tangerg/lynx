package bootstrap

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestDataDirectoryLeaseRejectsCanonicalAndSymlinkAliases(t *testing.T) {
	directory := t.TempDir()
	first, err := acquireDataDirectoryLease(directory)
	if err != nil {
		t.Fatalf("acquire first lease: %v", err)
	}
	t.Cleanup(func() { _ = first.release() })

	if _, err := acquireDataDirectoryLease(directory); !errors.Is(err, ErrDataDirectoryInUse) {
		t.Fatalf("second lease error = %v, want ErrDataDirectoryInUse", err)
	}

	alias := filepath.Join(t.TempDir(), "data")
	if err := os.Symlink(directory, alias); err != nil {
		t.Fatalf("create symlink alias: %v", err)
	}
	if _, err := acquireDataDirectoryLease(alias); !errors.Is(err, ErrDataDirectoryInUse) {
		t.Fatalf("symlink lease error = %v, want ErrDataDirectoryInUse", err)
	}
}

func TestDataDirectoryLeaseRejectsAnotherProcess(t *testing.T) {
	if os.Getenv("LYRA_TEST_LOCK_CHILD") == "1" {
		lease, err := acquireDataDirectoryLease(os.Getenv("LYRA_TEST_LOCK_DIRECTORY"))
		if errors.Is(err, ErrDataDirectoryInUse) {
			return
		}
		if err == nil {
			_ = lease.release()
		}
		t.Fatalf("child lease error = %v, want ErrDataDirectoryInUse", err)
	}

	directory := t.TempDir()
	lease, err := acquireDataDirectoryLease(directory)
	if err != nil {
		t.Fatalf("acquire parent lease: %v", err)
	}
	t.Cleanup(func() { _ = lease.release() })

	command := exec.Command(os.Args[0], "-test.run=^TestDataDirectoryLeaseRejectsAnotherProcess$")
	command.Env = append(os.Environ(),
		"LYRA_TEST_LOCK_CHILD=1",
		"LYRA_TEST_LOCK_DIRECTORY="+directory,
	)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("child lock probe: %v\n%s", err, output)
	}
}

func TestDataDirectoryLeaseReleasesOnlyWhenAsked(t *testing.T) {
	directory := t.TempDir()
	first, err := acquireDataDirectoryLease(directory)
	if err != nil {
		t.Fatalf("acquire first lease: %v", err)
	}
	if err := first.release(); err != nil {
		t.Fatalf("release first lease: %v", err)
	}
	if err := first.release(); err != nil {
		t.Fatalf("release first lease again: %v", err)
	}

	second, err := acquireDataDirectoryLease(directory)
	if err != nil {
		t.Fatalf("acquire after release: %v", err)
	}
	if err := second.release(); err != nil {
		t.Fatalf("release second lease: %v", err)
	}
}

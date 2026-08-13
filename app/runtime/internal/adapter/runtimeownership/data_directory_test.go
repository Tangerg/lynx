package runtimeownership

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDataDirectorySetupCanonicalizesAndProtectsDirectory(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "data")
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatalf("create directory: %v", err)
	}
	if err := os.Symlink(directory, alias); err != nil {
		t.Fatalf("create alias: %v", err)
	}
	setup, err := PrepareDataDirectory(t.Context(), alias)
	if err != nil {
		t.Fatalf("acquire setup: %v", err)
	}
	defer func() { _ = setup.Release() }()
	physical, err := filepath.EvalSymlinks(directory)
	if err != nil {
		t.Fatalf("resolve directory: %v", err)
	}
	if setup.Directory != physical {
		t.Fatalf("canonical directory = %q, want %q", setup.Directory, physical)
	}
	info, err := os.Stat(physical)
	if err != nil {
		t.Fatalf("stat directory: %v", err)
	}
	if info.Mode().Perm() != 0o700 {
		t.Fatalf("directory permissions = %o, want 700", info.Mode().Perm())
	}
}

func TestDataDirectorySetupSerializesOnlySetupWindow(t *testing.T) {
	directory := t.TempDir()
	first, err := PrepareDataDirectory(t.Context(), directory)
	if err != nil {
		t.Fatalf("acquire first setup: %v", err)
	}

	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	secondResult := make(chan *DataDirectorySetup, 1)
	errorResult := make(chan error, 1)
	go func() {
		second, err := PrepareDataDirectory(ctx, directory)
		if err != nil {
			errorResult <- err
			return
		}
		secondResult <- second
	}()
	select {
	case <-secondResult:
		t.Fatal("second setup crossed the first setup window")
	case err := <-errorResult:
		t.Fatalf("second setup failed while waiting: %v", err)
	case <-time.After(30 * time.Millisecond):
	}
	if err := first.Release(); err != nil {
		t.Fatalf("release first setup: %v", err)
	}
	if err := first.Release(); err != nil {
		t.Fatalf("release first setup again: %v", err)
	}
	select {
	case second := <-secondResult:
		if err := second.Release(); err != nil {
			t.Fatalf("release second setup: %v", err)
		}
	case err := <-errorResult:
		t.Fatalf("second setup: %v", err)
	case <-ctx.Done():
		t.Fatal("second setup did not acquire after release")
	}
}

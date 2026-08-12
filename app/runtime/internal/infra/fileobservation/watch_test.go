package fileobservation

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWatchObservesMissingParentsReplacementAndRemoval(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "nested", ".lyra", "hooks.json")
	events := make(chan []string, 8)
	watcher, err := Watch([]Target{{Key: "hooks", Path: target}}, func(keys []string) { events <- keys })
	if err != nil {
		t.Fatalf("watch missing target: %v", err)
	}
	defer func() { _ = watcher.Close() }()

	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("one"), 0o644); err != nil {
		t.Fatal(err)
	}
	assertObservedKey(t, events, "hooks")

	replacement := filepath.Join(filepath.Dir(target), "replacement")
	if err := os.WriteFile(replacement, []byte("two"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(replacement, target); err != nil {
		t.Fatal(err)
	}
	assertObservedKey(t, events, "hooks")

	if err := os.RemoveAll(filepath.Join(root, "nested")); err != nil {
		t.Fatal(err)
	}
	assertObservedKey(t, events, "hooks")
}

func TestWatchObservesPhysicalSymlinkTargetAndCloseJoins(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "knowledge-target.md")
	alias := filepath.Join(root, "LYRA.md")
	if err := os.WriteFile(target, []byte("one"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Base(target), alias); err != nil {
		t.Fatal(err)
	}
	events := make(chan []string, 8)
	watcher, err := Watch([]Target{{Key: "knowledge", Path: alias, Boundary: root}}, func(keys []string) {
		events <- keys
	})
	if err != nil {
		t.Fatalf("watch symlink: %v", err)
	}

	if err := os.WriteFile(target, []byte("two"), 0o644); err != nil {
		t.Fatal(err)
	}
	assertObservedKey(t, events, "knowledge")
	if err := watcher.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("three"), 0o644); err != nil {
		t.Fatal(err)
	}
	select {
	case keys := <-events:
		t.Fatalf("callback after Close = %v", keys)
	case <-time.After(300 * time.Millisecond):
	}
}

func TestAcceptRefreshesOnlyTheExactIdentity(t *testing.T) {
	root := t.TempDir()
	first := filepath.Join(root, "first", "LYRA.md")
	second := filepath.Join(root, "second", "LYRA.md")
	for _, path := range []string{first, second} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("before"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	events := make(chan []string, 4)
	watcher, err := Watch([]Target{
		{Key: "knowledge", Path: first, Boundary: filepath.Dir(first)},
		{Key: "knowledge", Path: second, Boundary: filepath.Dir(second)},
	}, func(keys []string) { events <- keys })
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = watcher.Close() }()
	if err := os.WriteFile(first, []byte("api write"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(second, []byte("external write"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := watcher.Accept([]string{"knowledge"}, []string{first}); err != nil {
		t.Fatal(err)
	}
	assertObservedKey(t, events, "knowledge")
	select {
	case keys := <-events:
		t.Fatalf("accepted identity produced a duplicate callback: %v", keys)
	case <-time.After(300 * time.Millisecond):
	}
}

func TestWatchSuppressesMetadataNoiseWithoutSemanticChange(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "LYRA.md")
	if err := os.WriteFile(target, []byte("stable"), 0o644); err != nil {
		t.Fatal(err)
	}
	events := make(chan []string, 2)
	watcher, err := Watch([]Target{{Key: "knowledge", Path: target, Boundary: root}}, func(keys []string) {
		events <- keys
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = watcher.Close() }()
	info, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(target, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(target, 0o600); err != nil {
		t.Fatal(err)
	}
	select {
	case keys := <-events:
		t.Fatalf("metadata-only noise published %v", keys)
	case <-time.After(300 * time.Millisecond):
	}
}

func assertObservedKey(t *testing.T, events <-chan []string, want string) {
	t.Helper()
	select {
	case keys := <-events:
		if len(keys) != 1 || keys[0] != want {
			t.Fatalf("keys = %v, want [%s]", keys, want)
		}
	case <-time.After(3 * time.Second):
		t.Fatalf("no %s observation", want)
	}
}

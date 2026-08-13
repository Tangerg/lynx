package fileobservation

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWatchTreesObservesDynamicExactFiles(t *testing.T) {
	root := filepath.Join(t.TempDir(), "skills")
	events := make(chan []string, 8)
	watcher, err := WatchTrees([]TreeTarget{{
		Key: "skills", Path: root, Boundary: filepath.Dir(root), FileName: "SKILL.md",
	}}, func(keys []string) { events <- keys })
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = watcher.Close() }()

	file := filepath.Join(root, "lint", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte("one"), 0o644); err != nil {
		t.Fatal(err)
	}
	assertObservedKey(t, events, "skills")
	if err := os.WriteFile(file, []byte("two"), 0o644); err != nil {
		t.Fatal(err)
	}
	assertObservedKey(t, events, "skills")
	if err := os.RemoveAll(filepath.Dir(file)); err != nil {
		t.Fatal(err)
	}
	assertObservedKey(t, events, "skills")
}

func TestWatchTreesAcceptsOnlyExactCommittedFiles(t *testing.T) {
	root := t.TempDir()
	first := filepath.Join(root, "first", "SKILL.md")
	second := filepath.Join(root, "second", "SKILL.md")
	for _, path := range []string{first, second} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("before"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	events := make(chan []string, 4)
	watcher, err := WatchTrees([]TreeTarget{{
		Key: "skills", Path: root, Boundary: root, FileName: "SKILL.md",
	}}, func(keys []string) { events <- keys })
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
	if err := watcher.Accept([]string{"skills"}, []string{first}); err != nil {
		t.Fatal(err)
	}
	assertObservedKey(t, events, "skills")
	select {
	case keys := <-events:
		t.Fatalf("accepted identity produced a duplicate callback: %v", keys)
	case <-time.After(300 * time.Millisecond):
	}
}

func TestWatchTreesIgnoresNonProjectionFiles(t *testing.T) {
	root := t.TempDir()
	events := make(chan []string, 2)
	watcher, err := WatchTrees([]TreeTarget{{
		Key: "skills", Path: root, Boundary: root, FileName: "SKILL.md",
	}}, func(keys []string) { events <- keys })
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = watcher.Close() }()
	if err := os.WriteFile(filepath.Join(root, ".usage.json"), []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}
	select {
	case keys := <-events:
		t.Fatalf("non-projection file published %v", keys)
	case <-time.After(300 * time.Millisecond):
	}
}

func TestWatchTreesDoesNotWedgeOnNonRegularProjectionPath(t *testing.T) {
	root := t.TempDir()
	events := make(chan []string, 4)
	watcher, err := WatchTrees([]TreeTarget{{
		Key: "skills", Path: root, Boundary: root, FileName: "SKILL.md",
	}}, func(keys []string) { events <- keys })
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = watcher.Close() }()
	path := filepath.Join(root, "broken", "SKILL.md")
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	select {
	case keys := <-events:
		t.Fatalf("invalid projection directory published %v", keys)
	case <-time.After(300 * time.Millisecond):
	}
	if err := os.RemoveAll(path); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("now regular"), 0o644); err != nil {
		t.Fatal(err)
	}
	assertObservedKey(t, events, "skills")
}

package sandbox

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	toolshell "github.com/Tangerg/lynx/tools/shell"
)

type memorySnapshots struct {
	mu   sync.Mutex
	data map[string][]byte
}

func (s *memorySnapshots) SaveSandboxSnapshot(_ context.Context, id string, archive []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.data == nil {
		s.data = make(map[string][]byte)
	}
	s.data[id] = append([]byte(nil), archive...)
	return nil
}

func (s *memorySnapshots) LoadSandboxSnapshot(_ context.Context, id string) ([]byte, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	archive, ok := s.data[id]
	return append([]byte(nil), archive...), ok, nil
}

type recordingRunner struct{}

func (recordingRunner) Run(_ context.Context, dir string, input toolshell.Input) (toolshell.Output, error) {
	return toolshell.Output{Stdout: []byte(dir + ":" + input.Cmd)}, nil
}

func TestWorkspaceStopResumeShutdown(t *testing.T) {
	source := t.TempDir()
	if err := os.Mkdir(filepath.Join(source, "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "nested", "note.txt"), []byte("before"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("nested/note.txt", filepath.Join(source, "note-link")); err != nil {
		t.Fatal(err)
	}
	store := &memorySnapshots{}
	config := Config{BaseDir: t.TempDir(), Store: store}
	workspace, err := newWorkspace(t.Context(), config, source, recordingRunner{})
	if err != nil {
		t.Fatal(err)
	}
	path, err := workspace.Path()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "nested", "note.txt"), []byte("after"), 0o640); err != nil {
		t.Fatal(err)
	}
	out, err := workspace.Run(t.Context(), toolshell.Input{Cmd: "pwd"})
	if err != nil || string(out.Stdout) != path+":pwd" {
		t.Fatalf("Run = (%q, %v)", out.Stdout, err)
	}

	id, err := workspace.Stop(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if same, err := workspace.Stop(t.Context()); err != nil || same != id {
		t.Fatalf("repeated Stop = (%q, %v), want (%q, nil)", same, err, id)
	}
	if _, err := workspace.Run(t.Context(), toolshell.Input{Cmd: "pwd"}); !errors.Is(err, ErrStopped) {
		t.Fatalf("Run after Stop error = %v, want ErrStopped", err)
	}

	restored, err := resumeWorkspace(t.Context(), config, id, recordingRunner{})
	if err != nil {
		t.Fatal(err)
	}
	restoredPath, _ := restored.Path()
	content, err := os.ReadFile(filepath.Join(restoredPath, "note-link"))
	if err != nil || string(content) != "after" {
		t.Fatalf("restored symlink content = (%q, %v)", content, err)
	}
	if err := restored.Shutdown(); err != nil {
		t.Fatal(err)
	}
	if err := restored.Shutdown(); err != nil {
		t.Fatalf("repeated Shutdown: %v", err)
	}
	if _, err := os.Stat(restoredPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("workspace still exists after Shutdown: %v", err)
	}
	if _, err := restored.Path(); !errors.Is(err, ErrShutdown) {
		t.Fatalf("Path after Shutdown error = %v, want ErrShutdown", err)
	}
	if err := workspace.Shutdown(); err != nil {
		t.Fatal(err)
	}
}

func TestResumeRejectsMissingAndCorruptSnapshots(t *testing.T) {
	store := &memorySnapshots{data: map[string][]byte{}}
	config := Config{BaseDir: t.TempDir(), Store: store}
	missing := identifySnapshot([]byte("missing"))
	if _, err := resumeWorkspace(t.Context(), config, missing, recordingRunner{}); !errors.Is(err, ErrSnapshotNotFound) {
		t.Fatalf("missing resume error = %v", err)
	}
	store.data[missing.String()] = []byte("corrupt")
	if _, err := resumeWorkspace(t.Context(), config, missing, recordingRunner{}); err == nil {
		t.Fatal("corrupt snapshot was accepted")
	}
}

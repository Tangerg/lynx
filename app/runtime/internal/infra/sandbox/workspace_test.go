package sandbox

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	toolshell "github.com/Tangerg/scope/tools/shell"
)

type recordingRunner struct{}

func (recordingRunner) Run(_ context.Context, dir string, input toolshell.Input) (toolshell.Output, error) {
	return toolshell.Output{Stdout: []byte(dir + ":" + input.Cmd)}, nil
}

func TestWorkspaceCopiesSourceAndShutsDown(t *testing.T) {
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
	workspace, err := newWorkspace(t.Context(), Config{BaseDir: t.TempDir()}, source, recordingRunner{})
	if err != nil {
		t.Fatal(err)
	}
	path, err := workspace.Path()
	if err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(filepath.Join(path, "note-link"))
	if err != nil || string(content) != "before" {
		t.Fatalf("copied symlink content = (%q, %v)", content, err)
	}
	out, err := workspace.Run(t.Context(), toolshell.Input{Cmd: "pwd"})
	if err != nil || string(out.Stdout) != path+":pwd" {
		t.Fatalf("Run = (%q, %v)", out.Stdout, err)
	}
	if err := workspace.Shutdown(); err != nil {
		t.Fatal(err)
	}
	if err := workspace.Shutdown(); err != nil {
		t.Fatalf("repeated Shutdown: %v", err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("workspace still exists after Shutdown: %v", err)
	}
	if _, err := workspace.Path(); !errors.Is(err, ErrShutdown) {
		t.Fatalf("Path after Shutdown error = %v, want ErrShutdown", err)
	}
}

func TestNewWorkspaceRequiresAbsoluteBaseDirectory(t *testing.T) {
	if _, err := newWorkspace(t.Context(), Config{BaseDir: "relative"}, "", recordingRunner{}); err == nil {
		t.Fatal("newWorkspace accepted a relative base directory")
	}
}

func TestWorkspaceCopyDoesNotMaterializeSourceInHeap(t *testing.T) {
	const sourceBytes = 16 << 20
	source := t.TempDir()
	file, err := os.Create(filepath.Join(source, "sparse.bin"))
	if err != nil {
		t.Fatal(err)
	}
	if truncateErr := file.Truncate(sourceBytes); truncateErr != nil {
		_ = file.Close()
		t.Fatal(truncateErr)
	}
	if closeErr := file.Close(); closeErr != nil {
		t.Fatal(closeErr)
	}

	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	workspace, err := newWorkspace(t.Context(), Config{BaseDir: t.TempDir()}, source, recordingRunner{})
	runtime.ReadMemStats(&after)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = workspace.Shutdown() }()

	allocated := after.TotalAlloc - before.TotalAlloc
	if allocated >= sourceBytes/2 {
		t.Fatalf("copying %d bytes allocated %d heap bytes; want chunk-bounded copy below %d", sourceBytes, allocated, sourceBytes/2)
	}
}

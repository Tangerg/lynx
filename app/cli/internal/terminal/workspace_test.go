package terminal

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Tangerg/oolong/core/input"

	"github.com/Tangerg/scope/app/cli/internal/agent"
	"github.com/Tangerg/scope/app/cli/internal/agent/mock"
	"github.com/Tangerg/scope/app/cli/internal/workbench"
)

func TestResolveWorkspaceUsesTheCurrentRootForRelativePathsAndRejectsFiles(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "nested")
	if err := os.Mkdir(nested, 0o700); err != nil {
		t.Fatal(err)
	}
	resolved, err := resolveWorkspace(root, "nested")
	if err != nil {
		t.Fatal(err)
	}
	expected, err := filepath.EvalSymlinks(nested)
	if err != nil {
		t.Fatal(err)
	}
	if resolved != expected {
		t.Fatalf("workspace = %s, want %s", resolved, expected)
	}
	file := filepath.Join(root, "not-a-directory")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveWorkspace(root, file); err == nil {
		t.Fatal("regular file was accepted as a workspace")
	}
}

type workspaceRecordingRuntime struct {
	*mock.Runtime
	created chan string
}

func (w *workspaceRecordingRuntime) CreateSession(ctx context.Context, input agent.CreateSession) (agent.Session, error) {
	w.created <- input.Workspace
	return w.Runtime.CreateSession(ctx, input)
}

func TestRecentWorkspacePickerCreatesAndSwitchesToTheSelectedRoot(t *testing.T) {
	root := t.TempDir()
	current := filepath.Join(root, "current")
	recent := filepath.Join(root, "recent-project")
	state := filepath.Join(root, "state")
	for _, directory := range []string{current, recent, state} {
		if err := os.Mkdir(directory, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	store, err := workbench.Open(state, workbench.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.RememberWorkspace(recent); err != nil {
		t.Fatal(err)
	}
	backend := &workspaceRecordingRuntime{Runtime: mock.New(), created: make(chan string, 4)}
	backend.Instant = true
	host, stop := runUIWithState(t, backend, current, "", state)
	host.Shows(t, "Ask lyra")
	if opened := <-backend.created; !samePath(opened, current) {
		t.Fatalf("opening workspace = %s, want %s", opened, current)
	}

	host.Type("/workspace")
	host.Press(input.Enter)
	host.Shows(t, "Workspaces")
	host.Type("recent-project")
	host.Shows(t, "recent-project")
	host.Press(input.Enter)
	if selected := <-backend.created; !samePath(selected, recent) {
		t.Fatalf("selected workspace = %s, want %s", selected, recent)
	}
	host.Shows(t, "session ·")
	stop()
}

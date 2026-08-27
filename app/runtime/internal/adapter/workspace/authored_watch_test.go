package workspace

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	workspaceapp "github.com/Tangerg/scope/app/runtime/internal/application/workspace"
)

func TestAuthoredWatcherMapsGlobalAndWorkspaceCascades(t *testing.T) {
	home := t.TempDir()
	knowledgeHome := t.TempDir()
	skillsHome := t.TempDir()
	project := t.TempDir()
	workspace := filepath.Join(project, "packages", "desktop")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	watcher, err := NewAuthoredWatcher(knowledgeHome, home, skillsHome)
	if err != nil {
		t.Fatal(err)
	}
	events := make(chan workspaceapp.AuthoredResource, 8)
	closer, err := watcher.Watch(
		[]workspaceapp.AuthoredScope{{Workspace: workspace, ProjectRoot: project}},
		[]workspaceapp.AuthoredResource{workspaceapp.AuthoredKnowledge, workspaceapp.AuthoredHooks, workspaceapp.AuthoredSkills},
		func(resource workspaceapp.AuthoredResource) { events <- resource },
	)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = closer.Close() }()

	for _, change := range []struct {
		path     string
		resource workspaceapp.AuthoredResource
	}{
		{filepath.Join(knowledgeHome, "SCOPEAPP.md"), workspaceapp.AuthoredKnowledge},
		{filepath.Join(project, "SCOPEAPP.md"), workspaceapp.AuthoredKnowledge},
		{filepath.Join(workspace, "SCOPEAPP.md"), workspaceapp.AuthoredKnowledge},
		{filepath.Join(home, ".scopeapp", "hooks.json"), workspaceapp.AuthoredHooks},
		{filepath.Join(project, ".scopeapp", "hooks.json"), workspaceapp.AuthoredHooks},
		{filepath.Join(workspace, ".scopeapp", "hooks.json"), workspaceapp.AuthoredHooks},
		{filepath.Join(skillsHome, "global-skill", "SKILL.md"), workspaceapp.AuthoredSkills},
		{filepath.Join(project, ".scopeapp", "skills", "project-skill", "SKILL.md"), workspaceapp.AuthoredSkills},
	} {
		if err := os.MkdirAll(filepath.Dir(change.path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(change.path, []byte(time.Now().String()), 0o644); err != nil {
			t.Fatal(err)
		}
		select {
		case got := <-events:
			if got != change.resource {
				t.Fatalf("change %s produced %v, want %v", change.path, got, change.resource)
			}
		case <-time.After(3 * time.Second):
			t.Fatalf("no resource event for %s", change.path)
		}
	}
}

func TestDirectoriesRootToLeafRejectsWorkspaceOutsideProject(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if _, err := directoriesRootToLeaf(root, outside); err == nil {
		t.Fatal("outside workspace was accepted")
	}
}

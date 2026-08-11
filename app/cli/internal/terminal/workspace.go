package terminal

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/layout"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
	"github.com/Tangerg/lynx/app/cli/internal/workbench"
)

type workspaceChoice struct {
	workspace workbench.Workspace
	current   bool
}

func (a *app) buildWorkspacePicker(theme kit.Theme, glyphs kit.Glyphs) {
	a.workspacePicker = newPicker(theme, glyphs, "search recent workspaces",
		func(choice workspaceChoice) string { return choice.workspace.Path },
		func(choice workspaceChoice) string {
			if choice.current {
				return "current"
			}
			return compactRelativeAge(choice.workspace.LastOpened)
		},
		func(choice workspaceChoice) {
			a.workspaceDialog.Dismiss()
			if err := a.createSessionInWorkspace(choice.workspace.Path); err != nil {
				a.message(err.Error())
			}
		},
	)
	a.workspaceDialog = kit.NewDialog(kit.DialogConfig{
		Stack: &a.stack, Theme: theme, Glyphs: glyphs, Title: "Workspaces", Body: a.workspacePicker,
		Where: layout.Placement{Width: 92, Height: 20},
	})
	a.workspacePicker.cancel = a.workspaceDialog.Dismiss
}

func (a *app) chooseWorkspace(argument string) error {
	if strings.TrimSpace(argument) != "" {
		return a.createSessionInWorkspace(argument)
	}
	workspaces := a.workbench.Workspaces()
	if len(workspaces) == 0 {
		return errors.New("there are no recent workspaces")
	}
	choices := make([]workspaceChoice, 0, len(workspaces))
	for _, workspace := range workspaces {
		choices = append(choices, workspaceChoice{
			workspace: workspace,
			current:   samePath(workspace.Path, a.session.Workspace),
		})
	}
	a.workspacePicker.Reset()
	a.workspacePicker.SetItems(choices)
	a.workspaceDialog.Show()
	a.status.note("choose a workspace")
	return nil
}

func (a *app) createSessionInWorkspace(requested string) error {
	workspace, err := resolveWorkspace(a.session.Workspace, requested)
	if err != nil {
		return err
	}
	a.startSessionInWorkspace(workspace)
	return nil
}

// startSessionInWorkspace accepts an already established workspace identity.
// Callers resolving new user input go through createSessionInWorkspace; /new
// reuses the runtime-authoritative workspace of the current session directly.
func (a *app) startSessionInWorkspace(workspace string) {
	runSessionChange(a, "creating session in "+workspace,
		func(ctx context.Context) (agent.SessionSnapshot, error) {
			created, err := a.runtime.CreateSession(ctx, agent.CreateSession{Workspace: workspace})
			return agent.SessionSnapshot{Session: created}, err
		},
		func(snapshot agent.SessionSnapshot) error { return a.installSnapshot(snapshot) },
	)
}

func resolveWorkspace(current, requested string) (string, error) {
	requested = strings.TrimSpace(requested)
	if requested == "" {
		return "", errors.New("workspace path is empty")
	}
	if requested == "~" || strings.HasPrefix(requested, "~"+string(filepath.Separator)) {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve workspace home: %w", err)
		}
		if requested == "~" {
			requested = home
		} else {
			requested = filepath.Join(home, strings.TrimPrefix(requested, "~"+string(filepath.Separator)))
		}
	}
	if !filepath.IsAbs(requested) {
		requested = filepath.Join(current, requested)
	}
	resolved, err := filepath.EvalSymlinks(filepath.Clean(requested))
	if err != nil {
		return "", fmt.Errorf("resolve workspace: %w", err)
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", fmt.Errorf("inspect workspace: %w", err)
	}
	if !info.IsDir() {
		return "", errors.New("workspace is not a directory")
	}
	return resolved, nil
}

func samePath(left, right string) bool {
	left, leftErr := canonicalPath(left)
	right, rightErr := canonicalPath(right)
	return leftErr == nil && rightErr == nil && left == right
}

func canonicalPath(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err == nil {
		absolute = resolved
	}
	return filepath.Clean(absolute), nil
}

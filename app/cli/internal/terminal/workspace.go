package terminal

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/layout"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
	"github.com/Tangerg/lynx/app/cli/internal/runtimeprofile"
	"github.com/Tangerg/lynx/app/cli/internal/workbench"
	"github.com/Tangerg/lynx/app/cli/internal/workspace"
)

type workspaceChoice struct {
	workspace workbench.Workspace
	current   bool
	available bool
	detail    string
}

func (a *app) buildWorkspacePicker(theme kit.Theme, glyphs kit.Glyphs) {
	a.workspacePicker = newPicker(theme, glyphs, "search recent workspaces",
		func(choice workspaceChoice) string { return choice.workspace.Path },
		func(choice workspaceChoice) string {
			if choice.current {
				return "current"
			}
			if choice.detail != "" {
				return choice.detail
			}
			return compactRelativeAge(choice.workspace.LastOpened)
		},
		func(choice workspaceChoice) {
			a.workspaceDialog.Dismiss()
			if !choice.available {
				a.message("workspace is no longer available · " + choice.workspace.Path)
				return
			}
			if a.workspaces != nil {
				a.resolveAndStartWorkspace(choice.workspace.Path)
				return
			}
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
		if a.workspaces != nil {
			a.resolveAndStartWorkspace(argument)
			return nil
		}
		return a.createSessionInWorkspace(argument)
	}
	if a.workspaces != nil {
		a.loadWorkspaceChoices()
		return nil
	}
	return a.showLocalWorkspaceChoices()
}

func (a *app) showLocalWorkspaceChoices() error {
	workspaces := a.workbench.Workspaces()
	if len(workspaces) == 0 {
		return errors.New("there are no recent workspaces")
	}
	choices := make([]workspaceChoice, 0, len(workspaces))
	for _, workspace := range workspaces {
		choices = append(choices, workspaceChoice{
			workspace: workspace,
			current:   samePath(workspace.Path, a.session.Workspace.Path),
			available: true,
		})
	}
	a.workspacePicker.Reset()
	a.workspacePicker.SetItems(choices)
	a.workspaceDialog.Show()
	a.status.note("choose a workspace")
	return nil
}

func (a *app) loadWorkspaceChoices() {
	a.status.note("loading runtime workspaces")
	runOperation(a, workspaceQueryOperation, true,
		func(ctx context.Context) ([]workspaceChoice, error) {
			known, err := a.workspaces.List(ctx)
			if err != nil {
				return nil, err
			}
			byPath := make(map[string]workspaceChoice, len(known))
			for _, summary := range known {
				lastOpened := time.Time{}
				if summary.LastActive != nil {
					lastOpened = *summary.LastActive
				}
				detail := fmt.Sprintf("%d sessions", summary.Sessions)
				if !summary.Workspace.IsAvailable() {
					detail += " · missing"
				}
				byPath[summary.Workspace.Path] = workspaceChoice{
					workspace: workbench.Workspace{Path: summary.Workspace.Path, LastOpened: lastOpened},
					current:   samePath(summary.Workspace.Path, a.session.Workspace.Path),
					available: summary.Workspace.IsAvailable(), detail: detail,
				}
			}
			for _, recent := range a.workbench.Workspaces() {
				if _, exists := byPath[recent.Path]; exists {
					continue
				}
				byPath[recent.Path] = workspaceChoice{
					workspace: recent, current: samePath(recent.Path, a.session.Workspace.Path), available: true,
				}
			}
			choices := make([]workspaceChoice, 0, len(byPath))
			for _, choice := range byPath {
				choices = append(choices, choice)
			}
			sort.Slice(choices, func(left, right int) bool {
				if choices[left].current != choices[right].current {
					return choices[left].current
				}
				return choices[left].workspace.LastOpened.After(choices[right].workspace.LastOpened)
			})
			return choices, nil
		},
		func(choices []workspaceChoice, err error) {
			if err != nil {
				a.message("could not load workspaces: " + err.Error())
				return
			}
			if len(choices) == 0 {
				a.message("there are no known workspaces")
				return
			}
			a.workspacePicker.Reset()
			a.workspacePicker.SetItems(choices)
			a.workspaceDialog.Show()
			a.status.note("choose a workspace")
		},
	)
}

func (a *app) resolveAndStartWorkspace(requested string) {
	path, err := resolveWorkspace(a.session.Workspace.Path, requested)
	if err != nil {
		a.message(err.Error())
		return
	}
	a.status.note("resolving workspace")
	runOperation(a, workspaceQueryOperation, true,
		func(ctx context.Context) (workspace.Workspace, error) {
			return a.workspaces.Resolve(ctx, workspace.ResolveRequest{Path: path})
		},
		func(resolved workspace.Workspace, err error) {
			if err != nil {
				a.message("resolve workspace: " + err.Error())
				return
			}
			if !resolved.IsAvailable() {
				a.message("workspace is unavailable · " + resolved.Path)
				return
			}
			a.startSessionInWorkspace(resolved.Path)
		},
	)
}

func (a *app) createSessionInWorkspace(requested string) error {
	workspace, err := resolveWorkspace(a.session.Workspace.Path, requested)
	if err != nil {
		return err
	}
	a.startSessionInWorkspace(workspace)
	return nil
}

func (a *app) RelocateSession(requested string) error {
	if err := a.requireRuntimeFeature(runtimeprofile.FeatureRelocate); err != nil {
		return err
	}
	path, err := resolveWorkspace(a.session.Workspace.Path, requested)
	if err != nil {
		return err
	}
	if a.workspaces == nil {
		a.relocateSession(path)
		return nil
	}
	a.status.note("resolving workspace")
	runOperation(a, workspaceQueryOperation, true,
		func(ctx context.Context) (workspace.Workspace, error) {
			return a.workspaces.Resolve(ctx, workspace.ResolveRequest{Path: path})
		},
		func(resolved workspace.Workspace, err error) {
			if err != nil {
				a.message("resolve workspace: " + err.Error())
				return
			}
			if !resolved.IsAvailable() {
				a.message("workspace is unavailable · " + resolved.Path)
				return
			}
			a.relocateSession(resolved.Path)
		},
	)
	return nil
}

func (a *app) relocateSession(path string) {
	if samePath(path, a.session.Workspace.Path) {
		a.message("session already uses " + path)
		return
	}
	sessionID := a.session.ID
	runSessionChange(a, "relocating session",
		func(ctx context.Context) (agent.SessionSnapshot, error) {
			latest, err := a.runtime.GetSession(ctx, sessionID)
			if err != nil {
				return agent.SessionSnapshot{}, err
			}
			if _, err := a.runtime.UpdateSession(ctx, agent.UpdateSession{
				SessionID: sessionID, Workspace: &path, ExpectedRevision: latest.Session.Revision,
			}); err != nil {
				return agent.SessionSnapshot{}, err
			}
			return a.runtime.GetSession(ctx, sessionID)
		},
		func(snapshot agent.SessionSnapshot) error { return a.installSnapshot(snapshot) },
	)
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

// Package workspacefs adapts local filesystem identity and content to the
// Workspace application boundary.
package workspacefs

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Tangerg/lynx/app2/runtime/domain/session"
)

type Resolution struct {
	Workspace    session.Workspace
	ProjectRoot  string
	Available    bool
}

type Resolver struct{ defaultPath string }

func NewResolver(defaultPath string) (*Resolver, error) {
	workspace, err := session.NewWorkspace(defaultPath)
	if err != nil {
		return nil, fmt.Errorf("workspacefs: default workspace: %w", err)
	}
	return &Resolver{defaultPath: workspace.Path()}, nil
}

func (resolver *Resolver) Resolve(ctx context.Context, requested string) (Resolution, error) {
	if err := ctx.Err(); err != nil {
		return Resolution{}, err
	}
	path := strings.TrimSpace(requested)
	if path == "" {
		path = resolver.defaultPath
	}
	workspace, err := session.NewWorkspace(path)
	if err != nil {
		return Resolution{}, err
	}
	info, err := os.Stat(workspace.Path())
	if errors.Is(err, os.ErrNotExist) {
		return Resolution{Workspace: workspace, ProjectRoot: workspace.Path()}, nil
	}
	if err != nil {
		return Resolution{}, fmt.Errorf("workspacefs: inspect %s: %w", workspace.Path(), err)
	}
	if !info.IsDir() {
		return Resolution{}, fmt.Errorf("workspacefs: %s is not a directory", workspace.Path())
	}
	real, err := filepath.EvalSymlinks(workspace.Path())
	if err != nil {
		return Resolution{}, fmt.Errorf("workspacefs: resolve %s: %w", workspace.Path(), err)
	}
	workspace, err = session.NewWorkspace(real)
	if err != nil {
		return Resolution{}, err
	}
	return Resolution{
		Workspace: workspace, ProjectRoot: findProjectRoot(workspace.Path()), Available: true,
	}, nil
}

func findProjectRoot(path string) string {
	current := path
	for {
		if _, err := os.Lstat(filepath.Join(current, ".git")); err == nil {
			return current
		}
		parent := filepath.Dir(current)
		if parent == current {
			return path
		}
		current = parent
	}
}

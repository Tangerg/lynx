package workspace

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/knowledge"
)

// ErrKnowledgeUnavailable reports that this runtime has no knowledge store.
var ErrKnowledgeUnavailable = errors.New("workspace: knowledge unavailable")

// KnowledgeStore is the complete persistence surface consumed by Knowledge.
type KnowledgeStore interface {
	Get(ctx context.Context, scope knowledge.Scope, dir string) (string, error)
	Update(ctx context.Context, scope knowledge.Scope, dir string, content string) error
	List(ctx context.Context, cwd, projectRoot string) ([]knowledge.Entry, error)
}

// KnowledgeWorkspaceInspector supplies the one live identity fact needed to
// distinguish a nested workspace root from its project-discovery root.
type KnowledgeWorkspaceInspector interface {
	Inspect(path string) (Resolved, error)
}

// Knowledge owns the human-authored LYRA.md cascade use cases.
type Knowledge struct {
	scope      *Scope
	workspaces KnowledgeWorkspaceInspector
	store      KnowledgeStore
}

func NewKnowledge(scope *Scope, workspaces KnowledgeWorkspaceInspector, store KnowledgeStore) *Knowledge {
	return &Knowledge{scope: scope, workspaces: workspaces, store: store}
}

// Available reports whether this runtime has a long-term knowledge store.
func (k *Knowledge) Available() bool { return k != nil && k.store != nil }

// Entries enumerates LYRA.md entries across scopes.
func (k *Knowledge) Entries(ctx context.Context, cwd string) ([]knowledge.Entry, error) {
	if k.store == nil {
		return nil, ErrKnowledgeUnavailable
	}
	root, err := k.scope.root(cwd)
	if err != nil {
		return nil, err
	}
	projectRoot, err := k.projectRoot(root)
	if err != nil {
		return nil, err
	}
	return k.store.List(ctx, root, projectRoot)
}

// Read returns the LYRA.md content for one scope.
func (k *Knowledge) Read(ctx context.Context, scope knowledge.Scope, cwd string) (string, error) {
	if err := scope.Validate(); err != nil {
		return "", err
	}
	if k.store == nil {
		return "", ErrKnowledgeUnavailable
	}
	if scope == knowledge.ScopeHome {
		return k.store.Get(ctx, scope, "")
	}
	root, err := k.scope.root(cwd)
	if err != nil {
		return "", err
	}
	if scope == knowledge.ScopeProjectRoot {
		root, err = k.projectRoot(root)
		if err != nil {
			return "", err
		}
	}
	return k.store.Get(ctx, scope, root)
}

// Update overwrites the LYRA.md content for one scope.
func (k *Knowledge) Update(ctx context.Context, scope knowledge.Scope, cwd, content string) error {
	if err := scope.Validate(); err != nil {
		return err
	}
	if k.store == nil {
		return ErrKnowledgeUnavailable
	}
	if scope == knowledge.ScopeHome {
		return k.store.Update(ctx, scope, "", content)
	}
	root, err := k.scope.root(cwd)
	if err != nil {
		return err
	}
	if scope == knowledge.ScopeProjectRoot {
		root, err = k.projectRoot(root)
		if err != nil {
			return err
		}
	}
	return k.store.Update(ctx, scope, root, content)
}

func (k *Knowledge) projectRoot(cwd string) (string, error) {
	if k.workspaces == nil {
		return "", fmt.Errorf("%w: workspace inspector is not configured", ErrCWDUnavailable)
	}
	resolved, err := k.workspaces.Inspect(cwd)
	if err != nil {
		return "", fmt.Errorf("%w: inspect %s: %w", ErrCWDUnavailable, cwd, err)
	}
	if resolved.Missing || resolved.ProjectRoot == "" {
		return "", fmt.Errorf("%w: %s", ErrCWDUnavailable, cwd)
	}
	return resolved.ProjectRoot, nil
}

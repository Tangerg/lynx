package workspace

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/scope/app/runtime/internal/application/invalidation"
	"github.com/Tangerg/scope/app/runtime/internal/domain/knowledge"
)

// ErrKnowledgeUnavailable reports that this runtime has no knowledge store.
var ErrKnowledgeUnavailable = errors.New("workspace: knowledge unavailable")

// KnowledgeStore is the complete persistence surface consumed by Knowledge.
type KnowledgeStore interface {
	Get(ctx context.Context, scope knowledge.Scope, dir string) (knowledge.Entry, error)
	Update(ctx context.Context, scope knowledge.Scope, dir, expectedRevision, content string) (knowledge.Entry, error)
	List(ctx context.Context, cwd, projectRoot string) ([]knowledge.Entry, error)
}

// KnowledgeWorkspaceInspector supplies the one live identity fact needed to
// distinguish a nested workspace root from its project-discovery root.
type KnowledgeWorkspaceInspector interface {
	Inspect(path string) (Resolved, error)
}

// Knowledge owns the human-authored LYRA.md cascade use cases.
type Knowledge struct {
	scope         *Scope
	workspaces    KnowledgeWorkspaceInspector
	store         KnowledgeStore
	observations  *AuthoredWatch
	invalidations invalidation.Publish
}

func NewKnowledge(
	scope *Scope,
	workspaces KnowledgeWorkspaceInspector,
	store KnowledgeStore,
	observations *AuthoredWatch,
	invalidations invalidation.Publish,
) *Knowledge {
	return &Knowledge{
		scope: scope, workspaces: workspaces, store: store,
		observations: observations, invalidations: invalidations,
	}
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
	entries, err := k.store.List(ctx, root, projectRoot)
	return entries, knowledgePathError(err)
}

// Read returns the LYRA.md content for one scope.
func (k *Knowledge) Read(ctx context.Context, scope knowledge.Scope, cwd string) (knowledge.Entry, error) {
	if err := scope.Validate(); err != nil {
		return knowledge.Entry{}, err
	}
	if k.store == nil {
		return knowledge.Entry{}, ErrKnowledgeUnavailable
	}
	if scope == knowledge.ScopeHome {
		entry, err := k.store.Get(ctx, scope, "")
		return entry, knowledgePathError(err)
	}
	root, err := k.scope.root(cwd)
	if err != nil {
		return knowledge.Entry{}, err
	}
	if scope == knowledge.ScopeProjectRoot {
		root, err = k.projectRoot(root)
		if err != nil {
			return knowledge.Entry{}, err
		}
	}
	entry, err := k.store.Get(ctx, scope, root)
	return entry, knowledgePathError(err)
}

// Update conditionally replaces one LYRA.md document and returns the committed fact.
func (k *Knowledge) Update(ctx context.Context, scope knowledge.Scope, cwd, expectedRevision, content string) (knowledge.Entry, error) {
	if err := scope.Validate(); err != nil {
		return knowledge.Entry{}, err
	}
	if expectedRevision == "" {
		return knowledge.Entry{}, knowledge.ErrRevisionRequired
	}
	if err := knowledge.ValidateDocument(content); err != nil {
		return knowledge.Entry{}, err
	}
	if k.store == nil {
		return knowledge.Entry{}, ErrKnowledgeUnavailable
	}
	if scope == knowledge.ScopeHome {
		return k.update(ctx, scope, "", expectedRevision, content)
	}
	root, err := k.scope.root(cwd)
	if err != nil {
		return knowledge.Entry{}, err
	}
	if scope == knowledge.ScopeProjectRoot {
		root, err = k.projectRoot(root)
		if err != nil {
			return knowledge.Entry{}, err
		}
	}
	return k.update(ctx, scope, root, expectedRevision, content)
}

func (k *Knowledge) update(ctx context.Context, scope knowledge.Scope, root, expectedRevision, content string) (knowledge.Entry, error) {
	entry, err := k.store.Update(ctx, scope, root, expectedRevision, content)
	if err == nil {
		if k.observations != nil {
			k.observations.Accept(AuthoredChange{
				Resource: AuthoredKnowledge, Identities: []string{entry.Path},
			})
		}
		k.invalidations.Notify(invalidation.Notice{Resource: invalidation.Knowledge})
	}
	return entry, knowledgePathError(err)
}

func knowledgePathError(err error) error {
	if errors.Is(err, knowledge.ErrPathOutsideScope) {
		return ErrPathOutsideRoot
	}
	return err
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

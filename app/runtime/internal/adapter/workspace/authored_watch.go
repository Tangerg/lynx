package workspace

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	workspaceapp "github.com/Tangerg/lynx/app/runtime/internal/application/workspace"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/fileobservation"
)

const (
	authoredKnowledgeKey = "knowledge"
	authoredHooksKey     = "hooks"
)

// AuthoredWatcher maps the Knowledge and Hooks filesystem layouts onto the
// workspace application's semantic observation port. Global roots are fixed at
// process composition; request-owned workspace roots arrive from Application.
type AuthoredWatcher struct {
	knowledgeHome string
	hooksHome     string
}

var _ workspaceapp.AuthoredResourceWatcher = AuthoredWatcher{}

// NewAuthoredWatcher binds the global Knowledge storage root and OS user home
// explicitly. They are intentionally distinct product locations.
func NewAuthoredWatcher(knowledgeHome, hooksHome string) (AuthoredWatcher, error) {
	for name, path := range map[string]string{"knowledge home": knowledgeHome, "hooks home": hooksHome} {
		if path == "" || !filepath.IsAbs(path) {
			return AuthoredWatcher{}, fmt.Errorf("workspace authored watcher: %s must be absolute", name)
		}
	}
	return AuthoredWatcher{
		knowledgeHome: filepath.Clean(knowledgeHome),
		hooksHome:     filepath.Clean(hooksHome),
	}, nil
}

// Watch observes only the selected resources. Filenames and cascade expansion
// belong to this filesystem translation; consumers see semantic resources.
func (w AuthoredWatcher) Watch(
	scopes []workspaceapp.AuthoredScope,
	resources []workspaceapp.AuthoredResource,
	notify func(workspaceapp.AuthoredResource),
) (workspaceapp.AuthoredObservation, error) {
	targets := make([]fileobservation.Target, 0, 2+len(scopes)*4)
	if slices.Contains(resources, workspaceapp.AuthoredKnowledge) {
		targets = append(targets, knowledgeTarget(w.knowledgeHome))
		for _, scope := range scopes {
			targets = append(targets, knowledgeTarget(scope.ProjectRoot), knowledgeTarget(scope.Workspace))
		}
	}
	if slices.Contains(resources, workspaceapp.AuthoredHooks) {
		targets = append(targets, fileobservation.Target{
			Key: authoredHooksKey, Path: filepath.Join(w.hooksHome, ".lyra", "hooks.json"),
		})
		for _, scope := range scopes {
			directories, err := directoriesRootToLeaf(scope.ProjectRoot, scope.Workspace)
			if err != nil {
				return nil, err
			}
			for _, directory := range directories {
				targets = append(targets, fileobservation.Target{
					Key: authoredHooksKey, Path: filepath.Join(directory, ".lyra", "hooks.json"),
				})
			}
		}
	}
	observation, err := fileobservation.Watch(targets, func(keys []string) {
		for _, key := range keys {
			switch key {
			case authoredKnowledgeKey:
				notify(workspaceapp.AuthoredKnowledge)
			case authoredHooksKey:
				notify(workspaceapp.AuthoredHooks)
			}
		}
	})
	if err != nil {
		return nil, err
	}
	return &authoredObservation{files: observation}, nil
}

type authoredObservation struct {
	files fileobservation.Observation
}

func (o *authoredObservation) Close() error { return o.files.Close() }

func (o *authoredObservation) Accept(changes []workspaceapp.AuthoredChange) error {
	keys := make([]string, 0, len(changes))
	identities := make([]string, 0, len(changes))
	for _, change := range changes {
		switch change.Resource {
		case workspaceapp.AuthoredKnowledge:
			keys = append(keys, authoredKnowledgeKey)
		case workspaceapp.AuthoredHooks:
			keys = append(keys, authoredHooksKey)
		}
		identities = append(identities, change.Identities...)
	}
	return o.files.Accept(keys, identities)
}

func knowledgeTarget(root string) fileobservation.Target {
	return fileobservation.Target{
		Key: authoredKnowledgeKey, Path: filepath.Join(root, "LYRA.md"), Boundary: root,
	}
}

func directoriesRootToLeaf(root, leaf string) ([]string, error) {
	root = filepath.Clean(root)
	leaf = filepath.Clean(leaf)
	relative, err := filepath.Rel(root, leaf)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return nil, fmt.Errorf("workspace authored watcher: workspace %q is outside project root %q", leaf, root)
	}
	chain := []string{leaf}
	for current := leaf; current != root; {
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		chain = append(chain, parent)
		current = parent
	}
	slices.Reverse(chain)
	return chain, nil
}

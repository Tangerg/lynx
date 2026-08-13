package workspace

import (
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	skillspec "github.com/Tangerg/lynx/skills"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/promptsource"
	workspaceapp "github.com/Tangerg/lynx/app/runtime/internal/application/workspace"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/fileobservation"
)

const (
	authoredKnowledgeKey = "knowledge"
	authoredHooksKey     = "hooks"
	authoredSkillsKey    = "skills"
)

// AuthoredWatcher maps the Knowledge, Hooks, and Skills filesystem layouts onto the
// workspace application's semantic observation port. Global roots are fixed at
// process composition; request-owned workspace roots arrive from Application.
type AuthoredWatcher struct {
	knowledgeHome string
	hooksHome     string
	skillsHome    string
}

var _ workspaceapp.AuthoredResourceWatcher = AuthoredWatcher{}

// NewAuthoredWatcher binds the global Knowledge, Hooks, and Skills roots
// explicitly. They are intentionally distinct product locations. An empty
// Skills root disables only global Skill observation; project Skills remain
// observable from request scopes.
func NewAuthoredWatcher(knowledgeHome, hooksHome, skillsHome string) (AuthoredWatcher, error) {
	for name, path := range map[string]string{"knowledge home": knowledgeHome, "hooks home": hooksHome} {
		if path == "" || !filepath.IsAbs(path) {
			return AuthoredWatcher{}, fmt.Errorf("workspace authored watcher: %s must be absolute", name)
		}
	}
	if skillsHome != "" && !filepath.IsAbs(skillsHome) {
		return AuthoredWatcher{}, errors.New("workspace authored watcher: skills home must be absolute when set")
	}
	return AuthoredWatcher{
		knowledgeHome: filepath.Clean(knowledgeHome),
		hooksHome:     filepath.Clean(hooksHome),
		skillsHome:    cleanOptionalPath(skillsHome),
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
	files, err := fileobservation.Watch(targets, func(keys []string) {
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
	trees, err := fileobservation.WatchTrees(w.skillTreeTargets(scopes, resources), func(keys []string) {
		if slices.Contains(keys, authoredSkillsKey) {
			notify(workspaceapp.AuthoredSkills)
		}
	})
	if err != nil {
		return nil, errors.Join(err, files.Close())
	}
	return &authoredObservation{observations: []fileobservation.Observation{files, trees}}, nil
}

type authoredObservation struct {
	observations []fileobservation.Observation
}

func (o *authoredObservation) Close() error {
	var errs []error
	for _, observation := range o.observations {
		errs = append(errs, observation.Close())
	}
	return errors.Join(errs...)
}

func (o *authoredObservation) Accept(changes []workspaceapp.AuthoredChange) error {
	keys := make([]string, 0, len(changes))
	identities := make([]string, 0, len(changes))
	for _, change := range changes {
		switch change.Resource {
		case workspaceapp.AuthoredKnowledge:
			keys = append(keys, authoredKnowledgeKey)
		case workspaceapp.AuthoredHooks:
			keys = append(keys, authoredHooksKey)
		case workspaceapp.AuthoredSkills:
			keys = append(keys, authoredSkillsKey)
		}
		identities = append(identities, change.Identities...)
	}
	var errs []error
	for _, observation := range o.observations {
		errs = append(errs, observation.Accept(keys, identities))
	}
	return errors.Join(errs...)
}

func (w AuthoredWatcher) skillTreeTargets(scopes []workspaceapp.AuthoredScope, resources []workspaceapp.AuthoredResource) []fileobservation.TreeTarget {
	if !slices.Contains(resources, workspaceapp.AuthoredSkills) {
		return nil
	}
	targets := make([]fileobservation.TreeTarget, 0, len(scopes)+1)
	if w.skillsHome != "" {
		targets = append(targets, fileobservation.TreeTarget{
			Key: authoredSkillsKey, Path: w.skillsHome, Boundary: w.skillsHome, FileName: skillspec.SkillFile,
		})
	}
	for _, scope := range scopes {
		targets = append(targets, fileobservation.TreeTarget{
			Key:  authoredSkillsKey,
			Path: promptsource.ProjectSkillDir(scope.ProjectRoot), Boundary: scope.ProjectRoot,
			FileName: skillspec.SkillFile,
		})
	}
	return targets
}

func cleanOptionalPath(path string) string {
	if path == "" {
		return ""
	}
	return filepath.Clean(path)
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

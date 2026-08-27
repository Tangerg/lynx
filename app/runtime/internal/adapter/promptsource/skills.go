package promptsource

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"

	sdk "github.com/Tangerg/scope/skills"

	workspaceapp "github.com/Tangerg/scope/app/runtime/internal/application/workspace"
)

const projectSkillsSubdir = ".lyra/skills"

// ProjectSkillDir resolves the project skill-source directory for a working
// directory. The .lyra layout is a prompt-source filesystem convention, not a
// skills-domain concern.
func ProjectSkillDir(cwd string) string {
	if cwd == "" {
		return ""
	}
	return filepath.Join(cwd, projectSkillsSubdir)
}

// MergeSkillSource builds the merged skill source: projectDir layered over
// userDir, the project copy winning on name collisions. Returns nil when
// neither directory exists, so a session that ships no skills gets no skill tool
// at all rather than one that always lists nothing.
//
// decorateUser, when non-nil, wraps the USER source only (e.g. to record
// loads for the idle-lifecycle curator). It must not wrap the project source:
// only the user library is auto-curated, and merge resolves a shadowed
// name to the project copy, so decorating the user source records exactly the
// user-resolved loads and nothing else.
//
// Building a source just wraps an os.DirFS, so this is cheap enough to call per
// tool resolution (the engine rebuilds the skill tool per Run cwd).
func MergeSkillSource(projectDir, userDir string, decorateUser func(sdk.ResourceSource) sdk.ResourceSource) sdk.ResourceSource {
	var sources []sdk.ResourceSource
	if dirExists(projectDir) {
		sources = append(sources, newRuntimeSkillSource(projectDir))
	}
	if dirExists(userDir) {
		user := newRuntimeSkillSource(userDir)
		if decorateUser != nil {
			user = decorateUser(user)
		}
		sources = append(sources, user)
	}
	if len(sources) == 0 {
		return nil
	}
	return sdk.Merge(sources...)
}

// ListSkills enumerates the skills visible from projectDir layered over
// userDir, project winning on a name collision (the same precedence
// MergeSkillSource gives the model). A missing directory contributes nothing
// rather than erroring. Result is sorted by name.
func ListSkills(ctx context.Context, projectDir, userDir string) ([]workspaceapp.SkillSummary, error) {
	seen := make(map[string]struct{})
	var out []workspaceapp.SkillSummary
	add := func(dir string, scope workspaceapp.SkillScope) error {
		if !dirExists(dir) {
			return nil
		}
		summaries, err := newRuntimeSkillSource(dir).List(ctx)
		if err != nil {
			return err
		}
		for _, s := range summaries {
			if _, dup := seen[s.Name]; dup {
				continue // a higher-precedence (project) source already provided it
			}
			seen[s.Name] = struct{}{}
			out = append(out, workspaceapp.SkillSummary{Name: s.Name, Description: s.Description, Scope: scope})
		}
		return nil
	}
	if err := add(projectDir, workspaceapp.SkillScopeProject); err != nil {
		return nil, err
	}
	if err := add(userDir, workspaceapp.SkillScopeUser); err != nil {
		return nil, err
	}
	slices.SortFunc(out, func(a, b workspaceapp.SkillSummary) int { return strings.Compare(a.Name, b.Name) })
	return out, nil
}

// dirExists reports whether path names an existing directory.
func dirExists(path string) bool {
	if path == "" {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

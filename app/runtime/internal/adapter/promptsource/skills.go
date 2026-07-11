package promptsource

import (
	"context"
	"os"
	"slices"
	"strings"

	sdk "github.com/Tangerg/lynx/skills"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/skills"
)

// MergeSkillSource builds the merged skill source: projectDir layered over
// globalDir, the project copy winning on name collisions. Returns nil when
// neither directory exists, so a session that ships no skills gets no skill tool
// at all rather than one that always lists nothing.
//
// Building a source just wraps an os.DirFS, so this is cheap enough to call per
// tool resolution (the engine rebuilds the skill tool per turn cwd).
func MergeSkillSource(projectDir, globalDir string) sdk.ResourceSource {
	var sources []sdk.ResourceSource
	if dirExists(projectDir) {
		sources = append(sources, sdk.Dir(projectDir))
	}
	if dirExists(globalDir) {
		sources = append(sources, sdk.Dir(globalDir))
	}
	if len(sources) == 0 {
		return nil
	}
	return sdk.Merge(sources...)
}

// ListSkills enumerates the skills visible from projectDir layered over
// globalDir, project winning on a name collision (the same precedence
// MergeSkillSource gives the model). A missing directory contributes nothing
// rather than erroring. Result is sorted by name.
func ListSkills(ctx context.Context, projectDir, globalDir string) ([]skills.Info, error) {
	seen := make(map[string]struct{})
	var out []skills.Info
	add := func(dir, scope string) error {
		if !dirExists(dir) {
			return nil
		}
		summaries, err := sdk.Dir(dir).List(ctx)
		if err != nil {
			return err
		}
		for _, s := range summaries {
			if _, dup := seen[s.Name]; dup {
				continue // a higher-precedence (project) source already provided it
			}
			seen[s.Name] = struct{}{}
			out = append(out, skills.Info{Name: s.Name, Description: s.Description, Scope: scope})
		}
		return nil
	}
	if err := add(projectDir, "project"); err != nil {
		return nil, err
	}
	if err := add(globalDir, "global"); err != nil {
		return nil, err
	}
	slices.SortFunc(out, func(a, b skills.Info) int { return strings.Compare(a.Name, b.Name) })
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

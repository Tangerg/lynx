// Package skill provides the skill tool — progressive-disclosure access to the
// SKILL.md skills visible from a turn's working directory. One tool, one
// package. It is working-directory scoped, so it's rebuilt per resolution.
package skill

import (
	"context"
	"time"

	skillspec "github.com/Tangerg/lynx/skills"
	"github.com/Tangerg/lynx/tools"
	skillstool "github.com/Tangerg/lynx/tools/skills"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/promptsource"
)

// UsageRecorder records that a skill was loaded, feeding the idle-lifecycle
// curator's last-used signal. The composition root supplies the authoring store;
// nil disables use recording (a session that ships no authoring store).
type UsageRecorder interface {
	RecordUse(ctx context.Context, name string, now time.Time) error
}

// Build assembles the working-directory-scoped skill tool over the merged skill
// source (project <workdir>/.lyra/skills layered over the global dir, project
// winning). It returns nil when neither directory exists, so a session that
// ships no skills gets no skill tool at all. When recorder is non-nil, loading a
// skill records a use so the curator can tell active skills from idle ones.
//
// Rebuilt per resolution like fs/shell, because the project directory depends on
// the turn's working directory; the merged source just wraps os.DirFS, so the
// cost is negligible.
func Build(workdir, globalDir string, recorder UsageRecorder) tools.Tool {
	var decorateGlobal func(skillspec.ResourceSource) skillspec.ResourceSource
	if recorder != nil {
		// Wrap only the global source: the curator governs the global library, and
		// merge resolves a shadowed name to the project copy, so this records
		// exactly the global-resolved loads (a project skill never touches the
		// global usage record).
		decorateGlobal = func(global skillspec.ResourceSource) skillspec.ResourceSource {
			return recordingSource{ResourceSource: global, recorder: recorder}
		}
	}
	source := promptsource.MergeSkillSource(promptsource.ProjectSkillDir(workdir), globalDir, decorateGlobal)
	if source == nil {
		return nil
	}
	// source is non-nil, so NewTool cannot fail; the error is checked only to
	// satisfy the signature.
	tool, err := skillstool.NewTool(source)
	if err != nil {
		return nil
	}
	return tool
}

// recordingSource records a use each time a (global-library) skill loads, then
// delegates. The record is best-effort: a usage-write failure never fails the
// skill load.
type recordingSource struct {
	skillspec.ResourceSource
	recorder UsageRecorder
}

func (r recordingSource) Load(ctx context.Context, name string) (*skillspec.Skill, error) {
	skill, err := r.ResourceSource.Load(ctx, name)
	if err == nil {
		_ = r.recorder.RecordUse(ctx, name, time.Now())
	}
	return skill, err
}

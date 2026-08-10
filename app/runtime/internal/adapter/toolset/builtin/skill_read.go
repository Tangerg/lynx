// Skill readers provide model-facing Skill discovery and resource access.
package builtin

import (
	"context"
	"fmt"
	"time"

	toolcontract "github.com/Tangerg/lynx/tool"

	skillspec "github.com/Tangerg/lynx/skills"
	skillstool "github.com/Tangerg/lynx/tools/skills"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/promptsource"
)

// SkillUsageRecorder records that a skill was loaded, feeding the idle-lifecycle
// curator's last-used signal. nil disables use recording.
type SkillUsageRecorder interface {
	RecordUse(ctx context.Context, name string, now time.Time) error
}

// BuildReaders assembles the working-directory-scoped reading tools over the
// merged skill source (project <cwd>/.lyra/skills layered over the user dir,
// project winning). It returns nil when neither directory exists, so a session
// that ships no skills gets no skill tools at all. When recorder is non-nil,
// loading a skill records a use so the curator can tell active skills from idle
// ones.
//
// Rebuilt per resolution like fs/shell, because the project directory depends on
// the Run's working directory; the merged source just wraps os.DirFS, so the
// cost is negligible.
func BuildReaders(cwd, userDir string, recorder SkillUsageRecorder) ([]toolcontract.Tool, error) {
	var decorateUser func(skillspec.ResourceSource) skillspec.ResourceSource
	if recorder != nil {
		// Wrap only the user source: the curator governs the user library, and
		// merge resolves a shadowed name to the project copy, so this records
		// exactly the user-resolved loads (a project skill never touches the
		// user-library usage record).
		decorateUser = func(user skillspec.ResourceSource) skillspec.ResourceSource {
			return recordingSource{ResourceSource: user, recorder: recorder}
		}
	}
	source := promptsource.MergeSkillSource(promptsource.ProjectSkillDir(cwd), userDir, decorateUser)
	if source == nil {
		return nil, nil
	}
	tools, err := skillstool.NewTools(source)
	if err != nil {
		return nil, fmt.Errorf("skill: build tools: %w", err)
	}
	return tools, nil
}

// recordingSource records a use each time a user-library Skill loads, then
// delegates. The record is best-effort: a usage-write failure never fails the
// skill load.
type recordingSource struct {
	skillspec.ResourceSource
	recorder SkillUsageRecorder
}

func (r recordingSource) Load(ctx context.Context, name string) (*skillspec.Skill, error) {
	skill, err := r.ResourceSource.Load(ctx, name)
	if err == nil {
		_ = r.recorder.RecordUse(ctx, name, time.Now())
	}
	return skill, err
}

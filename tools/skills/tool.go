package skills

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/samber/lo"

	toolcontract "github.com/Tangerg/scope/core/tool"
	skillsrc "github.com/Tangerg/scope/skills"
)

// LoadSkillRequest names a skill exactly. Loading is by name rather than by a
// query so a model cannot broaden what it loads, and so the same prompt
// resolves to the same skill on every run.
type LoadSkillRequest struct {
	Name string `json:"name" jsonschema:"minLength=1" jsonschema_description:"Exact skill name returned by list_skills or already known from the prompt."`
}

// ReadSkillResourceRequest carries the skill name alongside the resource path,
// so a read is confined to the skill that referenced it. A path alone would
// let one skill read another's files.
type ReadSkillResourceRequest struct {
	Name string `json:"name" jsonschema:"minLength=1" jsonschema_description:"Exact name of the skill whose instructions referenced the resource."`
	Path string `json:"path" jsonschema:"minLength=1" jsonschema_description:"Resource path relative to the skill directory, such as references/REFERENCE.md or scripts/run.py."`
}

type toolSet struct {
	source         skillsrc.ResourceSource
	maxOutputBytes int64
}

// Exported defaults keep constructor behavior visible and overridable.
const (
	DefaultMaxOutputBytes = int64(256 * 1024)
	minimumMaxOutputBytes = int64(len("<available_skills>\n  <truncated>true</truncated>\n</available_skills>"))
	maximumMaxOutputBytes = int64(^uint(0)>>1) - 1
	truncationMarker      = "\n\n[output truncated]"
)

// Config bounds how much skill content may enter a model's context. The limit
// lives here rather than in the repository because the same skill set is
// reasonable or oversized depending on the model it is fed to.
type Config struct {
	MaxOutputBytes int64
}

// NewTools returns the listing, loading, and resource-reading tools together
// because they are only useful as a set: a model that can load a skill but not
// read the resources its instructions reference will follow a broken pointer.
func NewTools(source skillsrc.ResourceSource, config Config) ([]toolcontract.Tool, error) {
	if lo.IsNil(source) {
		return nil, ErrNilSource
	}
	if config.MaxOutputBytes < 0 {
		return nil, fmt.Errorf("%w: maximum output bytes must not be negative", skillsrc.ErrInvalidLimit)
	}
	maxOutputBytes := config.MaxOutputBytes
	if maxOutputBytes == 0 {
		maxOutputBytes = DefaultMaxOutputBytes
	}
	if maxOutputBytes < minimumMaxOutputBytes || maxOutputBytes > maximumMaxOutputBytes {
		return nil, fmt.Errorf(
			"%w: maximum output bytes must be between %d and %d",
			skillsrc.ErrInvalidLimit,
			minimumMaxOutputBytes,
			maximumMaxOutputBytes,
		)
	}
	set := &toolSet{source: source, maxOutputBytes: maxOutputBytes}
	list, err := toolcontract.NewFunc(
		toolcontract.FuncConfig{
			Name: "list_skills",
			Description: "List every skill visible to the current workspace as a name and description. " +
				"Use this when a relevant skill may exist but its exact name is unknown. Load a matching skill with load_skill before following it.",
		},
		set.list,
	)
	if err != nil {
		return nil, fmt.Errorf("skills: build list_skills: %w", err)
	}
	load, err := toolcontract.NewFunc(
		toolcontract.FuncConfig{
			Name: "load_skill",
			Description: "Load the complete instructions for one exact skill name. Call this when the task matches that skill's description, " +
				"then follow the returned instructions. Use read_skill_resource only for a bundled file referenced by those instructions.",
		},
		set.load,
	)
	if err != nil {
		return nil, fmt.Errorf("skills: build load_skill: %w", err)
	}
	readResource, err := toolcontract.NewFunc(
		toolcontract.FuncConfig{
			Name: "read_skill_resource",
			Description: "Read one bundled resource referenced by a loaded skill. The path is relative to that skill's directory. " +
				"This returns file contents only and never executes scripts.",
		},
		set.readResource,
	)
	if err != nil {
		return nil, fmt.Errorf("skills: build read_skill_resource: %w", err)
	}
	return []toolcontract.Tool{list, load, readResource}, nil
}

func (t *toolSet) list(ctx context.Context, _ struct{}) (string, error) {
	summaries, err := t.source.List(ctx)
	if err != nil {
		return "", err
	}
	return renderSummaries(summaries, t.maxOutputBytes), nil
}

func (t *toolSet) load(ctx context.Context, request LoadSkillRequest) (string, error) {
	skill, err := t.source.Load(ctx, request.Name)
	if err != nil {
		return "", err
	}
	return boundedText(skill.Instructions, false, t.maxOutputBytes), nil
}

func (t *toolSet) readResource(ctx context.Context, request ReadSkillResourceRequest) (string, error) {
	data, truncated, err := skillsrc.ReadResource(ctx, t.source, request.Name, request.Path, t.maxOutputBytes)
	if err != nil {
		return "", err
	}
	return boundedText(string(data), truncated, t.maxOutputBytes), nil
}

var xmlEscaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")

func renderSummaries(summaries []skillsrc.Summary, maxBytes int64) string {
	var b strings.Builder
	b.WriteString("<available_skills>")
	for _, summary := range summaries {
		entry := "\n  <skill>\n    <name>" + xmlEscaper.Replace(summary.Name) +
			"</name>\n    <description>" + xmlEscaper.Replace(summary.Description) +
			"</description>\n  </skill>"
		if int64(b.Len()+len(entry)+len("\n  <truncated>true</truncated>\n</available_skills>")) > maxBytes {
			b.WriteString("\n  <truncated>true</truncated>")
			break
		}
		b.WriteString(entry)
	}
	b.WriteString("\n</available_skills>")
	return b.String()
}

func boundedText(value string, truncated bool, maxBytes int64) string {
	if !truncated && int64(len(value)) <= maxBytes {
		return value
	}
	end := min(len(value), int(maxBytes)-len(truncationMarker))
	for end > 0 && end < len(value) && !utf8.RuneStart(value[end]) {
		end--
	}
	return value[:end] + truncationMarker
}

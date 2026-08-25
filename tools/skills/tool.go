package skills

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	skillsrc "github.com/Tangerg/lynx/skills"
	toolcontract "github.com/Tangerg/lynx/tool"
)

type LoadSkillRequest struct {
	Name string `json:"name" jsonschema:"minLength=1" jsonschema_description:"Exact skill name returned by list_skills or already known from the prompt."`
}

type ReadSkillResourceRequest struct {
	Name string `json:"name" jsonschema:"minLength=1" jsonschema_description:"Exact name of the skill whose instructions referenced the resource."`
	Path string `json:"path" jsonschema:"minLength=1" jsonschema_description:"Resource path relative to the skill directory, such as references/REFERENCE.md or scripts/run.py."`
}

type toolSet struct {
	source skillsrc.ResourceSource
}

// NewTools builds the three progressive-disclosure tools over source. A skill
// source has no sensible default, so nil returns [ErrNilSource].
func NewTools(source skillsrc.ResourceSource) ([]toolcontract.Tool, error) {
	if isNilSource(source) {
		return nil, ErrNilSource
	}
	set := &toolSet{source: source}
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

func isNilSource(source skillsrc.ResourceSource) bool {
	value := reflect.ValueOf(source)
	if !value.IsValid() {
		return true
	}
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

func (t *toolSet) list(ctx context.Context, _ struct{}) (string, error) {
	summaries, err := t.source.List(ctx)
	if err != nil {
		return "", err
	}
	return renderSummaries(summaries), nil
}

func (t *toolSet) load(ctx context.Context, request LoadSkillRequest) (string, error) {
	skill, err := t.source.Load(ctx, request.Name)
	if err != nil {
		return "", err
	}
	return skill.Instructions, nil
}

func (t *toolSet) readResource(ctx context.Context, request ReadSkillResourceRequest) (string, error) {
	data, err := skillsrc.ReadResource(ctx, t.source, request.Name, request.Path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

var xmlEscaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")

func renderSummaries(summaries []skillsrc.Summary) string {
	var b strings.Builder
	b.WriteString("<available_skills>")
	for _, summary := range summaries {
		b.WriteString("\n  <skill>\n    <name>")
		b.WriteString(xmlEscaper.Replace(summary.Name))
		b.WriteString("</name>\n    <description>")
		b.WriteString(xmlEscaper.Replace(summary.Description))
		b.WriteString("</description>\n  </skill>")
	}
	b.WriteString("\n</available_skills>")
	return b.String()
}

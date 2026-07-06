package skills

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Tangerg/lynx/core/model/chat"
	pkgjson "github.com/Tangerg/lynx/pkg/json"
	skillsrc "github.com/Tangerg/lynx/skills"
)

const (
	opList         = "list"
	opLoad         = "load"
	opLoadResource = "load_resource"
)

// Request is the LLM-facing argument shape. A single tool multiplexes the
// three progressive-disclosure operations on Op; Name and Path are required
// only for the operations that name a skill or a file.
type Request struct {
	Op   string `json:"op" jsonschema:"required,enum=list,enum=load,enum=load_resource" jsonschema_description:"What to do: \"list\" every available skill (name + description) so you can pick one; \"load\" one skill's full instructions by name; \"load_resource\" a file bundled with a skill."`
	Name string `json:"name,omitempty" jsonschema_description:"Skill name. Required for load and load_resource; ignored for list."`
	Path string `json:"path,omitempty" jsonschema_description:"Resource path relative to the skill directory, e.g. references/REFERENCE.md or scripts/run.py. Required for load_resource."`
}

var toolSchema, _ = pkgjson.StringDefSchemaOf(Request{})

const toolDescription = "Discover and read Agent Skills — reusable, on-demand instruction packs. " +
	"Call with op=\"list\" to see which skills exist (name + description); when one is relevant, op=\"load\" name=<skill> to pull in its full instructions; " +
	"op=\"load_resource\" name=<skill> path=<file> to read a bundled reference, asset, or script the instructions point to. " +
	"Load a skill only when the task matches its description; then follow the returned instructions, running any scripts with your own shell/file tools."

var _ chat.Tool = (*Tool)(nil)

// Tool is the LLM-callable adapter over a [skillsrc.Source]. It is a thin
// wrapper: all parsing, validation, and IO live in the source.
type Tool struct {
	source skillsrc.Source
}

// NewTool builds a [Tool] over source. Unlike the local-by-default file tools,
// a source has no sensible default — passing nil returns [ErrNilSource].
func NewTool(source skillsrc.Source) (*Tool, error) {
	if source == nil {
		return nil, ErrNilSource
	}
	return &Tool{source: source}, nil
}

func (t *Tool) Definition() chat.ToolDefinition {
	return chat.ToolDefinition{
		Name:        "skill",
		Description: toolDescription,
		InputSchema: toolSchema,
	}
}

// Call dispatches on Op. Source errors already carry a "skills:" prefix and
// the failing skill/path, so they pass through unwrapped — only the
// tool-specific argument parse is wrapped here.
func (t *Tool) Call(ctx context.Context, arguments string) (string, error) {
	var req Request
	if err := json.Unmarshal([]byte(arguments), &req); err != nil {
		return "", fmt.Errorf("skills.tool: parse arguments: %w", err)
	}
	switch req.Op {
	case opList:
		summaries, err := t.source.List(ctx)
		if err != nil {
			return "", err
		}
		return renderSummaries(summaries), nil
	case opLoad:
		if req.Name == "" {
			return "", ErrNameRequired
		}
		sk, err := t.source.Load(ctx, req.Name)
		if err != nil {
			return "", err
		}
		return sk.Body, nil
	case opLoadResource:
		if req.Name == "" {
			return "", ErrNameRequired
		}
		if req.Path == "" {
			return "", ErrPathRequired
		}
		data, err := t.source.LoadResource(ctx, req.Name, req.Path)
		if err != nil {
			return "", err
		}
		return string(data), nil
	default:
		return "", fmt.Errorf("%w: %q", ErrUnknownOp, req.Op)
	}
}

// xmlEscaper escapes the characters that would break the <available_skills>
// envelope. Skill names are constrained to [a-z0-9-] by the spec, but
// descriptions are free text.
var xmlEscaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")

// renderSummaries formats the skill list as the <available_skills> envelope
// the Agent Skills ecosystem uses — compact, and legible to the model.
func renderSummaries(summaries []skillsrc.Summary) string {
	var b strings.Builder
	b.WriteString("<available_skills>")
	for _, s := range summaries {
		b.WriteString("\n  <skill>\n    <name>")
		b.WriteString(xmlEscaper.Replace(s.Name))
		b.WriteString("</name>\n    <description>")
		b.WriteString(xmlEscaper.Replace(s.Description))
		b.WriteString("</description>\n  </skill>")
	}
	b.WriteString("\n</available_skills>")
	return b.String()
}

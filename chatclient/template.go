package chatclient

import (
	"bytes"
	"errors"
	"fmt"
	"slices"
	"strings"
	"text/template"
	"text/template/parse"

	"github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/core/media"
)

// ErrInvalidTemplate reports an empty, malformed, or unusable prompt
// template.
var ErrInvalidTemplate = errors.New("chatclient: invalid template")

// Template is an immutable, parsed prompt template. It is safe to render from
// multiple goroutines after construction. Variables are ordinary per-render
// data rather than mutable state retained by Template.
type Template struct {
	source   string
	compiled *template.Template
}

// ParseTemplate parses source using Go text/template syntax. Missing map keys
// fail at Render time instead of silently becoming "<no value>".
func ParseTemplate(source string) (*Template, error) {
	if strings.TrimSpace(source) == "" {
		return nil, fmt.Errorf("%w: source is empty", ErrInvalidTemplate)
	}
	compiled, err := template.New("prompt").Option("missingkey=error").Parse(source)
	if err != nil {
		return nil, fmt.Errorf("%w: parse: %w", ErrInvalidTemplate, err)
	}
	return &Template{source: source, compiled: compiled}, nil
}

// Source returns the original template source. A nil Template returns an
// empty string.
func (t *Template) Source() string {
	if t == nil {
		return ""
	}
	return t.source
}

// Render executes the parsed template with data. Maps and structs are the
// common inputs, following text/template's ordinary field and key rules.
func (t *Template) Render(data any) (string, error) {
	if t == nil || t.compiled == nil {
		return "", fmt.Errorf("%w: nil template", ErrInvalidTemplate)
	}
	var rendered bytes.Buffer
	if err := t.compiled.Execute(&rendered, data); err != nil {
		return "", fmt.Errorf("%w: render: %w", ErrInvalidTemplate, err)
	}
	return rendered.String(), nil
}

// Require verifies that every named field selector is referenced by the parsed
// template. It understands whitespace, pipelines, branches, and nested paths;
// for .User.Name, the first selector is User.
func (t *Template) Require(names ...string) error {
	if t == nil || t.compiled == nil {
		return fmt.Errorf("%w: nil template", ErrInvalidTemplate)
	}
	used := make(map[string]struct{})
	for _, associated := range t.compiled.Templates() {
		if associated.Tree != nil && associated.Tree.Root != nil {
			collectTemplateFields(associated.Tree.Root, used)
		}
	}

	missing := make([]string, 0)
	for _, name := range names {
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("%w: required field name is empty", ErrInvalidTemplate)
		}
		if _, ok := used[name]; !ok {
			missing = append(missing, name)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	slices.Sort(missing)
	missing = slices.Compact(missing)
	return fmt.Errorf("%w: missing fields: %s", ErrInvalidTemplate, strings.Join(missing, ", "))
}

// SystemMessage renders data into a validated text-only system message.
func (t *Template) SystemMessage(data any) (chat.Message, error) {
	rendered, err := t.Render(data)
	if err != nil {
		return chat.Message{}, err
	}
	message := chat.NewSystemMessage(rendered)
	if err := message.Validate(); err != nil {
		return chat.Message{}, fmt.Errorf("%w: system message: %w", ErrInvalidTemplate, err)
	}
	return message, nil
}

// UserMessage renders data into a validated user message and appends media in
// the supplied order. An empty rendered string is allowed when media supplies
// the message content.
func (t *Template) UserMessage(data any, attachments ...*media.Media) (chat.Message, error) {
	rendered, err := t.Render(data)
	if err != nil {
		return chat.Message{}, err
	}
	parts := make([]chat.Part, 0, 1+len(attachments))
	if rendered != "" {
		parts = append(parts, chat.NewTextPart(rendered))
	}
	for _, attachment := range attachments {
		parts = append(parts, chat.NewMediaPart(attachment))
	}
	message := chat.NewUserMessage(parts...)
	if err := message.Validate(); err != nil {
		return chat.Message{}, fmt.Errorf("%w: user message: %w", ErrInvalidTemplate, err)
	}
	return message, nil
}

func collectTemplateFields(node parse.Node, fields map[string]struct{}) {
	if node == nil {
		return
	}
	switch value := node.(type) {
	case *parse.ListNode:
		for _, child := range value.Nodes {
			collectTemplateFields(child, fields)
		}
	case *parse.ActionNode:
		collectTemplateFields(value.Pipe, fields)
	case *parse.IfNode:
		collectBranchFields(value.Pipe, value.List, value.ElseList, fields)
	case *parse.RangeNode:
		collectBranchFields(value.Pipe, value.List, value.ElseList, fields)
	case *parse.WithNode:
		collectBranchFields(value.Pipe, value.List, value.ElseList, fields)
	case *parse.TemplateNode:
		if value.Pipe != nil {
			collectTemplateFields(value.Pipe, fields)
		}
	case *parse.PipeNode:
		for _, command := range value.Cmds {
			collectTemplateFields(command, fields)
		}
	case *parse.CommandNode:
		for _, argument := range value.Args {
			collectTemplateFields(argument, fields)
		}
	case *parse.FieldNode:
		if len(value.Ident) > 0 {
			fields[value.Ident[0]] = struct{}{}
		}
	case *parse.ChainNode:
		collectTemplateFields(value.Node, fields)
		if _, dot := value.Node.(*parse.DotNode); dot && len(value.Field) > 0 {
			fields[value.Field[0]] = struct{}{}
		}
	}
}

func collectBranchFields(pipe *parse.PipeNode, list, elseList *parse.ListNode, fields map[string]struct{}) {
	if pipe != nil {
		collectTemplateFields(pipe, fields)
	}
	if list != nil {
		collectTemplateFields(list, fields)
	}
	if elseList != nil {
		collectTemplateFields(elseList, fields)
	}
}

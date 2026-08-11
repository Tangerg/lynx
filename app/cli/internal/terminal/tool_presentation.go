package terminal

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
)

type toolSectionStyle uint8

const (
	toolSectionCode toolSectionStyle = iota
	toolSectionDiff
	toolSectionParagraph
)

// ToolSection is one semantic region of a tool presentation. It deliberately
// carries no Oolong component so inline blocks, the reader, and approvals can
// project the same meaning at different sizes without sharing widget state.
type ToolSection struct {
	Title       string
	Style       toolSectionStyle
	Language    string
	Text        string
	LineNumbers bool
	Links       bool
}

// ToolPresentation is the deterministic terminal meaning of one tool call.
type ToolPresentation struct {
	Label    string
	Sections []ToolSection
}

// ToolPresenter claims tool calls it understands and produces their semantic
// presentation. Matchers are ordered; the first match wins and the built-in
// generic presenter remains the final fallback.
type ToolPresenter struct {
	ID      string
	Matches func(agent.ToolCall) bool
	Present func(agent.ToolCall) ToolPresentation
}

func defaultToolPresenters() []ToolPresenter {
	return []ToolPresenter{
		kindToolPresenter("shell", agent.ToolShell, presentShellTool),
		kindToolPresenter("edit", agent.ToolEdit, presentEditTool),
		kindToolPresenter("read", agent.ToolRead, presentReadTool),
		kindToolPresenter("search", agent.ToolSearch, presentSearchTool),
		kindToolPresenter("web", agent.ToolWeb, presentWebTool),
		kindToolPresenter("task", agent.ToolTask, presentTaskTool),
		{ID: "generic", Matches: func(agent.ToolCall) bool { return true }, Present: presentUnknownTool},
	}
}

func kindToolPresenter(id string, kind agent.ToolKind, present func(agent.ToolCall) ToolPresentation) ToolPresenter {
	return ToolPresenter{
		ID: id,
		Matches: func(call agent.ToolCall) bool {
			return call.Kind == kind
		},
		Present: present,
	}
}

func presentShellTool(call agent.ToolCall) ToolPresentation {
	return ToolPresentation{
		Label: shellToolLabel(call),
		Sections: toolSections(call, ToolSection{
			Title: "Output", Style: toolSectionCode, Language: "text", Text: call.Output,
		}),
	}
}

func presentEditTool(call agent.ToolCall) ToolPresentation {
	sections := make([]ToolSection, 0, 2)
	if strings.TrimSpace(call.Diff) != "" {
		sections = append(sections, ToolSection{Title: "Changes", Style: toolSectionDiff, Language: "diff", Text: call.Diff})
	}
	if strings.TrimSpace(call.Output) != "" {
		sections = append(sections, ToolSection{Title: "Output", Style: toolSectionCode, Language: "text", Text: call.Output})
	}
	return ToolPresentation{Label: toolKindLabel("edit", toolPrimary(call.Path, call.Summary)), Sections: sections}
}

func presentReadTool(call agent.ToolCall) ToolPresentation {
	return ToolPresentation{
		Label: toolKindLabel("read", toolPrimary(call.Path, call.Summary)),
		Sections: toolSections(call, ToolSection{
			Title: "Content", Style: toolSectionCode, Language: languageForPath(call.Path), Text: call.Output, LineNumbers: true,
		}),
	}
}

func presentSearchTool(call agent.ToolCall) ToolPresentation {
	return ToolPresentation{
		Label: toolKindLabel("search", toolPrimary(call.Query, call.Summary)),
		Sections: toolSections(call, ToolSection{
			Title: "Matches", Style: toolSectionParagraph, Text: call.Output,
		}),
	}
}

func presentWebTool(call agent.ToolCall) ToolPresentation {
	return ToolPresentation{
		Label: toolKindLabel("web", toolPrimary(call.URL, call.Summary)),
		Sections: toolSections(call, ToolSection{
			Title: "Response", Style: toolSectionParagraph, Text: call.Output, Links: true,
		}),
	}
}

func presentTaskTool(call agent.ToolCall) ToolPresentation {
	return ToolPresentation{
		Label: toolKindLabel("task", strings.TrimSpace(call.Summary)),
		Sections: toolSections(call, ToolSection{
			Title: "Result", Style: toolSectionParagraph, Text: call.Output,
		}),
	}
}

func presentUnknownTool(call agent.ToolCall) ToolPresentation {
	return ToolPresentation{
		Label: unknownToolLabel(call),
		Sections: toolSections(call, ToolSection{
			Title: "Output", Style: toolSectionCode, Language: "text", Text: call.Output,
		}),
	}
}

func toolSections(call agent.ToolCall, output ToolSection) []ToolSection {
	sections := make([]ToolSection, 0, 2)
	if strings.TrimSpace(call.Diff) != "" {
		sections = append(sections, ToolSection{Title: "Changes", Style: toolSectionDiff, Language: "diff", Text: call.Diff})
	}
	if strings.TrimSpace(output.Text) != "" {
		sections = append(sections, output)
	}
	return sections
}

func selectToolPresentation(presenters []ToolPresenter, call agent.ToolCall) (ToolPresentation, error) {
	if len(presenters) == 0 {
		presenters = defaultToolPresenters()
	} else {
		presenters = slices.Clone(presenters)
	}
	for _, presenter := range presenters {
		if err := validateToolPresenter(presenter); err != nil {
			return ToolPresentation{}, err
		}
		matches, err := matchToolSafely(presenter, call)
		if err != nil {
			return ToolPresentation{}, err
		}
		if !matches {
			continue
		}
		presentation, err := presentToolSafely(presenter, call)
		if err != nil {
			return ToolPresentation{}, err
		}
		if strings.TrimSpace(presentation.Label) == "" {
			return ToolPresentation{}, fmt.Errorf("tool presenter %q returned an empty label", presenter.ID)
		}
		presentation.Sections = slices.Clone(presentation.Sections)
		return presentation, nil
	}
	return ToolPresentation{}, errors.New("no tool presenter matched the call")
}

func validateToolPresenter(presenter ToolPresenter) error {
	switch {
	case strings.TrimSpace(presenter.ID) == "":
		return errors.New("tool presenter has no id")
	case presenter.Matches == nil:
		return fmt.Errorf("tool presenter %q has no matcher", presenter.ID)
	case presenter.Present == nil:
		return fmt.Errorf("tool presenter %q has no projection", presenter.ID)
	default:
		return nil
	}
}

func matchToolSafely(presenter ToolPresenter, call agent.ToolCall) (matched bool, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("tool presenter %q matcher panicked: %v", presenter.ID, recovered)
		}
	}()
	return presenter.Matches(call), nil
}

func presentToolSafely(presenter ToolPresenter, call agent.ToolCall) (presentation ToolPresentation, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("tool presenter %q projection panicked: %v", presenter.ID, recovered)
		}
	}()
	return presenter.Present(call), nil
}

func toolLabel(call agent.ToolCall) string {
	presentation, err := selectToolPresentation(nil, call)
	if err != nil {
		return unknownToolLabel(call)
	}
	return presentation.Label
}

func shellToolLabel(call agent.ToolCall) string {
	primary := toolPrimary(call.Command, call.Summary)
	if primary == "" {
		return "shell"
	}
	return "$ " + primary
}

func unknownToolLabel(call agent.ToolCall) string {
	name := strings.TrimSpace(call.Name)
	if name == "" {
		name = "tool"
	}
	return toolKindLabel(name, strings.TrimSpace(call.Summary))
}

func toolPrimary(specific, fallback string) string {
	if specific = strings.TrimSpace(specific); specific != "" {
		return specific
	}
	return strings.TrimSpace(fallback)
}

func toolKindLabel(kind, primary string) string {
	if primary == "" {
		return kind
	}
	return kind + " · " + primary
}

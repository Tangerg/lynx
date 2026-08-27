package terminal

import (
	"bytes"
	"encoding/json"
	"slices"

	"github.com/Tangerg/scope/app/cli/internal/agent"
)

func (a *app) openApprovalArgumentEditor() {
	if a.approval == nil {
		return
	}
	a.approvalEditor = a.openContextEditor(contextEditorRequest{
		Title:       "Edit tool arguments",
		Description: "The replacement applies once and is validated as one non-empty JSON object.",
		Content:     a.approvalArguments,
		Placeholder: "{\n  \"argument\": \"replacement\"\n}",
		Save: func(value string, complete func(error) bool) error {
			override, err := agent.ParseToolArgumentOverride([]byte(value))
			if err != nil {
				complete(err)
				return nil
			}
			if complete(nil) {
				a.approvalEditor = nil
				a.approvalOverride = override
				a.approvalArguments = formatToolArguments(override.JSON())
				a.setApprovalPreview(a.approvalPreviewSections())
				a.setApprovalForm(approvalAllowOnce)
				a.approvalPane.Focus(true)
				a.approvalDialog.Controller().SetDescription("Arguments edited · choose how to proceed")
			}
			return nil
		},
		Dismissed: func() { a.approvalEditor = nil },
	})
}

func (a *app) approvalPreviewSections() []ToolSection {
	sections := slices.Clone(a.approvalSections)
	if a.approvalOverride == nil {
		return sections
	}
	return append(sections, ToolSection{
		Title: "Edited arguments · one-shot", Style: toolSectionCode,
		Language: "json", Text: a.approvalArguments,
	})
}

func (a *app) dismissApprovalEditor() {
	if a.approvalEditor == nil {
		return
	}
	a.approvalEditor.Dismiss()
	a.approvalEditor = nil
}

func editableApprovalArguments(call *agent.ToolCall) string {
	if call == nil || len(call.ArgumentsJSON) == 0 {
		return "{}"
	}
	return formatToolArguments(call.ArgumentsJSON)
}

func formatToolArguments(encoded []byte) string {
	var formatted bytes.Buffer
	if json.Indent(&formatted, encoded, "", "  ") == nil {
		return formatted.String()
	}
	return string(encoded)
}

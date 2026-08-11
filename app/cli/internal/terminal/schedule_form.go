package terminal

import (
	"errors"
	"strings"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/keymap"
	"github.com/Tangerg/oolong/core/layout"

	"github.com/Tangerg/lynx/app/cli/internal/schedule"
)

type scheduleFormMode uint8

const (
	scheduleFormCreate scheduleFormMode = iota + 1
	scheduleFormUpdate
)

type scheduleFormDraft struct {
	title        string
	instructions string
	workspace    string
	provider     string
	model        string
	cron         string
	enabled      string
}

func newScheduleFormDraft(mode scheduleFormMode, scheduled schedule.Schedule, defaultWorkspace string) scheduleFormDraft {
	if mode == scheduleFormUpdate {
		enabled := "disabled"
		if scheduled.Enabled {
			enabled = "enabled"
		}
		return scheduleFormDraft{
			title: scheduled.Title, instructions: scheduled.Instructions, workspace: scheduled.Workspace,
			provider: scheduled.Provider, model: scheduled.Model, cron: scheduled.Cron, enabled: enabled,
		}
	}
	return scheduleFormDraft{workspace: defaultWorkspace, cron: "0 9 * * 1-5", enabled: "enabled"}
}

func (draft scheduleFormDraft) candidate() (schedule.Candidate, error) {
	candidate := schedule.Candidate{
		Title: strings.TrimSpace(draft.title), Instructions: strings.TrimSpace(draft.instructions),
		Workspace: strings.TrimSpace(draft.workspace), Provider: strings.TrimSpace(draft.provider),
		Model: strings.TrimSpace(draft.model), Cron: strings.TrimSpace(draft.cron),
	}
	return candidate, candidate.Validate()
}

func (draft scheduleFormDraft) patch(original schedule.Schedule) (schedule.Patch, bool, error) {
	patch := schedule.Patch{ID: original.ID, ExpectedRevision: original.Revision}
	title := strings.TrimSpace(draft.title)
	if title != original.Title {
		patch.Title = &title
	}
	instructions := strings.TrimSpace(draft.instructions)
	if instructions != original.Instructions {
		patch.Instructions = &instructions
	}
	workspace := strings.TrimSpace(draft.workspace)
	if workspace != original.Workspace {
		patch.Workspace = &workspace
	}
	provider, model := strings.TrimSpace(draft.provider), strings.TrimSpace(draft.model)
	if provider != original.Provider || model != original.Model {
		patch.Provider, patch.Model = &provider, &model
	}
	cron := strings.TrimSpace(draft.cron)
	if cron != original.Cron {
		patch.Cron = &cron
	}
	enabled := draft.enabled == "enabled"
	if enabled != original.Enabled {
		patch.Enabled = &enabled
	}
	if !patch.HasChanges() {
		return patch, false, nil
	}
	if err := patch.Validate(); err != nil {
		return schedule.Patch{}, false, err
	}
	return patch, true, nil
}

func (a *app) openScheduleForm(mode scheduleFormMode, scheduled schedule.Schedule) {
	if a.scheduleDialog != nil {
		a.scheduleDialog.Dismiss()
		a.scheduleDialog = nil
	}
	draft := newScheduleFormDraft(mode, scheduled, a.session.Workspace)
	textField := func(label, placeholder string, value *string, check func(string) error) *headless.Text {
		field := &headless.Text{Label: label, Placeholder: placeholder, Value: headless.Bind(value), Check: check}
		field.Editor().Clipboard = a.loop.Clipboard()
		return field
	}
	fields := []headless.Field{
		textField("Title", "Optional name", &draft.title, nil),
		textField("Instructions", "Prompt sent when this schedule fires", &draft.instructions, requiredText),
		textField("Cron", "0 9 * * 1-5", &draft.cron, validateCronShape),
		textField("Workspace", "Optional absolute path", &draft.workspace, nil),
		textField("Provider", "Optional; set together with model", &draft.provider, nil),
		textField("Model", "Optional; set together with provider", &draft.model, func(string) error {
			return validateScheduleModelPair(draft.provider, draft.model)
		}),
	}
	if mode == scheduleFormUpdate {
		enabled := &headless.Select[string]{Label: "Lifecycle", Value: headless.Bind(&draft.enabled), Rows: 2}
		enabled.SetOptions([]headless.Option[string]{{Label: "Enabled", Value: "enabled"}, {Label: "Disabled", Value: "disabled"}})
		fields = append(fields, enabled)
	}
	form := headless.NewForm(fields...)
	form.Keys = headless.DefaultFormKeys()
	dismiss := func() {
		if a.scheduleDialog != nil {
			a.scheduleDialog.Dismiss()
			a.scheduleDialog = nil
		}
	}
	form.Done = func() {
		switch mode {
		case scheduleFormCreate:
			candidate, err := draft.candidate()
			if err != nil {
				a.message("schedule form: " + err.Error())
				return
			}
			dismiss()
			a.createSchedule(candidate)
		case scheduleFormUpdate:
			patch, changed, err := draft.patch(scheduled)
			if err != nil {
				a.message("schedule form: " + err.Error())
				return
			}
			dismiss()
			if !changed {
				a.message("schedule configuration unchanged")
				return
			}
			a.updateSchedule(patch, "updating schedule "+scheduled.ID)
		}
	}
	form.GaveUp = dismiss
	body := kit.NewForm(kit.FormConfig{
		Theme: a.transcript.theme, Glyphs: a.transcript.glyphs, Controller: form,
		Hints: []keymap.Action{headless.Submit, headless.Cancel},
	})
	title := "Create scheduled run"
	if mode == scheduleFormUpdate {
		title = "Edit scheduled run · " + scheduled.ID
	}
	a.scheduleDialog = kit.NewDialog(kit.DialogConfig{
		Stack: &a.stack, Theme: a.transcript.theme, Glyphs: a.transcript.glyphs,
		Title: title, Body: body,
		Where: layout.Placement{Width: 88, Height: formDialogHeight(body.Measure(84), len(fields), 24)},
	})
	a.scheduleDialog.Show()
}

func validateCronShape(value string) error {
	fields := strings.Fields(value)
	if len(fields) != 5 {
		return errors.New("cron must contain exactly five fields")
	}
	return nil
}

func validateScheduleModelPair(provider, model string) error {
	if (strings.TrimSpace(provider) == "") != (strings.TrimSpace(model) == "") {
		return errors.New("provider and model must both be set or both be empty")
	}
	return nil
}

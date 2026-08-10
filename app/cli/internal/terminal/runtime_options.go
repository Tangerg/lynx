package terminal

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/layout"

	"github.com/Tangerg/lynx/app/cli/internal/client"
)

func (a *app) buildRuntimePickers(theme kit.Theme, glyphs kit.Glyphs) {
	a.modelPicker = newPicker(theme, glyphs, "search models",
		func(model client.Model) string {
			if model.Default {
				return model.DisplayName + " · default"
			}
			return model.DisplayName
		},
		func(model client.Model) string { return model.ID },
		func(model client.Model) {
			a.modelDialog.Dismiss()
			a.options.Model = model.ID
			if !slices.Contains(model.Efforts, a.options.Effort) {
				a.options.Effort = preferredEffort(model.Efforts)
			}
			a.syncOptions("model · " + model.DisplayName)
		},
	)
	a.modelDialog = kit.NewDialog(&a.stack, theme, glyphs, "Models", a.modelPicker)
	a.modelDialog.Panel().Where = layout.Placement{Width: 76, Height: 14, Margin: 1}
	a.modelPicker.cancel = a.modelDialog.Dismiss

	a.permissionPicker = newPicker(theme, glyphs, "search permission modes",
		permissionTitle,
		permissionDetail,
		func(mode client.PermissionMode) {
			a.permissionDialog.Dismiss()
			a.options.Permission = mode
			a.syncOptions("permissions · " + string(mode))
		},
	)
	a.permissionPicker.SetItems([]client.PermissionMode{
		client.PermissionAsk, client.PermissionReadOnly, client.PermissionAutoEdit, client.PermissionFull,
	})
	a.permissionDialog = kit.NewDialog(&a.stack, theme, glyphs, "Permissions", a.permissionPicker)
	a.permissionDialog.Panel().Where = layout.Placement{Width: 88, Height: 10, Margin: 1}
	a.permissionPicker.cancel = a.permissionDialog.Dismiss
}

func (a *app) ChooseModel() {
	if a.state.Busy() || a.following {
		a.message("model changes apply between runs")
		return
	}
	a.message("loading models")
	runOperation(a, pickerCatalogOperation, true,
		func(ctx context.Context) ([]client.Model, error) { return a.backend.ListModels(ctx) },
		func(models []client.Model, err error) {
			if err != nil {
				a.message("could not load models: " + err.Error())
				return
			}
			if err := client.ValidateModels(models); err != nil {
				a.message(fmt.Sprintf("runtime models: %v", err))
				return
			}
			a.modelPicker.Reset()
			a.modelPicker.SetItems(models)
			a.modelDialog.Show()
			a.status.note("choose a model")
		},
	)
}

func (a *app) CycleMode() {
	if a.state.Busy() || a.following {
		a.message("mode changes apply between runs")
		return
	}
	modes := []client.AgentMode{client.ModeBuild, client.ModePlan, client.ModeReview}
	at := slices.Index(modes, a.options.Mode)
	a.options.Mode = modes[(at+1)%len(modes)]
	a.syncOptions("mode · " + string(a.options.Mode))
}

func (a *app) ChoosePermission() {
	if a.state.Busy() || a.following {
		a.message("permission changes apply between runs")
		return
	}
	a.permissionPicker.Reset()
	a.permissionDialog.Show()
	a.status.note("choose a permission mode")
}

func (a *app) SetEffort(value string) {
	if a.state.Busy() || a.following {
		a.message("effort changes apply between runs")
		return
	}
	value = strings.ToLower(strings.TrimSpace(value))
	if !slices.Contains([]string{"low", "medium", "high", "max", "ultra"}, value) {
		a.message("effort must be low, medium, high, max, or ultra")
		return
	}
	a.options.Effort = value
	a.syncOptions("effort · " + value)
}

func (a *app) ShowRuntimeStatus() {
	a.transcript.Append(kit.Message{
		Theme: a.transcript.theme, Speaker: "runtime options",
		Body: fmt.Sprintf("model: %s\neffort: %s\nmode: %s\npermissions: %s", modelLabel(a.options.Model), a.options.Effort, a.options.Mode, a.options.Permission),
	})
}

func (a *app) ShowApprovalRules() {
	runOperation(a, approvalCatalogOperation, true,
		func(ctx context.Context) ([]client.ApprovalRule, error) { return a.backend.ListApprovalRules(ctx) },
		func(rules []client.ApprovalRule, err error) {
			if err != nil {
				a.message("could not load approval rules: " + err.Error())
				return
			}
			if err := client.ValidateApprovalRules(rules); err != nil {
				a.message(fmt.Sprintf("runtime approval rules: %v", err))
				return
			}
			if len(rules) == 0 {
				a.message("no remembered approval rules")
				return
			}
			lines := make([]string, 0, len(rules))
			for _, rule := range rules {
				lines = append(lines, fmt.Sprintf("%s  %s  %s  %s", rule.ID, rule.Scope, rule.Decision, rule.Rule))
			}
			a.transcript.Append(kit.Message{Theme: a.transcript.theme, Speaker: "approval rules", Body: strings.Join(lines, "\n")})
		},
	)
}

func (a *app) syncOptions(message string) {
	a.status.setOptions(a.options)
	a.message(message)
}

func preferredEffort(efforts []string) string {
	if slices.Contains(efforts, "medium") {
		return "medium"
	}
	if len(efforts) > 0 {
		return efforts[0]
	}
	return "medium"
}

func permissionTitle(mode client.PermissionMode) string {
	switch mode {
	case client.PermissionAsk:
		return "Ask before consequential work"
	case client.PermissionReadOnly:
		return "Read only"
	case client.PermissionAutoEdit:
		return "Auto-edit workspace files"
	case client.PermissionFull:
		return "Full access"
	default:
		return string(mode)
	}
}

func permissionDetail(mode client.PermissionMode) string {
	switch mode {
	case client.PermissionAsk:
		return "review writes and risky commands"
	case client.PermissionReadOnly:
		return "never mutate the workspace"
	case client.PermissionAutoEdit:
		return "writes allowed; risky external actions ask"
	case client.PermissionFull:
		return "all runtime capabilities allowed"
	default:
		return ""
	}
}

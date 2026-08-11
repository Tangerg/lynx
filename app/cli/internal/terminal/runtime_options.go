package terminal

import (
	"context"
	"fmt"
	"strings"

	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/layout"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
)

func (a *app) buildRuntimePickers(theme kit.Theme, glyphs kit.Glyphs) {
	a.modelPicker = newPicker(theme, glyphs, "search models",
		func(model agent.Model) string {
			label := model.DisplayName
			if label == "" {
				label = model.ID
			}
			if model.Deprecated {
				label += " · deprecated"
			}
			return label
		},
		func(model agent.Model) string { return model.Provider + "/" + model.ID },
		func(model agent.Model) {
			a.modelDialog.Dismiss()
			a.options.Provider, a.options.Model = model.Provider, model.ID
			a.syncOptions("model · " + model.Provider + "/" + model.ID)
		},
	)
	a.modelDialog = kit.NewDialog(kit.DialogConfig{
		Stack: &a.stack, Theme: theme, Glyphs: glyphs, Title: "Models", Body: a.modelPicker,
		Where: layout.Placement{Width: 76, Height: 14},
	})
	a.modelPicker.cancel = a.modelDialog.Dismiss

	a.approvalModePicker = newPicker(theme, glyphs, "search approval modes",
		approvalModeTitle,
		approvalModeDetail,
		func(mode agent.ApprovalMode) {
			a.approvalModeDialog.Dismiss()
			a.setApprovalMode(mode)
		},
	)
	a.approvalModePicker.SetItems([]agent.ApprovalMode{agent.ApprovalModeSafe, agent.ApprovalModeBalanced, agent.ApprovalModeYolo})
	a.approvalModeDialog = kit.NewDialog(kit.DialogConfig{
		Stack: &a.stack, Theme: theme, Glyphs: glyphs, Title: "Runtime approval mode", Body: a.approvalModePicker,
		Where: layout.Placement{Width: 88, Height: 9},
	})
	a.approvalModePicker.cancel = a.approvalModeDialog.Dismiss
}

func (a *app) ChooseModel() {
	if a.conversation.Busy() || a.following {
		a.message("model changes apply between runs")
		return
	}
	a.message("loading models")
	runOperation(a, pickerCatalogOperation, true,
		func(ctx context.Context) ([]agent.Model, error) { return a.runtime.ListModels(ctx) },
		func(models []agent.Model, err error) {
			if err != nil {
				a.message("could not load models: " + err.Error())
				return
			}
			if err := agent.ValidateModels(models); err != nil {
				a.message(fmt.Sprintf("runtime models: %v", err))
				return
			}
			a.modelPicker.Reset()
			a.modelPicker.SetItems(models)
			a.modelDialog.Show()
			a.status.note("choose a provider-qualified model")
		},
	)
}

func (a *app) ChooseApprovalMode() {
	if a.conversation.Busy() || a.following {
		a.message("approval mode changes apply between runs")
		return
	}
	a.approvalModePicker.Reset()
	a.approvalModeDialog.Show()
	a.status.note("choose the runtime approval mode")
}

func (a *app) setApprovalMode(mode agent.ApprovalMode) {
	runOperation(a, pickerCatalogOperation, true,
		func(ctx context.Context) (agent.ApprovalMode, error) { return a.runtime.SetApprovalMode(ctx, mode) },
		func(applied agent.ApprovalMode, err error) {
			if err != nil {
				a.message("could not set approval mode: " + err.Error())
				return
			}
			a.message("approval mode · " + string(applied))
		},
	)
}

func (a *app) ShowRuntimeStatus() {
	runOperation(a, pickerCatalogOperation, true,
		func(ctx context.Context) (agent.ApprovalMode, error) { return a.runtime.GetApprovalMode(ctx) },
		func(mode agent.ApprovalMode, err error) {
			if err != nil {
				a.message("could not read runtime status: " + err.Error())
				return
			}
			a.transcript.Append(&kit.Message{
				Theme: a.transcript.theme, Speaker: "runtime options",
				Body: fmt.Sprintf("model: %s\napproval mode: %s%s", modelLabel(a.options), mode, limitsLabel(a.options.Limits)),
			})
		},
	)
}

func (a *app) ShowApprovalRules() {
	sessionID := a.session.ID
	runOperation(a, approvalCatalogOperation, true,
		func(ctx context.Context) ([]agent.ApprovalRule, error) {
			return a.runtime.ListApprovalRules(ctx, sessionID)
		},
		func(rules []agent.ApprovalRule, err error) {
			if err != nil {
				a.message("could not load approval rules: " + err.Error())
				return
			}
			if err := agent.ValidateApprovalRules(rules); err != nil {
				a.message(fmt.Sprintf("runtime approval rules: %v", err))
				return
			}
			if len(rules) == 0 {
				a.message("no remembered approval rules")
				return
			}
			lines := make([]string, 0, len(rules))
			for _, rule := range rules {
				subject := rule.Subject
				if subject == "" {
					subject = "*"
				}
				lines = append(lines, fmt.Sprintf("%s  %s  %s  %s:%s", rule.ID, rule.Scope, rule.Decision, rule.Tool, subject))
			}
			a.transcript.Append(&kit.Message{Theme: a.transcript.theme, Speaker: "approval rules", Body: strings.Join(lines, "\n")})
		},
	)
}

func (a *app) syncOptions(message string) {
	a.status.setOptions(a.options)
	a.prompt.SetOptions(a.options)
	a.message(message)
}

func approvalModeTitle(mode agent.ApprovalMode) string {
	switch mode {
	case agent.ApprovalModeSafe:
		return "Safe"
	case agent.ApprovalModeBalanced:
		return "Balanced"
	case agent.ApprovalModeYolo:
		return "Yolo"
	default:
		return string(mode)
	}
}

func approvalModeDetail(mode agent.ApprovalMode) string {
	switch mode {
	case agent.ApprovalModeSafe:
		return "ask before write, exec, and network tools"
	case agent.ApprovalModeBalanced:
		return "allow writes and network; ask before shell execution"
	case agent.ApprovalModeYolo:
		return "allow every tool without approval prompts"
	default:
		return ""
	}
}

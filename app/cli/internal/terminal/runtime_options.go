package terminal

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/layout"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
	"github.com/Tangerg/lynx/app/cli/internal/runtimeprofile"
	"github.com/Tangerg/lynx/app/cli/internal/session"
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
			a.selectSessionModel(model)
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

func (a *app) selectSessionModel(model agent.Model) {
	sessionID := a.session.ID
	runSessionChange(a, "selecting model",
		func(ctx context.Context) (agent.Session, error) {
			latest, err := a.runtime.GetSession(ctx, sessionID)
			if err != nil {
				return agent.Session{}, err
			}
			return session.Update(ctx, a.runtime, agent.UpdateSession{
				SessionID: sessionID, Model: &model.ID, ExpectedRevision: latest.Session.Revision,
			})
		},
		func(updated agent.Session) error {
			a.setActiveSession(updated)
			a.options.Provider, a.options.Model = model.Provider, model.ID
			a.syncOptions("model · " + model.Provider + "/" + model.ID)
			return nil
		},
	)
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
				Body: runtimeStatusText(a.runtimeProfile, a.options, mode),
			})
		},
	)
}

func runtimeStatusText(profile *runtimeprofile.Profile, options agent.RunOptions, mode agent.ApprovalMode) string {
	lines := []string{
		"model: " + modelLabel(options),
		"approval mode: " + string(mode) + limitsLabel(options.Limits),
	}
	if profile == nil {
		return strings.Join(lines, "\n")
	}
	features := profile.AvailableFeatureNames()
	if len(features) == 0 {
		features = []string{"none"}
	}
	limits := profile.Limits
	profileLines := []string{
		fmt.Sprintf("runtime: %s %s", profile.Server.Name, profile.Server.Version),
		fmt.Sprintf("protocol: %s (minimum %s)", profile.Protocol.Current, profile.Protocol.MinSupported),
		"default workspace: " + profile.Server.DefaultWorkspace,
		"home: " + profile.Server.Home,
		"available features: " + strings.Join(features, ", "),
		fmt.Sprintf("run concurrency: %s", optionalLimit(limits.MaxConcurrentRuns)),
		fmt.Sprintf("run replay: %d events / %s / %s", limits.RunReplay.MaxEvents, formatRuntimeBytes(limits.RunReplay.MaxBytes), limits.RunReplay.Scope),
		"command replay retention: " + formatRuntimeSeconds(limits.IdempotencyRetentionSeconds),
		"MCP authorization retention: " + formatRuntimeSeconds(limits.MCPAuthorizationRetentionSeconds),
		fmt.Sprintf("runtime subscriptions: %d topics / %d watches", limits.RuntimeSubscription.MaxTopics, limits.RuntimeSubscription.MaxWatches),
		fmt.Sprintf("surface: %d run events / %d topics / %d snapshots / %d streaming methods", len(profile.RunEvents), len(profile.RuntimeTopics), len(profile.StateSnapshots), len(profile.StreamingMethods)),
	}
	return strings.Join(slices.Concat(profileLines, lines), "\n")
}

func optionalLimit(value int) string {
	if value == 0 {
		return "runtime default"
	}
	return fmt.Sprintf("%d runs", value)
}

func formatRuntimeSeconds(value int) string {
	if value%3600 == 0 {
		return fmt.Sprintf("%dh", value/3600)
	}
	if value%60 == 0 {
		return fmt.Sprintf("%dm", value/60)
	}
	return fmt.Sprintf("%ds", value)
}

func formatRuntimeBytes(value int) string {
	const unit = 1024
	switch {
	case value >= unit*unit && value%(unit*unit) == 0:
		return fmt.Sprintf("%d MiB", value/(unit*unit))
	case value >= unit && value%unit == 0:
		return fmt.Sprintf("%d KiB", value/unit)
	default:
		return fmt.Sprintf("%d B", value)
	}
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
	a.brand.SetOptions(a.options)
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

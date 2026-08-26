package terminal

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/keymap"
	"github.com/Tangerg/oolong/core/layout"

	"github.com/Tangerg/lynx/app/cli/internal/agentmemory"
	"github.com/Tangerg/lynx/app/cli/internal/goal"
	"github.com/Tangerg/lynx/app/cli/internal/knowledge"
	"github.com/Tangerg/lynx/app/cli/internal/modelconfig"
	"github.com/Tangerg/lynx/app/cli/internal/usage"
)

type runtimeReaderMode uint8

const (
	runtimeReaderNone runtimeReaderMode = iota
	runtimeReaderGoal
	runtimeReaderDiscoveredSkills
	runtimeReaderManagedSkills
	runtimeReaderSkillProposals
	runtimeReaderMCPServers
	runtimeReaderMCPTools
	runtimeReaderMCPAuthorization
	runtimeReaderSchedules
	runtimeReaderModels
	runtimeReaderModelRoles
	runtimeReaderProviders
	runtimeReaderApprovalRules
	runtimeReaderAgentMemory
	runtimeReaderKnowledge
	runtimeReaderDiagnosticTools
	runtimeReaderAgentDocuments
	runtimeReaderRecipes
	runtimeReaderHooks
)

// runtimeReaderQuery describes one authoritative runtime projection read.
// Keeping the request as a value lets user-initiated reads and event-driven
// convergence share the same query without sharing their failure policy.
type runtimeReaderQuery struct {
	status    string
	mode      runtimeReaderMode
	selection runtimeReaderSelection
	read      func(context.Context) (readerDocument, error)
}

// runtimeReaderSelection carries typed identity needed to converge an open
// reader after an authoritative change event. It deliberately contains no UI
// or transport state.
type runtimeReaderSelection struct {
	knowledgeTarget   knowledge.Target
	knowledgeEntry    bool
	agentMemoryTarget agentmemory.Target
}

type usageReport struct {
	session usage.SessionReport
	summary usage.Summary
}

func (a *app) setRuntimeReader(mode runtimeReaderMode) {
	a.runtimeReader = mode
	a.runtimeSelection = runtimeReaderSelection{}
	if mode != runtimeReaderMCPTools {
		a.mcpToolServer = ""
	}
	if mode != runtimeReaderMCPAuthorization {
		a.mcpAuthorizationID = ""
	}
}

func (a *app) ShowUsage(argument string) error {
	if a.usage == nil {
		return errors.New("this runtime composition has no usage service")
	}
	sinceDays, err := parseSinceDays(argument)
	if err != nil {
		return err
	}
	sessionID := a.session.ID
	a.runRuntimeReaderQuery("loading runtime usage", runtimeReaderNone,
		func(ctx context.Context) (readerDocument, error) {
			session, err := a.usage.SessionUsage(ctx, sessionID)
			if err != nil {
				return readerDocument{}, err
			}
			summary, err := a.usage.Summary(ctx, sinceDays)
			if err != nil {
				return readerDocument{}, err
			}
			return usageDocument(usageReport{session: session, summary: summary}), nil
		})
	return nil
}

func parseSinceDays(argument string) (int, error) {
	argument = strings.TrimSpace(argument)
	if argument == "" || strings.EqualFold(argument, "all") {
		return 0, nil
	}
	days, err := strconv.Atoi(argument)
	if err != nil || days <= 0 {
		return 0, errors.New("usage: /usage [positive-days|all]")
	}
	return days, nil
}

func usageDocument(report usageReport) readerDocument {
	window := "all time"
	if report.summary.SinceDays > 0 {
		window = fmt.Sprintf("last %d days", report.summary.SinceDays)
	}
	sections := []ToolSection{
		{Title: "Current session", Style: toolSectionCode, Language: "text", Text: usageTotalsText(report.session.Total)},
		{Title: "Runtime total", Style: toolSectionCode, Language: "text", Text: usageTotalsText(report.summary.Total)},
	}
	sections = appendUsageBreakdown(sections, "By provider", report.summary.ByProvider)
	sections = appendUsageBreakdown(sections, "By model", report.summary.ByModel)
	sections = appendUsageBreakdown(sections, "By day", report.summary.ByDay)
	return readerDocument{
		Title: "Runtime usage", Detail: fmt.Sprintf("%s · %d sessions · %d runs", window, report.summary.Sessions, report.summary.Runs),
		Sections: sections,
	}
}

func appendUsageBreakdown(sections []ToolSection, title string, buckets []usage.Bucket) []ToolSection {
	if len(buckets) == 0 {
		return sections
	}
	lines := make([]string, 0, len(buckets))
	for _, bucket := range buckets {
		line := bucket.Key + "  " + usageTotalsText(bucket.Totals)
		if bucket.Runs > 0 {
			line += fmt.Sprintf("  · %d runs", bucket.Runs)
		}
		lines = append(lines, line)
	}
	return append(sections, ToolSection{Title: title, Style: toolSectionCode, Language: "text", Text: strings.Join(lines, "\n")})
}

func usageTotalsText(totals usage.Totals) string {
	parts := []string{
		"input " + formatThousands(totals.InputTokens),
		"output " + formatThousands(totals.OutputTokens),
	}
	if totals.CacheReadTokens > 0 {
		parts = append(parts, "cache read "+formatThousands(totals.CacheReadTokens))
	}
	if totals.CacheWriteTokens > 0 {
		parts = append(parts, "cache write "+formatThousands(totals.CacheWriteTokens))
	}
	if totals.ReasoningTokens > 0 {
		parts = append(parts, "reasoning "+formatThousands(totals.ReasoningTokens))
	}
	if totals.CostUSD != nil {
		parts = append(parts, "$"+strconv.FormatFloat(*totals.CostUSD, 'f', 4, 64))
	} else {
		parts = append(parts, "cost unavailable")
	}
	return strings.Join(parts, "  · ")
}

func (a *app) ShowModelRoles() {
	if a.modelConfig == nil {
		a.message("this runtime composition has no model configuration service")
		return
	}
	a.executeRuntimeReaderQuery(a.modelRolesReaderQuery())
}

func (a *app) modelRolesReaderQuery() runtimeReaderQuery {
	return runtimeReaderQuery{
		status: "loading model roles", mode: runtimeReaderModelRoles,
		read: func(ctx context.Context) (readerDocument, error) {
			roles, err := a.modelConfig.Roles(ctx)
			if err != nil {
				return readerDocument{}, err
			}
			return modelRolesDocument(roles), nil
		},
	}
}

func modelRolesDocument(roles modelconfig.Roles) readerDocument {
	return paragraphDocument("Auxiliary model roles", "runtime configuration", []string{
		"utility    " + roles.Utility.Label(),
		"embedding  " + roles.Embedding.Label(),
	})
}

func (a *app) SetModelRole(kind modelconfig.RoleKind, argument string) error {
	if a.modelConfig == nil {
		return errors.New("this runtime composition has no model configuration service")
	}
	role, err := parseModelRole(kind, argument)
	if err != nil {
		return err
	}
	a.status.note("updating " + string(kind) + " model role")
	started := a.runAdmissionMutation(modelConfigOperation, false,
		func(ctx context.Context) (modelconfig.Role, error) { return a.modelConfig.SetRole(ctx, role) },
		func(updated modelconfig.Role, err error) {
			if err != nil {
				a.message("update " + string(kind) + " role failed: " + err.Error())
				return
			}
			a.message(string(kind) + " model · " + updated.Label())
		},
	)
	if !started {
		return errors.New("another model configuration operation is running")
	}
	return nil
}

func parseModelRole(kind modelconfig.RoleKind, argument string) (modelconfig.Role, error) {
	argument = strings.TrimSpace(argument)
	role := modelconfig.Role{Kind: kind}
	if strings.EqualFold(argument, "off") || strings.EqualFold(argument, "inherit") {
		return role, role.Validate()
	}
	provider, model, found := strings.Cut(argument, "/")
	if !found {
		return modelconfig.Role{}, fmt.Errorf("usage: /%s <provider/model|off>", kind)
	}
	role.Provider, role.Model = strings.TrimSpace(provider), strings.TrimSpace(model)
	return role, role.Validate()
}

func (a *app) ShowProviders() {
	if a.modelConfig == nil {
		a.message("this runtime composition has no model configuration service")
		return
	}
	a.executeRuntimeReaderQuery(a.providersReaderQuery())
}

func (a *app) providersReaderQuery() runtimeReaderQuery {
	return runtimeReaderQuery{
		status: "loading providers", mode: runtimeReaderProviders,
		read: func(ctx context.Context) (readerDocument, error) {
			providers, err := a.modelConfig.Providers(ctx)
			if err != nil {
				return readerDocument{}, err
			}
			return providersDocument(providers), nil
		},
	}
}

func providersDocument(providers []modelconfig.Provider) readerDocument {
	lines := make([]string, 0, len(providers))
	for _, provider := range providers {
		status := "not configured"
		if provider.Configured() {
			status = provider.APIKeyMasked
			if provider.KeySource == modelconfig.KeyEnv {
				status += " · from env"
			}
		}
		capabilities := ""
		if provider.EmbeddingCapable {
			capabilities = " · embeddings"
		}
		endpoint := ""
		if provider.BaseURL != "" {
			endpoint = " · " + provider.BaseURL
		} else if provider.RequiresBaseURL {
			endpoint = " · endpoint required"
		}
		lines = append(lines, provider.ID+"  "+status+endpoint+capabilities)
	}
	return paragraphDocument("Providers", fmt.Sprintf("%d available", len(providers)), lines)
}

func (a *app) TestConfiguredProvider(providerID string) error {
	if a.modelConfig == nil {
		return errors.New("this runtime composition has no model configuration service")
	}
	providerID = strings.TrimSpace(providerID)
	if providerID == "" {
		return errors.New("usage: /provider-test <provider>")
	}
	a.status.note("testing provider " + providerID)
	started := a.runApplicationOperation(modelConfigOperation, false,
		func(ctx context.Context) (modelconfig.TestResult, error) {
			return a.modelConfig.TestProvider(ctx, providerID)
		},
		func(result modelconfig.TestResult, err error) {
			if err != nil {
				a.message("provider test failed: " + err.Error())
				return
			}
			if result.OK {
				a.message("provider " + providerID + " is reachable")
				return
			}
			a.message("provider " + providerID + " failed: " + result.Problem.String())
		},
	)
	if !started {
		return errors.New("another model configuration operation is running")
	}
	return nil
}

func (a *app) ConfigureProvider(providerID string) error {
	if a.modelConfig == nil {
		return errors.New("this runtime composition has no model configuration service")
	}
	providerID = strings.TrimSpace(providerID)
	if providerID == "" {
		return errors.New("usage: /provider-config <provider>")
	}
	presentation := a.sessionContext
	a.status.note("loading provider " + providerID)
	started := a.runApplicationOperation(modelConfigOperation, false,
		func(ctx context.Context) (modelconfig.Provider, error) {
			providers, err := a.modelConfig.Providers(ctx)
			if err != nil {
				return modelconfig.Provider{}, err
			}
			for _, provider := range providers {
				if strings.EqualFold(provider.ID, providerID) {
					return provider, nil
				}
			}
			return modelconfig.Provider{}, errors.New("provider not found: " + providerID)
		},
		func(provider modelconfig.Provider, err error) {
			if err != nil {
				a.message("configure provider failed: " + err.Error())
				return
			}
			if presentation != a.sessionContext {
				a.message("provider loaded after the active session changed; reopen configuration to continue")
				return
			}
			a.openProviderConfig(provider)
		},
	)
	if !started {
		return errors.New("another model configuration operation is running")
	}
	return nil
}

func (a *app) openProviderConfig(provider modelconfig.Provider) {
	baseMode := "keep"
	if provider.RequiresBaseURL && provider.BaseURL == "" {
		baseMode = "set"
	}
	baseURL := provider.BaseURL
	keyMode, apiKey := "keep", ""
	baseChoice := &headless.Select[string]{Label: "Endpoint change", Value: headless.Bind(&baseMode), Rows: 3}
	baseChoice.SetOptions([]headless.Option[string]{
		{Label: "Keep current endpoint", Value: "keep"},
		{Label: "Set endpoint", Value: "set"},
		{Label: "Clear endpoint", Value: "clear"},
	})
	baseField := &headless.Text{
		Label: "Endpoint URL", Placeholder: "https://api.example.com", Value: headless.Bind(&baseURL),
		Check: func(value string) error {
			if baseMode == "set" {
				return requiredText(value)
			}
			return nil
		},
	}
	keyChoice := &headless.Select[string]{Label: "API key change", Value: headless.Bind(&keyMode), Rows: 3}
	keyOptions := []headless.Option[string]{{Label: "Keep current key", Value: "keep"}}
	if provider.KeyEditable() {
		keyOptions = append(keyOptions,
			headless.Option[string]{Label: "Set a new key", Value: "set"},
			headless.Option[string]{Label: "Clear stored key", Value: "clear"},
		)
	}
	keyChoice.SetOptions(keyOptions)
	keyField := &headless.Text{
		Label: "New API key", Placeholder: provider.APIKeyMasked, Value: headless.Bind(&apiKey),
		Check: func(value string) error {
			if keyMode == "set" {
				return requiredText(value)
			}
			return nil
		},
	}
	keyField.Editor().SetMask("•")
	baseField.Editor().Clipboard = a.loop.Clipboard()
	keyField.Editor().Clipboard = a.loop.Clipboard()
	form := headless.NewForm(baseChoice, baseField, keyChoice, keyField)
	form.Keys = headless.DefaultFormKeys()
	var dialog *kit.Dialog
	clearKey := func() {
		apiKey = ""
		keyField.Editor().SetText("")
	}
	dismiss := func() {
		clearKey()
		if a.providerDialog == dialog {
			dialog.Dismiss()
			a.providerDialog = nil
		}
	}
	form.Done = func() {
		if a.providerDialog != dialog {
			clearKey()
			return
		}
		update := providerUpdate(provider.ID, baseMode, baseURL, keyMode, apiKey)
		dismiss()
		if update.BaseURL == nil && update.APIKey == nil {
			a.message("provider configuration unchanged")
			return
		}
		a.updateProvider(update)
	}
	form.GaveUp = dismiss
	dressed := kit.NewForm(kit.FormConfig{
		Theme: a.transcript.theme, Glyphs: a.transcript.glyphs, Controller: form,
		Hints: []keymap.Action{headless.Submit, headless.Cancel},
	})
	dialog = kit.NewDialog(kit.DialogConfig{
		Stack: &a.stack, Theme: a.transcript.theme, Glyphs: a.transcript.glyphs,
		Title: "Configure provider · " + provider.ID, Body: dressed,
		Where: layout.Placement{Width: 82, Height: 18},
	})
	a.providerDialog = dialog
	dialog.Show()
}

func providerUpdate(providerID, baseMode, baseURL, keyMode, apiKey string) modelconfig.UpdateProvider {
	update := modelconfig.UpdateProvider{Provider: providerID}
	update.BaseURL = valueChange(baseMode, strings.TrimSpace(baseURL))
	update.APIKey = valueChange(keyMode, strings.TrimSpace(apiKey))
	return update
}

func valueChange(mode, value string) *modelconfig.ValueChange {
	switch mode {
	case "set":
		return &modelconfig.ValueChange{Kind: modelconfig.SetValue, Value: value}
	case "clear":
		return &modelconfig.ValueChange{Kind: modelconfig.ClearValue}
	default:
		return nil
	}
}

func (a *app) updateProvider(update modelconfig.UpdateProvider) {
	a.status.note("updating provider " + update.Provider)
	started := a.runAdmissionMutation(modelConfigOperation, false,
		func(ctx context.Context) (modelconfig.Provider, error) {
			return a.modelConfig.UpdateProvider(ctx, update)
		},
		func(provider modelconfig.Provider, err error) {
			if err != nil {
				a.message("update provider failed: " + err.Error())
				return
			}
			a.message("provider updated · " + provider.ID)
		},
	)
	if !started {
		a.message("another model configuration operation is running")
	}
}

func (a *app) ShowGoal() {
	if a.goals == nil {
		a.message("this runtime composition has no goal service")
		return
	}
	a.executeRuntimeReaderQuery(a.goalReaderQuery())
}

func (a *app) goalReaderQuery() runtimeReaderQuery {
	sessionID := a.session.ID
	return runtimeReaderQuery{
		status: "loading session goal",
		mode:   runtimeReaderGoal,
		read: func(ctx context.Context) (readerDocument, error) {
			current, exists, err := a.goals.GetGoal(ctx, sessionID)
			if err != nil {
				return readerDocument{}, err
			}
			return goalDocument(current, exists), nil
		},
	}
}

func goalDocument(current goal.Goal, exists bool) readerDocument {
	if !exists {
		return paragraphDocument("Session goal", "none", []string{"No autonomous goal is active or paused for this session."})
	}
	lines := []string{
		"objective  " + current.Objective,
		"status     " + string(current.Status),
		fmt.Sprintf("used       %d runs · %d steps · $%.4f", current.Used.Runs, current.Used.Steps, current.Used.CostUSD),
	}
	model := "runtime default"
	if current.Provider != "" {
		model = current.Provider + "/" + current.Model
	}
	lines = append(lines, "model      "+model)
	budget := []string{}
	if current.Budget.MaxRuns > 0 {
		budget = append(budget, fmt.Sprintf("%d runs", current.Budget.MaxRuns))
	}
	if current.Budget.MaxSteps > 0 {
		budget = append(budget, fmt.Sprintf("%d steps", current.Budget.MaxSteps))
	}
	if current.Budget.MaxCostUSD > 0 {
		budget = append(budget, fmt.Sprintf("$%.4f", current.Budget.MaxCostUSD))
	}
	if len(budget) == 0 {
		budget = append(budget, "unbounded")
	}
	lines = append(lines, "budget     "+strings.Join(budget, " · "))
	if current.Reason != nil {
		reason := string(current.Reason.Code)
		if current.Reason.Detail != "" {
			reason += " · " + current.Reason.Detail
		}
		lines = append(lines, "reason     "+reason)
	}
	return paragraphDocument("Session goal", string(current.Status), lines)
}

func (a *app) StartGoal(objective string) error {
	if a.goals == nil {
		return errors.New("this runtime composition has no goal service")
	}
	start := goal.Start{
		SessionID: a.session.ID, Objective: strings.TrimSpace(objective),
		Provider: a.options.Provider, Model: a.options.Model,
	}
	if err := start.Validate(); err != nil {
		return err
	}
	return a.changeGoal("starting session goal", func(ctx context.Context) (goal.Goal, error) {
		return a.goals.StartGoal(ctx, start)
	})
}

func (a *app) UpdateGoal(objective string) error {
	if a.goals == nil {
		return errors.New("this runtime composition has no goal service")
	}
	update := goal.Update{SessionID: a.session.ID, Objective: strings.TrimSpace(objective)}
	if err := update.Validate(); err != nil {
		return err
	}
	return a.changeGoal("updating session goal", func(ctx context.Context) (goal.Goal, error) {
		return a.goals.UpdateGoal(ctx, update)
	})
}

func (a *app) ClearGoal() error {
	if a.goals == nil {
		return errors.New("this runtime composition has no goal service")
	}
	presentation := a.sessionContext
	sessionID := a.session.ID
	label := "clearing session goal"
	a.status.note(label)
	started := a.runAdmissionMutation(goalOperation, false,
		func(ctx context.Context) (struct{}, error) {
			return struct{}{}, a.goals.ClearGoal(ctx, sessionID)
		},
		func(_ struct{}, err error) {
			if err != nil {
				a.message(label + " failed: " + err.Error())
				return
			}
			if a.sessionContext == presentation {
				a.setRuntimeReader(runtimeReaderGoal)
				a.workspaceReader = workspaceReaderNone
				a.openReaderDocument(goalDocument(goal.Goal{}, false))
			}
			a.status.note("goal · cleared")
		},
	)
	if !started {
		return errors.New("another goal operation is running")
	}
	return nil
}

func (a *app) StopGoal() error {
	if a.goals == nil {
		return errors.New("this runtime composition has no goal service")
	}
	sessionID := a.session.ID
	return a.changeGoal("stopping session goal", func(ctx context.Context) (goal.Goal, error) {
		return a.goals.StopGoal(ctx, sessionID)
	})
}

func (a *app) ResumeGoal() error {
	if a.goals == nil {
		return errors.New("this runtime composition has no goal service")
	}
	sessionID := a.session.ID
	return a.changeGoal("resuming session goal", func(ctx context.Context) (goal.Goal, error) {
		return a.goals.ResumeGoal(ctx, sessionID)
	})
}

func (a *app) changeGoal(label string, change func(context.Context) (goal.Goal, error)) error {
	presentation := a.sessionContext
	a.status.note(label)
	sessionID := a.session.ID
	work := func(ctx context.Context) (goal.Goal, error) {
		current, exists, err := a.goals.GetGoal(ctx, sessionID)
		if err != nil {
			return goal.Goal{}, err
		}
		if exists && !current.Status.AllowsLifecycleCommands() {
			return goal.Goal{}, errors.New("goal is completing final accounting; wait for the next runtime change")
		}
		return change(ctx)
	}
	started := a.runAdmissionMutation(goalOperation, false, work, func(current goal.Goal, err error) {
		if err != nil {
			a.message(label + " failed: " + err.Error())
			return
		}
		if a.sessionContext == presentation {
			a.setRuntimeReader(runtimeReaderGoal)
			a.workspaceReader = workspaceReaderNone
			a.openReaderDocument(goalDocument(current, true))
		}
		a.status.note("goal · " + string(current.Status))
	})
	if !started {
		return errors.New("another goal operation is running")
	}
	return nil
}

func (a *app) runRuntimeReaderQuery(
	status string,
	mode runtimeReaderMode,
	read func(context.Context) (readerDocument, error),
) {
	a.executeRuntimeReaderQuery(runtimeReaderQuery{status: status, mode: mode, read: read})
}

func (a *app) executeRuntimeReaderQuery(query runtimeReaderQuery) {
	a.status.note(query.status)
	a.runOperation(readerDocumentOperation, true, query.read, func(document readerDocument, err error) {
		if err != nil {
			a.message(query.status + " failed: " + err.Error())
			return
		}
		a.setRuntimeReader(query.mode)
		a.runtimeSelection = query.selection
		a.workspaceReader = workspaceReaderNone
		a.openReaderDocument(document)
		a.status.note(strings.ToLower(document.Title))
	})
}

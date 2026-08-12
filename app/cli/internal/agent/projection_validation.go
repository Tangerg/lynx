package agent

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
)

func (r Run) Validate() error {
	var problems []error
	if strings.TrimSpace(r.ID) == "" {
		problems = append(problems, errors.New("id is empty"))
	}
	if strings.TrimSpace(r.SessionID) == "" {
		problems = append(problems, errors.New("session id is empty"))
	}
	if err := r.Lineage.validate(r.ID); err != nil {
		problems = append(problems, err)
	}
	if !slices.Contains([]RunStatus{RunStatusRunning, RunStatusWaiting, RunStatusFinished}, r.Status) {
		problems = append(problems, fmt.Errorf("status %q is invalid", r.Status))
	}
	if (r.Provider == "") != (r.Model == "") {
		problems = append(problems, errors.New("provider and model must be selected together"))
	}
	if r.Status == RunStatusRunning && strings.TrimSpace(r.ActiveSegmentID) == "" {
		problems = append(problems, errors.New("running run has no active segment"))
	}
	if r.Status != RunStatusRunning && r.ActiveSegmentID != "" {
		problems = append(problems, errors.New("non-running run carries an active segment"))
	}
	if r.Status != RunStatusFinished && !r.FinishedAt.IsZero() {
		problems = append(problems, errors.New("unfinished run carries a finish time"))
	}
	if !r.FinishedAt.IsZero() && r.CreatedAt.IsZero() {
		problems = append(problems, errors.New("finished run has no creation time"))
	}
	if !r.FinishedAt.IsZero() && r.FinishedAt.Before(r.CreatedAt) {
		problems = append(problems, errors.New("run finish time precedes creation time"))
	}
	if err := r.Limits.Validate(); err != nil {
		problems = append(problems, err)
	}
	if r.Status == RunStatusFinished {
		if err := r.Outcome.Validate(); err != nil {
			problems = append(problems, err)
		}
	} else if r.Outcome.Status != "" {
		problems = append(problems, errors.New("unfinished run carries an outcome"))
	}
	if err := r.Usage.Validate(); err != nil {
		problems = append(problems, err)
	}
	if err := errors.Join(problems...); err != nil {
		return fmt.Errorf("run: %w", err)
	}
	return nil
}

func (lineage RunLineage) validate(runID string) error {
	values := []string{lineage.SpawnedByBlockID, lineage.ParentRunID, lineage.RootRunID}
	present := 0
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			present++
		}
	}
	switch {
	case present == 0:
		return nil
	case present != len(values):
		return errors.New("run lineage must provide spawn block, parent, and root together")
	case lineage.ParentRunID == runID:
		return errors.New("run lineage names itself as parent")
	case lineage.RootRunID == runID:
		return errors.New("run lineage names itself as root")
	default:
		return nil
	}
}

func (o RunOptions) Validate() error {
	var problems []error
	if (strings.TrimSpace(o.Provider) == "") != (strings.TrimSpace(o.Model) == "") {
		problems = append(problems, errors.New("provider and model must be selected together"))
	}
	if err := o.Limits.Validate(); err != nil {
		problems = append(problems, err)
	}
	if err := o.Generation.Validate(); err != nil {
		problems = append(problems, err)
	}
	if err := errors.Join(problems...); err != nil {
		return fmt.Errorf("run options: %w", err)
	}
	return nil
}

func (l RunLimits) Validate() error {
	if l.MaxTotalTokens < 0 || l.MaxSteps < 0 || l.MaxBudgetUSD < 0 || math.IsNaN(l.MaxBudgetUSD) || math.IsInf(l.MaxBudgetUSD, 0) {
		return errors.New("run limits must be finite and non-negative")
	}
	return nil
}

func (p GenerationParams) Validate() error {
	if p.Temperature != nil && (math.IsNaN(*p.Temperature) || math.IsInf(*p.Temperature, 0) || *p.Temperature < 0 || *p.Temperature > 2) {
		return errors.New("generation temperature must be between 0 and 2")
	}
	if p.TopP != nil && (math.IsNaN(*p.TopP) || math.IsInf(*p.TopP, 0) || *p.TopP < 0 || *p.TopP > 1) {
		return errors.New("generation top-p must be between 0 and 1")
	}
	if p.MaxTokens != nil && *p.MaxTokens <= 0 {
		return errors.New("generation max tokens must be positive")
	}
	for i, stop := range p.Stop {
		if stop == "" {
			return fmt.Errorf("generation stop sequence %d is empty", i+1)
		}
	}
	return nil
}

func (o Outcome) Validate() error {
	if len(o.ProblemJSON) > 0 {
		var problem map[string]any
		if !json.Valid(o.ProblemJSON) || json.Unmarshal(o.ProblemJSON, &problem) != nil || problem == nil {
			return errors.New("outcome problem JSON is not an object")
		}
	}
	switch o.Status {
	case OutcomeCompleted:
		if strings.TrimSpace(o.Error) != "" || strings.TrimSpace(o.Detail) != "" || len(o.ProblemJSON) != 0 {
			return errors.New("completed outcome cannot carry an error, problem, or detail")
		}
	case OutcomeTimedOut, OutcomeFailed, OutcomeLost:
		if strings.TrimSpace(o.Error) == "" {
			return fmt.Errorf("%s outcome has no error", o.Status)
		}
		if strings.TrimSpace(o.Detail) != "" {
			return fmt.Errorf("%s outcome carries a policy detail", o.Status)
		}
	case OutcomeMaxSteps, OutcomeMaxBudget, OutcomeCanceled:
		if strings.TrimSpace(o.Error) != "" {
			return fmt.Errorf("%s outcome carries an error", o.Status)
		}
		if len(o.ProblemJSON) != 0 {
			return fmt.Errorf("%s outcome carries a problem", o.Status)
		}
	default:
		return fmt.Errorf("outcome status %q is invalid", o.Status)
	}
	return nil
}

func (u Usage) Validate() error {
	if err := validateModelUsage(ModelUsage{
		InputTokens: u.InputTokens, OutputTokens: u.OutputTokens,
		CacheReadTokens: u.CacheReadTokens, CacheWriteTokens: u.CacheWriteTokens,
		ReasoningTokens: u.ReasoningTokens, CostUSD: u.CostUSD,
	}); err != nil {
		return fmt.Errorf("total usage: %w", err)
	}
	if u.Steps < 0 {
		return errors.New("usage steps cannot be negative")
	}
	if u.Duration < 0 {
		return errors.New("usage duration cannot be negative")
	}
	for model, usage := range u.ByModel {
		if strings.TrimSpace(model) == "" {
			return errors.New("usage has an empty model key")
		}
		if err := validateModelUsage(usage); err != nil {
			return fmt.Errorf("model usage %q: %w", model, err)
		}
	}
	return nil
}

func validateModelUsage(usage ModelUsage) error {
	if usage.InputTokens < 0 || usage.OutputTokens < 0 || usage.CacheReadTokens < 0 ||
		usage.CacheWriteTokens < 0 || usage.ReasoningTokens < 0 {
		return errors.New("token counts cannot be negative")
	}
	if usage.CostUSD != nil && (*usage.CostUSD < 0 || math.IsNaN(*usage.CostUSD) || math.IsInf(*usage.CostUSD, 0)) {
		return errors.New("cost must be finite and non-negative when known")
	}
	return nil
}

func ValidateEvent(event Event) error {
	switch item := event.(type) {
	case SegmentStarted:
		if err := item.Run.Validate(); err != nil {
			return fmt.Errorf("segment started: %w", err)
		}
		if item.Run.Status != RunStatusRunning {
			return errors.New("segment started with a non-running run")
		}
		return nil
	case BlockStarted:
		return item.Block.validateLifecycle(false)
	case BlockDelta:
		if strings.TrimSpace(item.BlockID) == "" {
			return errors.New("block delta without a block id")
		}
		if item.Text == "" {
			return errors.New("block delta without text")
		}
		if item.ContentIndex != nil && *item.ContentIndex < 0 {
			return errors.New("block delta with a negative content index")
		}
		return nil
	case ToolArgumentsDelta:
		if strings.TrimSpace(item.BlockID) == "" {
			return errors.New("tool arguments delta without a block id")
		}
		return nil
	case RunProgress:
		if item.Step != nil && *item.Step < 0 {
			return errors.New("run progress step cannot be negative")
		}
		if item.ContextTokens != nil && *item.ContextTokens < 0 {
			return errors.New("run progress context tokens cannot be negative")
		}
		if item.Usage != nil {
			return item.Usage.Validate()
		}
		return nil
	case CustomEvent:
		if strings.TrimSpace(item.Name) == "" {
			return errors.New("custom event without a name")
		}
		if !json.Valid(item.PayloadJSON) {
			return errors.New("custom event payload is not valid JSON")
		}
		return nil
	case BlockCompleted:
		return item.Block.validateLifecycle(true)
	case PlanChanged:
		if item.Revision == 0 {
			return errors.New("plan changed without a revision")
		}
		return validatePlan(item.Items)
	case RunInterrupted:
		return errors.Join(ValidateInteractions(item.Interactions), item.Usage.Validate())
	case RunSuspended:
		return item.Usage.Validate()
	case RunFinished:
		if err := item.Outcome.Validate(); err != nil {
			return err
		}
		return item.Usage.Validate()
	case nil:
		return errors.New("event is nil")
	default:
		return fmt.Errorf("event %T is unsupported", event)
	}
}

func (block Block) validateLifecycle(completed bool) error {
	if err := block.validateEnvelope(completed); err != nil {
		return err
	}
	if err := block.validateAttachments(); err != nil {
		return err
	}
	return block.validateProjection()
}

func (block Block) validateEnvelope(completed bool) error {
	if strings.TrimSpace(block.ID) == "" {
		return errors.New("transcript block has no id")
	}
	if strings.TrimSpace(block.RunID) == "" {
		return fmt.Errorf("transcript block %s has no run id", block.ID)
	}
	if !slices.Contains([]BlockStatus{BlockStatusRunning, BlockStatusCompleted, BlockStatusIncomplete}, block.Status) {
		return fmt.Errorf("block %s has invalid status %q", block.ID, block.Status)
	}
	if completed == (block.Status == BlockStatusRunning) {
		return fmt.Errorf("block %s status %q disagrees with its event lifecycle", block.ID, block.Status)
	}
	if !slices.Contains([]BlockKind{BlockUser, BlockAssistant, BlockReasoning, BlockQuestion, BlockTool, BlockNotice, BlockError}, block.Kind) {
		return fmt.Errorf("block %s has invalid kind %q", block.ID, block.Kind)
	}
	if block.Status == BlockStatusRunning && !slices.Contains([]BlockKind{BlockAssistant, BlockReasoning, BlockTool}, block.Kind) {
		return fmt.Errorf("%s block %s cannot be running", block.Kind, block.ID)
	}
	wholeOnly := slices.Contains([]BlockKind{BlockUser, BlockQuestion, BlockNotice}, block.Kind)
	if wholeOnly && block.Status != BlockStatusCompleted {
		return fmt.Errorf("%s block %s must be completed", block.Kind, block.ID)
	}
	if block.Kind != BlockUser && len(block.Attachments) != 0 {
		return fmt.Errorf("%s block %s carries attachments", block.Kind, block.ID)
	}
	if block.Kind != BlockAssistant && len(block.Images) != 0 {
		return fmt.Errorf("%s block %s carries inline images", block.Kind, block.ID)
	}
	if block.Kind == BlockTool && !block.CreatedAt.IsZero() {
		return fmt.Errorf("tool block %s carries a message creation time", block.ID)
	}
	if block.Kind != BlockReasoning && block.Redacted {
		return fmt.Errorf("%s block %s is marked as redacted reasoning", block.Kind, block.ID)
	}
	if block.DroppedMessages < 0 {
		return fmt.Errorf("block %s has a negative dropped-message count", block.ID)
	}
	if block.Kind != BlockNotice && block.DroppedMessages != 0 {
		return fmt.Errorf("%s block %s carries a dropped-message count", block.Kind, block.ID)
	}
	return nil
}

func (block Block) validateAttachments() error {
	for i, attachment := range block.Attachments {
		if err := attachment.Validate(); err != nil {
			return fmt.Errorf("block %s attachment %d: %w", block.ID, i+1, err)
		}
	}
	for i, image := range block.Images {
		if err := image.Validate(); err != nil {
			return fmt.Errorf("block %s inline image %d: %w", block.ID, i+1, err)
		}
	}
	return nil
}

func (image InlineImage) Validate() error {
	var problems []error
	if strings.TrimSpace(image.ID) == "" {
		problems = append(problems, errors.New("id is empty"))
	}
	if strings.TrimSpace(image.Name) == "" {
		problems = append(problems, errors.New("name is empty"))
	}
	if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(image.MIMEType)), "image/") {
		problems = append(problems, errors.New("MIME type is not an image"))
	}
	if len(image.Data) == 0 {
		problems = append(problems, errors.New("data is empty"))
	}
	if err := errors.Join(problems...); err != nil {
		return fmt.Errorf("inline image: %w", err)
	}
	return nil
}

func (block Block) validateProjection() error {
	switch block.Kind {
	case BlockQuestion:
		return block.validateQuestionProjection()
	case BlockTool:
		return block.validateToolProjection()
	default:
		if block.Question != nil {
			return fmt.Errorf("%s block %s carries a question projection", block.Kind, block.ID)
		}
		if block.Tool != nil {
			return fmt.Errorf("%s block %s carries a tool projection", block.Kind, block.ID)
		}
	}
	return nil
}

func (block Block) validateQuestionProjection() error {
	if block.Question == nil {
		return fmt.Errorf("question block %s has no question projection", block.ID)
	}
	if block.Question.ItemID != block.ID {
		return fmt.Errorf("question block %s carries item id %s", block.ID, block.Question.ItemID)
	}
	if err := block.Question.Validate(); err != nil {
		return fmt.Errorf("block %s: %w", block.ID, err)
	}
	return nil
}

func (block Block) validateToolProjection() error {
	if block.Tool == nil {
		return fmt.Errorf("tool block %s has no tool projection", block.ID)
	}
	if err := block.Tool.Validate(); err != nil {
		return fmt.Errorf("block %s: %w", block.ID, err)
	}
	want := block.Tool.Status.blockStatus()
	if block.Status != want {
		return fmt.Errorf("block %s is %s while its tool is %s", block.ID, block.Status, block.Tool.Status)
	}
	return nil
}

func (status ToolStatus) blockStatus() BlockStatus {
	switch status {
	case ToolRunning:
		return BlockStatusRunning
	case ToolOK:
		return BlockStatusCompleted
	default:
		return BlockStatusIncomplete
	}
}

func validatePlan(items []PlanItem) error {
	active := 0
	for i, item := range items {
		if strings.TrimSpace(item.Title) == "" {
			return fmt.Errorf("plan item %d has no title", i+1)
		}
		if !slices.Contains([]PlanStatus{PlanPending, PlanActive, PlanDone}, item.Status) {
			return fmt.Errorf("plan item %d has invalid status %q", i+1, item.Status)
		}
		if item.Status == PlanActive {
			active++
		}
	}
	if active > 1 {
		return errors.New("plan has more than one active item")
	}
	return nil
}

func validateUsageProgress(previous, next Usage) error {
	if err := validateModelUsageProgress("total", ModelUsage{
		InputTokens: previous.InputTokens, OutputTokens: previous.OutputTokens,
		CacheReadTokens: previous.CacheReadTokens, CacheWriteTokens: previous.CacheWriteTokens,
		ReasoningTokens: previous.ReasoningTokens, CostUSD: previous.CostUSD,
	}, ModelUsage{
		InputTokens: next.InputTokens, OutputTokens: next.OutputTokens,
		CacheReadTokens: next.CacheReadTokens, CacheWriteTokens: next.CacheWriteTokens,
		ReasoningTokens: next.ReasoningTokens, CostUSD: next.CostUSD,
	}); err != nil {
		return err
	}
	switch {
	case next.Steps < previous.Steps:
		return errors.New("step usage regressed")
	case next.Duration < previous.Duration:
		return errors.New("active duration regressed")
	}
	for model, previousUsage := range previous.ByModel {
		nextUsage, exists := next.ByModel[model]
		if !exists {
			return fmt.Errorf("model usage %q disappeared", model)
		}
		if err := validateModelUsageProgress("model "+model, previousUsage, nextUsage); err != nil {
			return err
		}
	}
	return nil
}

func validateModelUsageProgress(label string, previous, next ModelUsage) error {
	switch {
	case next.InputTokens < previous.InputTokens:
		return fmt.Errorf("%s input-token usage regressed", label)
	case next.OutputTokens < previous.OutputTokens:
		return fmt.Errorf("%s output-token usage regressed", label)
	case next.CacheReadTokens < previous.CacheReadTokens:
		return fmt.Errorf("%s cache-read usage regressed", label)
	case next.CacheWriteTokens < previous.CacheWriteTokens:
		return fmt.Errorf("%s cache-write usage regressed", label)
	case next.ReasoningTokens < previous.ReasoningTokens:
		return fmt.Errorf("%s reasoning-token usage regressed", label)
	case previous.CostUSD != nil && next.CostUSD == nil:
		return fmt.Errorf("%s cost became unknown", label)
	case previous.CostUSD != nil && next.CostUSD != nil && *next.CostUSD < *previous.CostUSD:
		return fmt.Errorf("%s cost usage regressed", label)
	default:
		return nil
	}
}

func validateInteractionItem(interaction Interaction, block Block) error {
	itemID := InteractionItemID(interaction)
	if block.ID != itemID {
		return fmt.Errorf("interaction item %s resolved to block %s", itemID, block.ID)
	}
	if runID := InteractionRunID(interaction); block.RunID != runID {
		return fmt.Errorf("interaction item %s belongs to run %s, not %s", itemID, runID, block.RunID)
	}
	switch item := interaction.(type) {
	case Approval:
		if block.Kind != BlockTool || block.Status != BlockStatusRunning || block.Tool == nil {
			return fmt.Errorf("approval item %s is not a running tool", itemID)
		}
		if !block.Tool.Equal(*item.Tool) {
			return fmt.Errorf("approval item %s differs from its tool block", itemID)
		}
	case Question:
		if block.Kind != BlockQuestion || block.Status != BlockStatusCompleted || block.Question == nil {
			return fmt.Errorf("question item %s is not a completed question", itemID)
		}
		if !block.Question.Equal(item) {
			return fmt.Errorf("question item %s differs from its question block", itemID)
		}
	default:
		return fmt.Errorf("interaction item %s has unsupported type %T", itemID, interaction)
	}
	return nil
}

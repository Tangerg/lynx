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
	if r.Contract != nil {
		if err := r.Contract.validate(); err != nil {
			problems = append(problems, err)
		}
	}
	if err := errors.Join(problems...); err != nil {
		return fmt.Errorf("run: %w", err)
	}
	return nil
}

func (r RunContract) validate() error {
	if err := validateRunContractSet("required feature", r.RequiredFeatures, []RunFeature{RunFeatureSubagents}); err != nil {
		return err
	}
	return validateRunContractSet("interaction kind", r.InteractionKinds, []InteractionKind{
		InteractionApproval, InteractionQuestion,
	})
}

func validateRunContractSet[T comparable](label string, values, supported []T) error {
	seen := make(map[T]struct{}, len(values))
	for _, value := range values {
		if !slices.Contains(supported, value) {
			return fmt.Errorf("run contract: %s %v is unsupported", label, value)
		}
		if _, duplicate := seen[value]; duplicate {
			return fmt.Errorf("run contract: %s %v is duplicated", label, value)
		}
		seen[value] = struct{}{}
	}
	return nil
}

func (r RunLineage) validate(runID string) error {
	values := []string{r.SpawnedByBlockID, r.ParentRunID, r.RootRunID}
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
	case r.ParentRunID == runID:
		return errors.New("run lineage names itself as parent")
	case r.RootRunID == runID:
		return errors.New("run lineage names itself as root")
	default:
		return nil
	}
}

func (r RunOptions) Validate() error {
	var problems []error
	if (strings.TrimSpace(r.Provider) == "") != (strings.TrimSpace(r.Model) == "") {
		problems = append(problems, errors.New("provider and model must be selected together"))
	}
	if err := r.Limits.Validate(); err != nil {
		problems = append(problems, err)
	}
	if err := r.Generation.Validate(); err != nil {
		problems = append(problems, err)
	}
	if err := errors.Join(problems...); err != nil {
		return fmt.Errorf("run options: %w", err)
	}
	return nil
}

func (r RunLimits) Validate() error {
	if r.MaxTotalTokens < 0 || r.MaxSteps < 0 || r.MaxBudgetUSD < 0 || math.IsNaN(r.MaxBudgetUSD) || math.IsInf(r.MaxBudgetUSD, 0) {
		return errors.New("run limits must be finite and non-negative")
	}
	return nil
}

func (g GenerationParams) Validate() error {
	if g.Temperature != nil && (math.IsNaN(*g.Temperature) || math.IsInf(*g.Temperature, 0) || *g.Temperature < 0 || *g.Temperature > 2) {
		return errors.New("generation temperature must be between 0 and 2")
	}
	if g.TopP != nil && (math.IsNaN(*g.TopP) || math.IsInf(*g.TopP, 0) || *g.TopP < 0 || *g.TopP > 1) {
		return errors.New("generation top-p must be between 0 and 1")
	}
	if g.MaxTokens != nil && *g.MaxTokens <= 0 {
		return errors.New("generation max tokens must be positive")
	}
	for i, stop := range g.Stop {
		if stop == "" {
			return fmt.Errorf("generation stop sequence %d is empty", i+1)
		}
	}
	return nil
}

func (o Outcome) Validate() error {
	if o.Problem != nil {
		if err := o.Problem.Validate(); err != nil {
			return fmt.Errorf("outcome: %w", err)
		}
	}
	switch o.Status {
	case OutcomeCompleted:
		if strings.TrimSpace(o.Detail) != "" || o.Problem != nil {
			return errors.New("completed outcome cannot carry a problem or detail")
		}
	case OutcomeTimedOut, OutcomeFailed, OutcomeLost:
		if o.Problem == nil {
			return fmt.Errorf("%s outcome has no problem", o.Status)
		}
		if strings.TrimSpace(o.Detail) != "" {
			return fmt.Errorf("%s outcome carries a policy detail", o.Status)
		}
	case OutcomeMaxSteps, OutcomeMaxBudget, OutcomeCanceled:
		if o.Problem != nil {
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

func (b Block) validateLifecycle(completed bool) error {
	if err := b.validateEnvelope(completed); err != nil {
		return err
	}
	if err := b.validateAttachments(); err != nil {
		return err
	}
	return b.validateProjection()
}

func (b Block) validateEnvelope(completed bool) error {
	if strings.TrimSpace(b.ID) == "" {
		return errors.New("transcript block has no id")
	}
	if strings.TrimSpace(b.RunID) == "" {
		return fmt.Errorf("transcript block %s has no run id", b.ID)
	}
	if !slices.Contains([]BlockStatus{BlockStatusRunning, BlockStatusCompleted, BlockStatusIncomplete}, b.Status) {
		return fmt.Errorf("block %s has invalid status %q", b.ID, b.Status)
	}
	if completed == (b.Status == BlockStatusRunning) {
		return fmt.Errorf("block %s status %q disagrees with its event lifecycle", b.ID, b.Status)
	}
	if !slices.Contains([]BlockKind{BlockUser, BlockAssistant, BlockReasoning, BlockQuestion, BlockTool, BlockNotice, BlockError}, b.Kind) {
		return fmt.Errorf("block %s has invalid kind %q", b.ID, b.Kind)
	}
	if b.Status == BlockStatusRunning && !slices.Contains([]BlockKind{BlockAssistant, BlockReasoning, BlockTool}, b.Kind) {
		return fmt.Errorf("%s block %s cannot be running", b.Kind, b.ID)
	}
	wholeOnly := slices.Contains([]BlockKind{BlockUser, BlockQuestion, BlockNotice}, b.Kind)
	if wholeOnly && b.Status != BlockStatusCompleted {
		return fmt.Errorf("%s block %s must be completed", b.Kind, b.ID)
	}
	if b.Kind != BlockUser && len(b.Attachments) != 0 {
		return fmt.Errorf("%s block %s carries attachments", b.Kind, b.ID)
	}
	if b.Kind != BlockAssistant && len(b.Images) != 0 {
		return fmt.Errorf("%s block %s carries inline images", b.Kind, b.ID)
	}
	if b.Kind == BlockTool && !b.CreatedAt.IsZero() {
		return fmt.Errorf("tool block %s carries a message creation time", b.ID)
	}
	if b.Kind != BlockReasoning && b.Redacted {
		return fmt.Errorf("%s block %s is marked as redacted reasoning", b.Kind, b.ID)
	}
	if b.DroppedMessages < 0 {
		return fmt.Errorf("block %s has a negative dropped-message count", b.ID)
	}
	if b.Kind != BlockNotice && b.DroppedMessages != 0 {
		return fmt.Errorf("%s block %s carries a dropped-message count", b.Kind, b.ID)
	}
	return nil
}

func (b Block) validateAttachments() error {
	for i, attachment := range b.Attachments {
		if err := attachment.Validate(); err != nil {
			return fmt.Errorf("block %s attachment %d: %w", b.ID, i+1, err)
		}
	}
	for i, image := range b.Images {
		if err := image.Validate(); err != nil {
			return fmt.Errorf("block %s inline image %d: %w", b.ID, i+1, err)
		}
	}
	return nil
}

func (i InlineImage) Validate() error {
	var problems []error
	if strings.TrimSpace(i.ID) == "" {
		problems = append(problems, errors.New("id is empty"))
	}
	if strings.TrimSpace(i.Name) == "" {
		problems = append(problems, errors.New("name is empty"))
	}
	if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(i.MIMEType)), "image/") {
		problems = append(problems, errors.New("MIME type is not an image"))
	}
	if len(i.Data) == 0 {
		problems = append(problems, errors.New("data is empty"))
	}
	if err := errors.Join(problems...); err != nil {
		return fmt.Errorf("inline image: %w", err)
	}
	return nil
}

func (b Block) validateProjection() error {
	switch b.Kind {
	case BlockQuestion:
		return b.validateQuestionProjection()
	case BlockTool:
		return b.validateToolProjection()
	default:
		if b.Question != nil {
			return fmt.Errorf("%s block %s carries a question projection", b.Kind, b.ID)
		}
		if b.Tool != nil {
			return fmt.Errorf("%s block %s carries a tool projection", b.Kind, b.ID)
		}
	}
	return nil
}

func (b Block) validateQuestionProjection() error {
	if b.Question == nil {
		return fmt.Errorf("question block %s has no question projection", b.ID)
	}
	if b.Question.ItemID != b.ID {
		return fmt.Errorf("question block %s carries item id %s", b.ID, b.Question.ItemID)
	}
	if err := b.Question.Validate(); err != nil {
		return fmt.Errorf("block %s: %w", b.ID, err)
	}
	return nil
}

func (b Block) validateToolProjection() error {
	if b.Tool == nil {
		return fmt.Errorf("tool block %s has no tool projection", b.ID)
	}
	if err := b.Tool.Validate(); err != nil {
		return fmt.Errorf("block %s: %w", b.ID, err)
	}
	want := b.Tool.Status.blockStatus()
	if b.Status != want {
		return fmt.Errorf("block %s is %s while its tool is %s", b.ID, b.Status, b.Tool.Status)
	}
	return nil
}

func (t ToolStatus) blockStatus() BlockStatus {
	switch t {
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

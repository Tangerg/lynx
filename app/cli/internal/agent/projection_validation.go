package agent

import (
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
	switch o.Status {
	case OutcomeCompleted:
		if strings.TrimSpace(o.Error) != "" || strings.TrimSpace(o.Detail) != "" {
			return errors.New("completed outcome cannot carry an error or detail")
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
	default:
		return fmt.Errorf("outcome status %q is invalid", o.Status)
	}
	return nil
}

func (u Usage) Validate() error {
	if u.InputTokens < 0 || u.OutputTokens < 0 || u.CacheReadTokens < 0 || u.CacheWriteTokens < 0 || u.ReasoningTokens < 0 {
		return errors.New("usage token counts cannot be negative")
	}
	if u.CostUSD != nil && (*u.CostUSD < 0 || math.IsNaN(*u.CostUSD) || math.IsInf(*u.CostUSD, 0)) {
		return errors.New("usage cost must be finite and non-negative when known")
	}
	if u.Duration < 0 {
		return errors.New("usage duration cannot be negative")
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
	return nil
}

func (block Block) validateAttachments() error {
	for i, attachment := range block.Attachments {
		if err := attachment.Validate(); err != nil {
			return fmt.Errorf("block %s attachment %d: %w", block.ID, i+1, err)
		}
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
	switch {
	case next.InputTokens < previous.InputTokens:
		return errors.New("input-token usage regressed")
	case next.OutputTokens < previous.OutputTokens:
		return errors.New("output-token usage regressed")
	case next.CacheReadTokens < previous.CacheReadTokens:
		return errors.New("cache-read usage regressed")
	case next.CacheWriteTokens < previous.CacheWriteTokens:
		return errors.New("cache-write usage regressed")
	case next.ReasoningTokens < previous.ReasoningTokens:
		return errors.New("reasoning-token usage regressed")
	case previous.CostUSD != nil && next.CostUSD == nil:
		return errors.New("usage cost became unknown")
	case previous.CostUSD != nil && next.CostUSD != nil && *next.CostUSD < *previous.CostUSD:
		return errors.New("cost usage regressed")
	case next.Duration < previous.Duration:
		return errors.New("active duration regressed")
	default:
		return nil
	}
}

func validateInteractionItem(interaction Interaction, block Block) error {
	itemID := InteractionItemID(interaction)
	if block.ID != itemID {
		return fmt.Errorf("interaction item %s resolved to block %s", itemID, block.ID)
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

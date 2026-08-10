package agent

import (
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
)

// Validate checks the runtime-neutral run projection.
func (r Run) Validate() error {
	var problems []error
	if strings.TrimSpace(r.ID) == "" {
		problems = append(problems, errors.New("id is empty"))
	}
	if strings.TrimSpace(r.SessionID) == "" {
		problems = append(problems, errors.New("session id is empty"))
	}
	if !slices.Contains([]RunStatus{RunActive, RunWaiting, RunComplete}, r.Status) {
		problems = append(problems, fmt.Errorf("status %q is invalid", r.Status))
	}
	if err := errors.Join(problems...); err != nil {
		return fmt.Errorf("run: %w", err)
	}
	return nil
}

// Validate checks every explicitly selected run option. Empty values retain the
// runtime's defaults.
func (o RunOptions) Validate() error {
	var problems []error
	if o.Mode != "" && !slices.Contains([]AgentMode{ModeBuild, ModePlan, ModeReview}, o.Mode) {
		problems = append(problems, fmt.Errorf("mode %q is invalid", o.Mode))
	}
	if o.Permission != "" && !slices.Contains([]PermissionMode{PermissionAsk, PermissionReadOnly, PermissionAutoEdit, PermissionFull}, o.Permission) {
		problems = append(problems, fmt.Errorf("permission %q is invalid", o.Permission))
	}
	if o.Effort != "" && !slices.Contains([]string{"low", "medium", "high", "max", "ultra"}, o.Effort) {
		problems = append(problems, fmt.Errorf("effort %q is invalid", o.Effort))
	}
	if err := errors.Join(problems...); err != nil {
		return fmt.Errorf("run options: %w", err)
	}
	return nil
}

// Validate checks the closed outcome vocabulary and its error semantics.
func (o Outcome) Validate() error {
	switch o.Status {
	case OutcomeCompleted, OutcomeCanceled:
		if strings.TrimSpace(o.Error) != "" {
			return fmt.Errorf("outcome %q cannot carry an error", o.Status)
		}
	case OutcomeFailed:
		if strings.TrimSpace(o.Error) == "" {
			return errors.New("failed outcome has no error")
		}
	default:
		return fmt.Errorf("outcome status %q is invalid", o.Status)
	}
	return nil
}

// Validate rejects accounting values that cannot represent real usage.
func (u Usage) Validate() error {
	if u.InputTokens < 0 || u.OutputTokens < 0 || u.CachedTokens < 0 {
		return errors.New("usage token counts cannot be negative")
	}
	if u.CostUSD < 0 || math.IsNaN(u.CostUSD) || math.IsInf(u.CostUSD, 0) {
		return errors.New("usage cost must be finite and non-negative")
	}
	if u.Duration < 0 {
		return errors.New("usage duration cannot be negative")
	}
	return nil
}

// ValidateEvent checks the closed event payload independently from aggregate
// phase and envelope identity.
func ValidateEvent(event Event) error {
	if event == nil {
		return errors.New("event is nil")
	}
	if handled, err := validateRunEvent(event); handled {
		return err
	}
	return validateProgressEvent(event)
}

func validateRunEvent(event Event) (bool, error) {
	switch item := event.(type) {
	case RunStarted:
		return true, validateRunStarted(item)
	case RunResumed:
		if strings.TrimSpace(item.InterruptID) == "" {
			return true, errors.New("run resumed without an interrupt id")
		}
		return true, nil
	case RunInterrupted:
		return true, ValidateInteraction(item.Interaction)
	case RunFinished:
		return true, validateRunFinished(item)
	default:
		return false, nil
	}
}

func validateProgressEvent(event Event) error {
	switch item := event.(type) {
	case BlockStarted:
		return validateBlock(item.Block, false)
	case BlockDelta:
		if strings.TrimSpace(item.BlockID) == "" {
			return errors.New("block delta without a block id")
		}
	case BlockCompleted:
		return validateBlock(item.Block, true)
	case PlanChanged:
		return validatePlan(item.Items)
	default:
		return fmt.Errorf("event %T is unsupported", event)
	}
	return nil
}

func validateRunStarted(event RunStarted) error {
	if strings.TrimSpace(event.RunID) == "" {
		return errors.New("run started without an id")
	}
	if strings.TrimSpace(event.SessionID) == "" {
		return errors.New("run started without a session id")
	}
	return event.Options.Validate()
}

func validateRunFinished(event RunFinished) error {
	if err := event.Outcome.Validate(); err != nil {
		return err
	}
	return event.Usage.Validate()
}

func validateBlock(block Block, completed bool) error {
	if err := validateBlockIdentity(block); err != nil {
		return err
	}
	if err := validateBlockAttachments(block); err != nil {
		return err
	}
	return validateBlockTool(block, completed)
}

func validateBlockIdentity(block Block) error {
	if strings.TrimSpace(block.ID) == "" {
		return errors.New("transcript block has no id")
	}
	if !slices.Contains([]BlockKind{BlockUser, BlockAssistant, BlockReasoning, BlockTool, BlockNotice, BlockError}, block.Kind) {
		return fmt.Errorf("block %s has invalid kind %q", block.ID, block.Kind)
	}
	return nil
}

func validateBlockAttachments(block Block) error {
	if block.Kind != BlockUser && len(block.Attachments) != 0 {
		return fmt.Errorf("%s block %s carries attachments", block.Kind, block.ID)
	}
	for i, attachment := range block.Attachments {
		if err := attachment.Validate(); err != nil {
			return fmt.Errorf("block %s attachment %d: %w", block.ID, i+1, err)
		}
	}
	return nil
}

func validateBlockTool(block Block, completed bool) error {
	if block.Kind != BlockTool {
		if block.Tool != nil {
			return fmt.Errorf("%s block %s carries a tool projection", block.Kind, block.ID)
		}
		return nil
	}
	if block.Tool == nil {
		return fmt.Errorf("tool block %s has no tool projection", block.ID)
	}
	if err := block.Tool.Validate(); err != nil {
		return fmt.Errorf("block %s: %w", block.ID, err)
	}
	if !completed && block.Tool.Status != ToolRunning {
		return fmt.Errorf("block %s started with tool status %q", block.ID, block.Tool.Status)
	}
	if completed && block.Tool.Status == ToolRunning {
		return fmt.Errorf("block %s completed while tool is still running", block.ID)
	}
	return nil
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

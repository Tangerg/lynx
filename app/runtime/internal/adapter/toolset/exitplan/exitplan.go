// Package exitplan exposes the root Agent's exit_plan_mode tool. It reads the
// canonical session Plan, asks the user to approve that exact value, and only
// then restores the permission mode captured on entry.
package exitplan

import (
	"context"
	"errors"
	"fmt"

	toolcontract "github.com/Tangerg/lynx/tool"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/executionctx"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/planpresentation"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/approval"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/plan"
)

const (
	toolName     = "exit_plan_mode"
	approveLabel = "Approve"
	rejectLabel  = "Reject"
)

const description = `Request approval for the current session Plan and exit Plan mode.

Call this only after set_plan contains the complete proposed Plan. This tool
reads that stored Plan; it takes no Plan text or alternatives, so the approved
value cannot differ from the stored session Plan. Approval restores the permission mode
captured by enter_plan_mode. Rejection keeps the session in read-only Plan mode.`

type exitArgs struct{}

// ModePolicy is the exit tool's complete session-mode view.
type ModePolicy interface {
	Mode(ctx context.Context, sessionID string) (approval.Mode, error)
	ExitPlanMode(ctx context.Context, sessionID string) (restored approval.Mode, changed bool, err error)
}

// PlanReader is the exit tool's read-only view of the canonical session Plan.
type PlanReader interface {
	List(ctx context.Context, sessionID string) ([]plan.Step, error)
}

type tool struct {
	modes     ModePolicy
	plan      PlanReader
	interrupt runs.InterruptFunc
}

// New builds exit_plan_mode. A missing mode policy or Plan reader disables the
// capability; the interrupt defaults to the runtime's unavailable responder.
func New(modes ModePolicy, plan PlanReader, interrupt runs.InterruptFunc) (toolcontract.Tool, error) {
	if modes == nil || plan == nil {
		return nil, nil
	}
	if interrupt == nil {
		interrupt = runs.InterruptUnavailable
	}
	return toolcontract.NewFunc[exitArgs, string](
		toolcontract.FuncConfig{Name: toolName, Description: description},
		(&tool{modes: modes, plan: plan, interrupt: interrupt}).exit,
	)
}

func (t *tool) exit(ctx context.Context, _ exitArgs) (string, error) {
	sessionID := executionctx.SessionID(ctx)
	if sessionID == "" {
		return "", errors.New("exit_plan_mode: no active session")
	}
	mode, err := t.modes.Mode(ctx, sessionID)
	if err != nil {
		return "", err
	}
	if mode != approval.ModePlan {
		return "", errors.New("exit_plan_mode: current session is not in Plan mode")
	}
	steps, err := t.plan.List(ctx, sessionID)
	if err != nil {
		return "", err
	}
	if len(steps) == 0 {
		return "", errors.New("exit_plan_mode: current Plan is empty; call set_plan before requesting approval")
	}
	if err := plan.Validate(steps); err != nil {
		return "", fmt.Errorf("exit_plan_mode: stored Plan is invalid: %w", err)
	}

	arguments := `{}`
	pending := runs.Interrupt{
		Kind: execution.QuestionInterrupt,
		Question: &runs.QuestionPrompt{
			ToolName:  toolName,
			Arguments: arguments,
			Fields: []runs.QuestionFieldSpec{{
				Prompt: planpresentation.Render(steps),
				Header: "Plan",
				Options: []runs.QuestionOptionSpec{
					{Label: approveLabel, Description: "Approve this Plan and allow execution"},
					{Label: rejectLabel, Description: "Keep Plan mode and revise the Plan"},
				},
			}},
		},
	}
	if err := pending.Validate(); err != nil {
		return "", fmt.Errorf("exit_plan_mode: %w", err)
	}
	resolution, err := t.interrupt(
		ctx,
		interrupts.InterruptKey(execution.QuestionInterrupt.String(), toolName, arguments),
		pending,
	)
	if err != nil {
		return "", err
	}
	if selectedChoice(resolution.Answers) != approveLabel {
		return "Plan not approved. The session remains in Plan mode; revise the Plan or continue investigating.", nil
	}
	restored, changed, err := t.modes.ExitPlanMode(ctx, sessionID)
	if err != nil {
		return "", err
	}
	if !changed {
		return "", errors.New("exit_plan_mode: Plan mode ended before approval was applied")
	}
	return fmt.Sprintf("Plan approved. Plan mode exited and permission mode %s was restored. Execute the Plan.", modeName(restored)), nil
}

func selectedChoice(answers [][]string) string {
	if len(answers) == 0 || len(answers[0]) == 0 {
		return ""
	}
	return answers[0][0]
}

func modeName(mode approval.Mode) string {
	switch mode {
	case approval.ModeSafe:
		return "safe"
	case approval.ModeBalanced:
		return "balanced"
	case approval.ModeYolo:
		return "yolo"
	default:
		return "unknown"
	}
}

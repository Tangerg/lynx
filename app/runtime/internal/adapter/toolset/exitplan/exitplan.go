// Package exitplan provides the exit_plan_mode tool — the model's way to leave
// the read-only plan stance. In plan mode (approval ModePlan) write/exec/network
// tools are denied, so the agent can only investigate and draft a plan; it then
// calls exit_plan_mode to present the plan for approval. On approval the stance
// flips to ModeBalanced (execute) and the loop continues with full tools; on
// rejection it stays in plan mode with the user's feedback. The pattern —
// a tool that presents the plan, gets approval, and lifts the read-only
// restriction — is the standard exit-plan-mechanism used by coding agents.
package exitplan

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec/suspension"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/approval"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
	"github.com/Tangerg/lynx/tools"
)

const (
	toolName     = "exit_plan_mode"
	approveLabel = "Approve"
	rejectLabel  = "Reject"
)

// exitPlanArgs is the model-facing argument shape; [tools.New] derives
// the JSON schema from it and decodes calls back into it, so the advertised
// schema and parsed value cannot drift. The options mirror [runs.Option]
// with the LLM-facing copy kept here.
type exitPlanArgs struct {
	Plan    string      `json:"plan" jsonschema:"required" jsonschema_description:"The plan to present for approval — a concise, ordered list of the steps you intend to take. Markdown is fine."`
	Options []optionArg `json:"options,omitempty" jsonschema:"maxItems=2" jsonschema_description:"Optional alternative approaches (up to 2) for the user to choose among. The chosen one is returned to you on approval."`
}

type optionArg struct {
	Label       string `json:"label" jsonschema:"required" jsonschema_description:"The approach shown to the user."`
	Description string `json:"description,omitempty" jsonschema_description:"Optional one-line explanation of the approach."`
}

func (a exitPlanArgs) validate() error {
	if strings.TrimSpace(a.Plan) == "" {
		return errors.New("plan is required")
	}
	if len(a.Options) > 2 {
		return errors.New("at most two alternative approaches are allowed")
	}
	return nil
}

func (a exitPlanArgs) prompt(arguments string) runs.QuestionPrompt {
	opts := []runs.QuestionOptionSpec{{Label: approveLabel, Description: "Proceed with this plan"}}
	for _, o := range a.Options {
		opts = append(opts, o.toInterrupt())
	}
	opts = append(opts, runs.QuestionOptionSpec{Label: rejectLabel, Description: "Don't proceed; refine the plan"})
	return runs.QuestionPrompt{
		ToolName:  toolName,
		Arguments: arguments,
		Questions: []runs.QuestionSpec{{
			Question: a.Plan,
			Header:   "Plan",
			Options:  opts,
		}},
	}
}

func (a exitPlanArgs) arguments() (string, error) {
	b, err := json.Marshal(a)
	if err != nil {
		return "", fmt.Errorf("exit_plan_mode: encode arguments: %w", err)
	}
	return string(b), nil
}

func (o optionArg) toInterrupt() runs.QuestionOptionSpec {
	return runs.QuestionOptionSpec{Label: o.Label, Description: o.Description}
}

type tool struct {
	approval  ModePolicy
	interrupt suspension.Func
}

// ModePolicy is the exit-plan tool's complete view of approval state.
type ModePolicy interface {
	Mode(ctx context.Context) (approval.Mode, error)
	SetMode(ctx context.Context, mode approval.Mode) error
}

// New builds the exit_plan_mode tool over the approval policy (it flips the
// stance to execute on approval). A nil policy yields a nil tool (omitted).
//
// The toolset composes the interrupt suspension contract from the composition
// root.
func New(appr ModePolicy, interrupt suspension.Func) (tools.Tool, error) {
	if interrupt == nil {
		interrupt = suspension.Unavailable
	}
	if appr == nil {
		return nil, nil
	}
	t := &tool{approval: appr, interrupt: interrupt}
	return tools.New[exitPlanArgs, string](
		tools.Config{
			Name:        toolName,
			Description: "Present your plan for approval and leave plan mode. Call this ONLY in plan mode (the read-only stance) once you've investigated and drafted a plan. On approval, plan mode exits and all tools are enabled so you can execute the plan; on rejection you stay in plan mode with the user's feedback. Provide alternative approaches in options when the user should choose between them.",
		},
		t.exit,
	)
}

func (t *tool) exit(ctx context.Context, in exitPlanArgs) (string, error) {
	if err := in.validate(); err != nil {
		return "", fmt.Errorf("exit_plan_mode: %w", err)
	}
	mode, err := t.approval.Mode(ctx)
	if err != nil {
		return "", err
	}
	if mode != approval.ModePlan {
		return "Not in plan mode — nothing to exit. exit_plan_mode only applies in the read-only plan stance.", nil
	}

	arguments, err := in.arguments()
	if err != nil {
		return "", err
	}
	prompt := in.prompt(arguments)
	pending := runs.Interrupt{Kind: runs.QuestionInterruptKind, Question: &prompt}
	if err := pending.Validate(); err != nil {
		return "", fmt.Errorf("exit_plan_mode: %w", err)
	}
	res, err := t.interrupt(ctx,
		interrupts.InterruptKey(string(runs.QuestionInterruptKind), toolName, arguments),
		pending,
	)
	if err != nil {
		return "", err
	}
	return t.applyChoice(ctx, selectedChoice(res.Answer))
}

func (t *tool) applyChoice(ctx context.Context, choice string) (string, error) {
	if choice == "" || choice == rejectLabel {
		return "Plan not approved. Refine it and call exit_plan_mode again, or keep investigating (read-only).", nil
	}
	if err := t.approval.SetMode(ctx, approval.ModeBalanced); err != nil {
		return "", err
	}
	if choice != approveLabel {
		return "Plan approved — selected approach: " + choice + ". Plan mode exited; all tools are enabled. Execute that approach.", nil
	}
	return "Plan approved. Plan mode exited; all tools are enabled. Execute the plan.", nil
}

func selectedChoice(answer map[string][]string) string {
	if v := answer[runs.QuestionFieldID(0)]; len(v) > 0 {
		return v[0]
	}
	return ""
}

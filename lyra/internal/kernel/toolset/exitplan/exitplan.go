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
	"hash/fnv"
	"strconv"
	"strings"

	"github.com/Tangerg/lynx/agent/hitl"
	"github.com/Tangerg/lynx/core/model/chat"
	pkgjson "github.com/Tangerg/lynx/pkg/json"

	"github.com/Tangerg/lynx/lyra/internal/domain/approval"
	"github.com/Tangerg/lynx/lyra/internal/domain/interrupts"
)

const (
	toolName     = "exit_plan_mode"
	approveLabel = "Approve"
	rejectLabel  = "Reject"
)

// exitPlanArgs is the model-facing argument shape; it drives the JSON schema
// ([schema]) so the parsed struct and the advertised schema can't drift. The
// options mirror [interrupts.Option] with the LLM-facing copy kept here.
type exitPlanArgs struct {
	Plan    string      `json:"plan" jsonschema:"required" jsonschema_description:"The plan to present for approval — a concise, ordered list of the steps you intend to take. Markdown is fine."`
	Options []optionArg `json:"options,omitempty" jsonschema_description:"Optional alternative approaches (2-3) for the user to choose among. The chosen one is returned to you on approval."`
}

type optionArg struct {
	Label       string `json:"label" jsonschema:"required" jsonschema_description:"The approach shown to the user."`
	Description string `json:"description,omitempty" jsonschema_description:"Optional one-line explanation of the approach."`
}

var schema = pkgjson.MustStringDefSchemaOf(exitPlanArgs{})

func (o optionArg) toInterrupt() interrupts.Option {
	return interrupts.Option{Label: o.Label, Description: o.Description}
}

// New builds the exit_plan_mode tool over the approval service (it flips the
// stance to execute on approval). A nil service yields a nil tool (omitted).
func New(appr approval.Service) chat.Tool {
	if appr == nil {
		return nil
	}
	t, _ := chat.NewTool(
		chat.ToolDefinition{
			Name: toolName,
			Description: "Present your plan for approval and leave plan mode. Call this ONLY in plan mode (the read-only stance) once you've investigated and drafted a plan. On approval, plan mode exits and all tools are enabled so you can execute the plan; on rejection you stay in plan mode with the user's feedback. Provide alternative approaches in options when the user should choose between them.",
			InputSchema: schema,
		},
		chat.ToolMetadata{},
		func(ctx context.Context, arguments string) (string, error) {
			var in exitPlanArgs
			if err := json.Unmarshal([]byte(arguments), &in); err != nil {
				return "", fmt.Errorf("exit_plan_mode: invalid arguments: %w", err)
			}
			if strings.TrimSpace(in.Plan) == "" {
				return "", errors.New("exit_plan_mode: plan is required")
			}
			mode, err := appr.GetMode(ctx)
			if err != nil {
				return "", err
			}
			if mode != approval.ModePlan {
				return "Not in plan mode — nothing to exit. exit_plan_mode only applies in the read-only plan stance.", nil
			}

			// Present the plan as a choice: Approve / (alternatives) / Reject.
			// Reject keeps plan mode; anything else approves and names the chosen
			// approach. Reuses the structured-question HITL path (same as ask_user).
			opts := []interrupts.Option{{Label: approveLabel, Description: "Proceed with this plan"}}
			for _, o := range in.Options {
				opts = append(opts, o.toInterrupt())
			}
			opts = append(opts, interrupts.Option{Label: rejectLabel, Description: "Don't proceed; refine the plan"})
			prompt := interrupts.QuestionPrompt{Questions: []interrupts.Question{{
				Question: in.Plan,
				Header:   "Plan",
				Options:  opts,
			}}}

			res, _, err := hitl.Interrupt[interrupts.Resolution](ctx, key(arguments), prompt)
			if err != nil {
				return "", err
			}
			choice := ""
			if v := res.Answer[interrupts.QuestionFieldName(0)]; len(v) > 0 {
				choice = v[0]
			}
			if choice == "" || choice == rejectLabel {
				return "Plan not approved. Refine it and call exit_plan_mode again, or keep investigating (read-only).", nil
			}
			if err := appr.SetMode(ctx, approval.ModeBalanced); err != nil {
				return "", err
			}
			if choice != approveLabel {
				return "Plan approved — selected approach: " + choice + ". Plan mode exited; all tools are enabled. Execute that approach.", nil
			}
			return "Plan approved. Plan mode exited; all tools are enabled. Execute the plan.", nil
		},
	)
	return t
}

// key is the interrupt key for one exit_plan_mode call — keyed by arguments so
// the parked plan re-presents at the same call site on resume (mirrors ask_user).
func key(arguments string) string {
	h := fnv.New64a()
	_, _ = h.Write([]byte(toolName))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(arguments))
	return toolName + "." + strconv.FormatUint(h.Sum64(), 16)
}

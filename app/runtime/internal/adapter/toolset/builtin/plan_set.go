package builtin

import (
	"context"
	"errors"

	toolcontract "github.com/Tangerg/lynx/tool"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/executionctx"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/planpresentation"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/toolname"
	plandomain "github.com/Tangerg/lynx/app/runtime/internal/domain/plan"
)

const setDescription = `Set the current session's execution plan.

Use this for non-trivial multi-step work and whenever the plan or progress
changes. Pass the complete ordered list every time; this call replaces the
previous plan. Use an empty steps array to clear it. Each step needs a concise
description and a status: pending, in_progress, or completed. At most one step
may be in_progress. This changes Agent state only; it does not modify files or
enter or exit Plan mode.`

type setArgs struct {
	Steps []stepArg `json:"steps" jsonschema_description:"The complete ordered Plan. Replaces the current Plan; an empty array clears it."`
}

type stepArg struct {
	Description string `json:"description" jsonschema:"minLength=1" jsonschema_description:"Concise description of the work represented by this Step."`
	Status      string `json:"status" jsonschema:"enum=pending,enum=in_progress,enum=completed" jsonschema_description:"pending = not started; in_progress = active work; completed = fully done."`
}

func (a setArgs) steps() []plandomain.Step {
	steps := make([]plandomain.Step, len(a.Steps))
	for index, step := range a.Steps {
		steps[index] = plandomain.Step{Description: step.Description, Status: plandomain.Status(step.Status)}
	}
	return steps
}

type setter struct{ plans planReplacer }

func newSet(plans planReplacer) (toolcontract.Tool, error) {
	if plans == nil {
		return nil, nil
	}
	return toolcontract.NewFunc[setArgs, string](
		toolcontract.FuncConfig{Name: toolname.SetPlan, Description: setDescription},
		(&setter{plans: plans}).set,
	)
}

func (s *setter) set(ctx context.Context, args setArgs) (string, error) {
	sessionID := executionctx.SessionID(ctx)
	if sessionID == "" {
		return "", errors.New("set_plan: no active session")
	}
	state, err := s.plans.Replace(ctx, sessionID, args.steps())
	if err != nil {
		return "", err
	}
	steps := state.Steps()
	if rendered := planpresentation.Render(steps); rendered != "" {
		return "Plan updated:\n" + rendered, nil
	}
	return "Plan cleared.", nil
}

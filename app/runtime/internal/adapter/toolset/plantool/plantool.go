// Package plantool exposes the root Agent's set_plan tool over a narrow Plan
// persistence view. Plan invariants stay in the domain; model-visible naming,
// schema, and response text stay here.
package plantool

import (
	"context"
	"errors"

	toolcontract "github.com/Tangerg/lynx/tool"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/executionctx"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/planpresentation"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/plan"
)

const description = `Set the current session's execution plan.

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

func (a setArgs) steps() []plan.Step {
	steps := make([]plan.Step, len(a.Steps))
	for index, step := range a.Steps {
		steps[index] = plan.Step{Description: step.Description, Status: plan.Status(step.Status)}
	}
	return steps
}

// Store is the set_plan tool's complete persistence need. Session lifecycle
// cleanup and archive behavior belong to their own consumers.
type Store interface {
	Replace(ctx context.Context, sessionID string, steps []plan.Step) error
}

type setter struct{ store Store }

// New builds set_plan. A nil store disables the capability and returns a nil
// tool so composition can omit it.
func New(store Store) (toolcontract.Tool, error) {
	if store == nil {
		return nil, nil
	}
	return toolcontract.NewFunc[setArgs, string](
		toolcontract.FuncConfig{Name: "set_plan", Description: description},
		(&setter{store: store}).set,
	)
}

func (s *setter) set(ctx context.Context, args setArgs) (string, error) {
	sessionID := executionctx.SessionID(ctx)
	if sessionID == "" {
		return "", errors.New("set_plan: no active session")
	}
	steps := args.steps()
	if err := plan.Validate(steps); err != nil {
		return "", err
	}
	if err := s.store.Replace(ctx, sessionID, steps); err != nil {
		return "", err
	}
	if rendered := planpresentation.Render(steps); rendered != "" {
		return "Plan updated:\n" + rendered, nil
	}
	return "Plan cleared.", nil
}

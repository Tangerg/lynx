// Plan tools expose the root Agent's Plan lifecycle.
package builtin

import (
	"context"
	"fmt"

	toolcontract "github.com/Tangerg/scope/core/tool"

	"github.com/Tangerg/scope/app/runtime/internal/application/runs"
	"github.com/Tangerg/scope/app/runtime/internal/domain/approval"
	plandomain "github.com/Tangerg/scope/app/runtime/internal/domain/plan"
)

// planStateReader supplies the canonical session Plan approved by exit_plan_mode.
type planStateReader interface {
	State(ctx context.Context, sessionID string) (plandomain.State, error)
}

// planReplacer executes the application use case behind set_plan.
type planReplacer interface {
	Replace(ctx context.Context, sessionID string, steps []plandomain.Step) (plandomain.State, error)
}

// PlanUseCases is the Plan application surface consumed by this tool family.
type PlanUseCases interface {
	State(ctx context.Context, sessionID string) (plandomain.State, error)
	Replace(ctx context.Context, sessionID string, steps []plandomain.Step) (plandomain.State, error)
}

// planEnterPolicy narrows one session into Plan mode.
type planEnterPolicy interface {
	EnterPlanMode(ctx context.Context, sessionID string) (changed bool, err error)
}

// planExitPolicy reads and restores one session's permission mode.
type planExitPolicy interface {
	Mode(ctx context.Context, sessionID string) (approval.Mode, error)
	ExitPlanMode(ctx context.Context, sessionID string) (restored approval.Mode, changed bool, err error)
}

// PlanModePolicy owns both transitions of the same session Plan-mode lifecycle.
type PlanModePolicy interface {
	EnterPlanMode(ctx context.Context, sessionID string) (changed bool, err error)
	Mode(ctx context.Context, sessionID string) (approval.Mode, error)
	ExitPlanMode(ctx context.Context, sessionID string) (restored approval.Mode, changed bool, err error)
}

// PlanTools contains the three tools that implement one Plan lifecycle.
type PlanTools struct {
	Enter toolcontract.Tool
	Set   toolcontract.Tool
	Exit  toolcontract.Tool
}

// BuildPlan constructs every available Plan tool while preserving the family
// invariant that exit_plan_mode approves the same application state replaced by
// set_plan.
func BuildPlan(modes PlanModePolicy, plans PlanUseCases, interrupt runs.InterruptFunc) (PlanTools, error) {
	enter, err := newEnter(modes)
	if err != nil {
		return PlanTools{}, fmt.Errorf("plan: build enter_plan_mode: %w", err)
	}
	set, err := newSet(plans)
	if err != nil {
		return PlanTools{}, fmt.Errorf("plan: build set_plan: %w", err)
	}
	exit, err := newExit(modes, plans, interrupt)
	if err != nil {
		return PlanTools{}, fmt.Errorf("plan: build exit_plan_mode: %w", err)
	}
	return PlanTools{Enter: enter, Set: set, Exit: exit}, nil
}

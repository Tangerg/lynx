// Package plan exposes the root Agent's Plan lifecycle tools.
package plan

import (
	"context"
	"fmt"

	toolcontract "github.com/Tangerg/lynx/tool"

	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/approval"
	plandomain "github.com/Tangerg/lynx/app/runtime/internal/domain/plan"
)

// reader supplies the canonical session Plan approved by exit_plan_mode.
type reader interface {
	State(ctx context.Context, sessionID string) (plandomain.State, error)
}

// replacer executes the application use case behind set_plan.
type replacer interface {
	Replace(ctx context.Context, sessionID string, steps []plandomain.Step) (plandomain.State, error)
}

// UseCases is the Plan application surface consumed by this tool family.
type UseCases interface {
	State(ctx context.Context, sessionID string) (plandomain.State, error)
	Replace(ctx context.Context, sessionID string, steps []plandomain.Step) (plandomain.State, error)
}

// enterPolicy narrows one session into Plan mode.
type enterPolicy interface {
	EnterPlanMode(ctx context.Context, sessionID string) (changed bool, err error)
}

// exitPolicy reads and restores one session's permission mode.
type exitPolicy interface {
	Mode(ctx context.Context, sessionID string) (approval.Mode, error)
	ExitPlanMode(ctx context.Context, sessionID string) (restored approval.Mode, changed bool, err error)
}

// ModePolicy owns both transitions of the same session Plan-mode lifecycle.
type ModePolicy interface {
	EnterPlanMode(ctx context.Context, sessionID string) (changed bool, err error)
	Mode(ctx context.Context, sessionID string) (approval.Mode, error)
	ExitPlanMode(ctx context.Context, sessionID string) (restored approval.Mode, changed bool, err error)
}

// Family contains the three tools that implement one Plan lifecycle.
type Family struct {
	Enter toolcontract.Tool
	Set   toolcontract.Tool
	Exit  toolcontract.Tool
}

// Build constructs every available Plan tool while preserving the family
// invariant that exit_plan_mode approves the same application state replaced by
// set_plan.
func Build(modes ModePolicy, plans UseCases, interrupt runs.InterruptFunc) (Family, error) {
	enter, err := newEnter(modes)
	if err != nil {
		return Family{}, fmt.Errorf("plan: build enter_plan_mode: %w", err)
	}
	set, err := newSet(plans)
	if err != nil {
		return Family{}, fmt.Errorf("plan: build set_plan: %w", err)
	}
	exit, err := newExit(modes, plans, interrupt)
	if err != nil {
		return Family{}, fmt.Errorf("plan: build exit_plan_mode: %w", err)
	}
	return Family{Enter: enter, Set: set, Exit: exit}, nil
}

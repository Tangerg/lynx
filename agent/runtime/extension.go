package runtime

import (
	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/event"
	"github.com/Tangerg/lynx/agent/plan"
	"github.com/Tangerg/lynx/agent/plan/planner/goap"
	"github.com/Tangerg/lynx/agent/plan/planner/reactive"
)

// EventListener is the [event.Event] subscriber capability — runtime
// counterpart to the marker interfaces in [core]. It lives in runtime
// because [event.Event] is tied to the framework's concrete event types
// and putting it in core would create an import cycle (event → core →
// event). Implementing EventListener registers the value with the
// platform's multicast at boot.
//
// A registered EventListener also satisfies the simpler [event.Listener]
// (which the multicast accepts directly), so the runtime forwards the
// extension straight to [event.Multicast.Add].
type EventListener interface {
	core.Extension

	OnEvent(e event.Event)
}

// PlannerFactory builds a [plan.Planner] for a given [core.PlannerType].
// The runtime calls the most-recently-registered factory at process
// creation; built-in fallback is the GOAP A* planner. Lives in runtime
// (not core) to avoid a core → plan dependency cycle.
type PlannerFactory interface {
	core.Extension

	NewPlanner(plannerType core.PlannerType) plan.Planner
}

// defaultPlannerFactory is the built-in fallback. Dispatches on
// PlannerType:
//
//   - PlannerGOAP / unknown → A* GOAP planner.
//   - PlannerReactive       → reactive (greedy one-step) planner.
//   - PlannerHTN            → nil. HTN needs a user-supplied task
//     library; callers wanting HTN must register their own
//     PlannerFactory extension that returns a configured *htn.Planner.
type defaultPlannerFactory struct{}

func (defaultPlannerFactory) Name() string { return "default-planner-factory" }

func (defaultPlannerFactory) NewPlanner(t core.PlannerType) plan.Planner {
	switch t {
	case core.PlannerReactive:
		return reactive.NewPlanner()
	case core.PlannerHTN:
		return nil
	default:
		return goap.NewAStarPlanner()
	}
}

// DefaultPlannerFactory returns the framework's fallback PlannerFactory.
// Exported so tests / advanced configurations can pass it through
// PlatformConfig.Extensions explicitly.
func DefaultPlannerFactory() PlannerFactory { return defaultPlannerFactory{} }

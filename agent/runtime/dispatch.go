package runtime

import (
	"context"
	"fmt"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/event"
)

// extensionRegistry is the dedup-aware container the platform uses to
// hold registered extensions. Insertion order is preserved (drives
// onion / wrap chain ordering); empty Name and duplicate Name both
// panic at registration time — boot-time misconfiguration must fail
// fast.
type extensionRegistry struct {
	list   []core.Extension
	byName map[string]core.Extension
}

func newExtensionRegistry() extensionRegistry {
	return extensionRegistry{byName: map[string]core.Extension{}}
}

// register adds ext to the registry. Panics on nil ext, empty Name, or
// duplicate Name within this registry — boot-time errors must surface
// loudly so they can't ride into production silently.
func (r *extensionRegistry) register(scope string, ext core.Extension) {
	if ext == nil {
		panic(fmt.Sprintf("runtime: nil extension in %s", scope))
	}
	name := ext.Name()
	if name == "" {
		panic(fmt.Sprintf("runtime: extension %T returned empty Name() in %s", ext, scope))
	}
	if _, dup := r.byName[name]; dup {
		panic(fmt.Sprintf("runtime: extension %q already registered in %s", name, scope))
	}
	r.byName[name] = ext
	r.list = append(r.list, ext)
}

// ============================================================================
// Capability collectors — return the slice of extensions implementing a
// specific capability, in registration order. Cheap (linear scan over
// a small slice) and called at most once per dispatch site.
// ============================================================================

func collectActionInterceptors(extensions []core.Extension) []core.ActionInterceptor {
	var out []core.ActionInterceptor
	for _, ext := range extensions {
		if h, ok := ext.(core.ActionInterceptor); ok {
			out = append(out, h)
		}
	}
	return out
}

func collectToolDecorators(extensions []core.Extension) []core.ToolDecorator {
	var out []core.ToolDecorator
	for _, ext := range extensions {
		if d, ok := ext.(core.ToolDecorator); ok {
			out = append(out, d)
		}
	}
	return out
}

func collectAgentValidators(extensions []core.Extension) []core.AgentValidator {
	var out []core.AgentValidator
	for _, ext := range extensions {
		if v, ok := ext.(core.AgentValidator); ok {
			out = append(out, v)
		}
	}
	return out
}

func collectGoalApprovers(extensions []core.Extension) []core.GoalApprover {
	var out []core.GoalApprover
	for _, ext := range extensions {
		if a, ok := ext.(core.GoalApprover); ok {
			out = append(out, a)
		}
	}
	return out
}

func collectToolGroupResolvers(extensions []core.Extension) []core.ToolGroupResolver {
	var out []core.ToolGroupResolver
	for _, ext := range extensions {
		if r, ok := ext.(core.ToolGroupResolver); ok {
			out = append(out, r)
		}
	}
	return out
}

// lastIDGenerator returns the most-recently-registered IDGenerator (so a
// process-scope override beats a platform-scope baseline). Returns nil
// when none is registered — callers fall back to the runtime default.
func lastIDGenerator(extensions []core.Extension) core.IDGenerator {
	for i := len(extensions) - 1; i >= 0; i-- {
		if g, ok := extensions[i].(core.IDGenerator); ok {
			return g
		}
	}
	return nil
}

// lastPlannerFactory mirrors lastIDGenerator for PlannerFactory.
func lastPlannerFactory(extensions []core.Extension) PlannerFactory {
	for i := len(extensions) - 1; i >= 0; i-- {
		if f, ok := extensions[i].(PlannerFactory); ok {
			return f
		}
	}
	return nil
}

// lastBlackboardFactory mirrors lastIDGenerator for BlackboardFactory.
func lastBlackboardFactory(extensions []core.Extension) core.BlackboardFactory {
	for i := len(extensions) - 1; i >= 0; i-- {
		if f, ok := extensions[i].(core.BlackboardFactory); ok {
			return f
		}
	}
	return nil
}

// ============================================================================
// Chain runners
// ============================================================================

// runActionInterceptors executes the onion chain. The first registered
// interceptor is the outermost — its InterceptAction wraps everything
// after it (matches net/http middleware ordering). base is the inner
// "actually run the action" closure invoked when every interceptor has
// called next().
func runActionInterceptors(
	interceptors []core.ActionInterceptor,
	ctx context.Context,
	process core.Process,
	action core.Action,
	base func() core.ActionStatus,
) core.ActionStatus {
	if len(interceptors) == 0 {
		return base()
	}
	var run func(i int) core.ActionStatus
	run = func(i int) core.ActionStatus {
		if i >= len(interceptors) {
			return base()
		}
		return interceptors[i].InterceptAction(ctx, process, action, func() core.ActionStatus {
			return run(i + 1)
		})
	}
	return run(0)
}

// runToolDecorators wraps the supplied tool through every decorator in
// registration order. First decorator is the innermost wrap; later
// decorators see the result of earlier decorators. A decorator may
// return its input unchanged to no-op.
func runToolDecorators(
	decorators []core.ToolDecorator,
	process core.Process,
	action core.Action,
	tool core.AgentTool,
) core.AgentTool {
	for _, d := range decorators {
		tool = d.DecorateTool(process, action, tool)
	}
	return tool
}

// runAgentValidators runs every validator in order; the first error
// wins (fail-fast). The error is wrapped with the validator's Name so
// the failure is attributable.
func runAgentValidators(validators []core.AgentValidator, agent *core.Agent) error {
	for _, v := range validators {
		if err := v.ValidateAgent(agent); err != nil {
			return fmt.Errorf("validator %q: %w", v.Name(), err)
		}
	}
	return nil
}

// runGoalApprovers returns true only when every approver returns true
// (conjunction — any false vetoes). Empty approver list trivially
// approves.
func runGoalApprovers(approvers []core.GoalApprover, process core.Process, goal *core.Goal) bool {
	for _, a := range approvers {
		if !a.ApproveGoal(process, goal) {
			return false
		}
	}
	return true
}

// runToolGroupResolvers walks resolvers in order; the first non-nil
// group returned wins (a resolver returning a nil group + nil error
// means "I don't know this role, ask the next one"). Any resolver
// error short-circuits the chain.
func runToolGroupResolvers(
	resolvers []core.ToolGroupResolver,
	ctx context.Context,
	requirement core.ToolGroupRequirement,
) (core.ToolGroup, error) {
	for _, r := range resolvers {
		group, err := r.Resolve(ctx, requirement)
		if err != nil {
			return nil, fmt.Errorf("resolver %q: %w", r.Name(), err)
		}
		if group != nil {
			return group, nil
		}
	}
	return nil, nil
}

// ============================================================================
// EventListener helper
// ============================================================================

// addEventListenerExtensions walks the supplied extensions, adding any
// that implement EventListener to the multicast. EventListener
// satisfies event.Listener so it can plug straight in.
func addEventListenerExtensions(multicast *event.Multicast, extensions []core.Extension) {
	for _, ext := range extensions {
		if l, ok := ext.(EventListener); ok {
			multicast.Add(l)
		}
	}
}

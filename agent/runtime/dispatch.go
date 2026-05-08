package runtime

import (
	"context"
	"fmt"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/event"
)

// extensionRegistry is the dedup-aware container the platform uses
// to hold registered extensions. Insertion order is preserved
// (drives onion / wrap chain ordering); empty Name and duplicate
// Name both panic at registration time — boot-time misconfiguration
// must fail fast.
type extensionRegistry struct {
	list   []core.Extension
	byName map[string]core.Extension
}

func newExtensionRegistry() extensionRegistry {
	return extensionRegistry{byName: map[string]core.Extension{}}
}

// register adds ext to the registry. Panics on nil ext, empty Name,
// or duplicate Name.
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

// collectExtensions returns every extension that implements T, in
// registration order. Used for fan-out / chain capabilities
// (interceptor, decorator, validator, approver, resolver).
func collectExtensions[T any](extensions []core.Extension) []T {
	var out []T
	for _, ext := range extensions {
		if v, ok := ext.(T); ok {
			out = append(out, v)
		}
	}
	return out
}

// lastExtension returns the most-recently-registered extension
// implementing T, or T's zero value when none is registered. Used
// for last-wins singletons (IDGenerator, PlannerFactory,
// BlackboardFactory) where a process-scope override beats a
// platform-scope baseline.
func lastExtension[T any](extensions []core.Extension) T {
	for i := len(extensions) - 1; i >= 0; i-- {
		if v, ok := extensions[i].(T); ok {
			return v
		}
	}
	var zero T
	return zero
}

// runActionInterceptors executes the onion chain. The first
// registered interceptor is the outermost (matches net/http
// middleware ordering). base is the inner "actually run the action"
// closure invoked once every interceptor has called next().
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

// runToolDecorators wraps tool through every decorator in
// registration order. First decorator is innermost; a decorator may
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
// vetoes (fail-fast), wrapped with the validator's Name for
// attribution.
func runAgentValidators(validators []core.AgentValidator, agent *core.Agent) error {
	for _, v := range validators {
		if err := v.ValidateAgent(agent); err != nil {
			return fmt.Errorf("validator %q: %w", v.Name(), err)
		}
	}
	return nil
}

// runGoalApprovers returns true only when every approver returns
// true (conjunction — any false vetoes). Empty approver list
// trivially approves.
func runGoalApprovers(approvers []core.GoalApprover, process core.Process, goal *core.Goal) bool {
	for _, a := range approvers {
		if !a.ApproveGoal(process, goal) {
			return false
		}
	}
	return true
}

// runToolGroupResolvers walks resolvers in order; the first non-nil
// group wins. A resolver returning (nil, nil) means "I don't know
// this role, ask the next one"; any error short-circuits.
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

// addEventListenerExtensions adds every extension implementing
// EventListener to the multicast. EventListener satisfies
// [event.Listener] directly.
func addEventListenerExtensions(multicast *event.Multicast, extensions []core.Extension) {
	for _, ext := range extensions {
		if l, ok := ext.(EventListener); ok {
			multicast.Add(l)
		}
	}
}

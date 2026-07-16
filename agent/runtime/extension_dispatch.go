package runtime

import (
	"context"
	"fmt"
	"reflect"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/event"
	"github.com/Tangerg/lynx/tools"
)

// extensionRegistry is the dedup-aware container the engine uses
// to hold registered extensions. Insertion order is preserved
// (drives onion / wrap chain ordering). Registration returns ordinary errors
// so dynamic host configuration never has to recover from a panic.
type extensionRegistry struct {
	list   []core.Extension
	byName map[string]core.Extension
}

func newExtensionRegistry() extensionRegistry {
	return extensionRegistry{byName: map[string]core.Extension{}}
}

// register adds extension to the registry. It rejects nil (including typed nil),
// empty Name, and duplicate Name without mutating the registry.
func (r *extensionRegistry) register(scope string, extension core.Extension) error {
	if extensionIsNil(extension) {
		return fmt.Errorf("runtime: nil extension in %s", scope)
	}
	name := extension.Name()
	if name == "" {
		return fmt.Errorf("runtime: extension %T returned empty Name() in %s", extension, scope)
	}
	if _, duplicate := r.byName[name]; duplicate {
		return fmt.Errorf("runtime: extension %q already registered in %s", name, scope)
	}
	r.byName[name] = extension
	r.list = append(r.list, extension)
	return nil
}

func extensionIsNil(extension core.Extension) bool {
	return valueIsNil(extension)
}

func valueIsNil(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}

// collectExtensions returns every extension that implements T, in
// registration order. Used for fan-out / chain capabilities
// (interceptor, decorator, validator, approver, resolver).
func collectExtensions[T any](extensions []core.Extension) []T {
	var matched []T
	for _, extension := range extensions {
		if capability, ok := extension.(T); ok {
			matched = append(matched, capability)
		}
	}
	return matched
}

// lastExtension returns the most-recently-registered extension
// implementing T, or T's zero value when none is registered. Used
// for last-wins singletons (IDGenerator, Blackboard prototype) where
// a process-scope override beats an engine-scope baseline. Planners
// have their own name-based dispatch in [Engine.resolvePlanner].
func lastExtension[T any](extensions []core.Extension) T {
	for index := len(extensions) - 1; index >= 0; index-- {
		if capability, ok := extensions[index].(T); ok {
			return capability
		}
	}
	var zero T
	return zero
}

// runActionChain executes the onion chain. The first
// registered interceptor is the outermost (matches net/http
// middleware ordering). base is the inner "actually run the action"
// closure invoked once every interceptor has called next().
func runActionChain(
	actionMiddleware []core.ActionMiddleware,
	ctx context.Context,
	process core.ProcessView,
	action core.Action,
	base func() (core.ActionStatus, error),
) (core.ActionStatus, error) {
	if len(actionMiddleware) == 0 {
		return base()
	}
	var run func(index int) (core.ActionStatus, error)
	run = func(index int) (core.ActionStatus, error) {
		if index >= len(actionMiddleware) {
			return base()
		}
		return actionMiddleware[index].RunAction(ctx, process, action, func() (core.ActionStatus, error) {
			return run(index + 1)
		})
	}
	return run(0)
}

// wrapTool wraps tool through every decorator in
// registration order. First decorator is innermost; a decorator may
// return its input unchanged to no-op.
func wrapTool(
	toolMiddleware []core.ToolMiddleware,
	process core.ProcessView,
	action core.Action,
	tool tools.Tool,
) tools.Tool {
	for _, middleware := range toolMiddleware {
		tool = middleware.WrapTool(process, action, tool)
	}
	return tool
}

// runAgentValidators runs every validator and collects all their errors
// (each wrapped with the validator's Name for attribution) so Deploy can
// report every problem at once rather than stopping at the first.
func runAgentValidators(validators []core.AgentValidator, agent *core.Agent) []error {
	var problems []error
	for _, validator := range validators {
		if err := validator.Validate(agent); err != nil {
			problems = append(problems, fmt.Errorf("runtime.runAgentValidators: validator %q: %w", validator.Name(), err))
		}
	}
	return problems
}

// runGoalApprovers returns true only when every approver returns
// true (conjunction — any false vetoes). Empty approver list
// trivially approves.
func runGoalApprovers(approvers []core.GoalApprover, process core.ProcessView, goal *core.Goal) bool {
	for _, approver := range approvers {
		if !approver.Approve(process, goal) {
			return false
		}
	}
	return true
}

// runToolGroupResolvers walks resolvers in order; the first resolver
// reporting ok=true wins. A resolver returning (ok=false) means "I don't know
// this role, ask the next one"; any error short-circuits.
//
// Resolved groups are rejected when their declared permissions exceed
// what the requirement grants — a sandboxed action can't pick up a
// resolver implementation that quietly upgrades the privilege set.
func runToolGroupResolvers(
	resolvers []core.ToolGroupResolver,
	ctx context.Context,
	requirement core.ToolGroupRequirement,
) (core.ToolGroup, bool, error) {
	for _, resolver := range resolvers {
		group, ok, err := resolver.Resolve(ctx, requirement)
		if err != nil {
			return nil, false, fmt.Errorf("runtime.runToolGroupResolvers: resolver %q: %w", resolver.Name(), err)
		}
		if !ok {
			if !valueIsNil(group) {
				return nil, false, fmt.Errorf("runtime.runToolGroupResolvers: resolver %q returned a group for a miss", resolver.Name())
			}
			continue
		}
		if valueIsNil(group) {
			return nil, false, fmt.Errorf("runtime.runToolGroupResolvers: resolver %q matched role %q with a nil group", resolver.Name(), requirement.Role)
		}
		info := group.Info()
		if info.Role == "" {
			return nil, false, fmt.Errorf("runtime.runToolGroupResolvers: resolver %q matched role %q with an empty group role", resolver.Name(), requirement.Role)
		}
		if info.Role != requirement.Role {
			return nil, false, fmt.Errorf("runtime.runToolGroupResolvers: resolver %q matched role %q with group role %q", resolver.Name(), requirement.Role, info.Role)
		}
		required := info.Permissions
		for _, permission := range required {
			if permission.String() == "unknown" {
				return nil, false, fmt.Errorf("runtime.runToolGroupResolvers: resolver %q returned unknown permission %d for role %q", resolver.Name(), permission, requirement.Role)
			}
		}
		if !core.AllowsPermissions(requirement.AllowedPermissions, required) {
			return nil, false, fmt.Errorf("runtime.runToolGroupResolvers: resolver %q: tool group %q requires permissions %v, allowed %v",
				resolver.Name(), info.Role, required, requirement.AllowedPermissions)
		}
		return group, true, nil
	}
	return nil, false, nil
}

// addEventListenerExtensions adds every extension implementing
// EventListener to the multicast. EventListener satisfies
// [event.Listener] directly.
func addEventListenerExtensions(multicast *event.Multicast, extensions []core.Extension) {
	for _, extension := range extensions {
		if listener, ok := extension.(EventListener); ok {
			multicast.Add(listener)
		}
	}
}

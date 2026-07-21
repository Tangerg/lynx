package runtime

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/event"
	"github.com/Tangerg/lynx/agent/internal/panicerr"
	"github.com/Tangerg/lynx/agent/planning"
	"github.com/Tangerg/lynx/tools"
)

// EventListener is the [event.Event] subscriber extension. It lives in runtime
// because event depends on core; placing this contract in core would create an
// import cycle. Valid at engine and process scope. The same listener may receive
// events from different processes concurrently and owns its synchronization and
// backpressure policy.
type EventListener interface {
	core.Extension

	OnEvent(ctx context.Context, event event.Event)
}

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
	if valueIsNil(extension) {
		return fmt.Errorf("runtime: nil extension in %s", scope)
	}
	name, err := extensionName(extension)
	if err != nil {
		return fmt.Errorf("runtime: extension in %s: %w", scope, err)
	}
	if name == "" {
		return fmt.Errorf("runtime: extension %T returned empty Name() in %s", extension, scope)
	}
	if _, duplicate := r.byName[name]; duplicate {
		return fmt.Errorf("runtime: extension %q already registered in %s", name, scope)
	}
	if !supportsEngineScope(extension) {
		return fmt.Errorf("runtime: extension %q in %s has no engine-scoped capability", name, scope)
	}
	r.byName[name] = extension
	r.list = append(r.list, extension)
	return nil
}

func supportsEngineScope(extension core.Extension) bool {
	switch extension.(type) {
	case core.ActionMiddleware,
		core.ToolMiddleware,
		core.AgentValidator,
		core.GoalApprover,
		core.ChatProvider,
		core.StopPolicy,
		core.ToolGroupResolver,
		core.IDGenerator,
		core.Blackboard,
		planning.Planner,
		EventListener:
		return true
	default:
		return false
	}
}

func validateProcessExtensionScope(extension core.Extension) error {
	var engineOnly []string
	if _, ok := extension.(core.AgentValidator); ok {
		engineOnly = append(engineOnly, "AgentValidator")
	}
	if _, ok := extension.(core.IDGenerator); ok {
		engineOnly = append(engineOnly, "IDGenerator")
	}
	if _, ok := extension.(core.Blackboard); ok {
		engineOnly = append(engineOnly, "Blackboard")
	}
	if len(engineOnly) > 0 {
		return fmt.Errorf("engine-only capabilities: %s", strings.Join(engineOnly, ", "))
	}

	switch extension.(type) {
	case core.ActionMiddleware,
		core.ToolMiddleware,
		core.GoalApprover,
		core.ChatProvider,
		core.StopPolicy,
		core.ToolGroupResolver,
		planning.Planner,
		EventListener:
		return nil
	default:
		return errors.New("no process-scoped capability")
	}
}

func extensionName(extension core.Extension) (name string, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = panicerr.New(fmt.Sprintf("extension %T Name panicked", extension), recovered)
		}
	}()
	return extension.Name(), nil
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

// runActionChain executes the process's action-middleware onion chain. The first
// registered interceptor is the outermost (matches net/http
// middleware ordering). base is the inner "actually run the action"
// closure invoked once every interceptor has called next().
func (p *Process) runActionChain(
	ctx context.Context,
	action core.Action,
	base func() (core.ActionStatus, error),
) (core.ActionStatus, error) {
	actionMiddleware := collectExtensions[core.ActionMiddleware](p.combinedExtensions())
	if len(actionMiddleware) == 0 {
		return base()
	}
	var run func(index int) (core.ActionStatus, error)
	run = func(index int) (core.ActionStatus, error) {
		if index >= len(actionMiddleware) {
			return base()
		}
		next := sync.OnceValues(func() (core.ActionStatus, error) {
			return run(index + 1)
		})
		return runActionMiddleware(ctx, actionMiddleware[index], p, action, next)
	}
	return run(0)
}

func runActionMiddleware(
	ctx context.Context,
	middleware core.ActionMiddleware,
	process core.ProcessView,
	action core.Action,
	next func() (core.ActionStatus, error),
) (status core.ActionStatus, err error) {
	name, err := extensionName(middleware)
	if err != nil {
		return core.ActionFailed, err
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			status = core.ActionFailed
			err = panicerr.New(fmt.Sprintf("action middleware %q panicked", name), recovered)
		}
	}()
	return middleware.RunAction(ctx, process, action, next)
}

// wrapTool wraps tool through every supplied decorator in
// registration order. First decorator is innermost; a decorator may
// return its input unchanged to no-op.
func (p *Process) wrapTool(
	toolMiddleware []core.ToolMiddleware,
	action core.Action,
	tool tools.Tool,
) (tools.Tool, error) {
	for _, middleware := range toolMiddleware {
		wrapped, err := wrapToolWith(middleware, p, action, tool)
		if err != nil {
			return nil, err
		}
		if valueIsNil(wrapped) {
			name, nameErr := extensionName(middleware)
			if nameErr != nil {
				return nil, nameErr
			}
			return nil, fmt.Errorf("tool middleware %q returned nil", name)
		}
		tool = wrapped
	}
	return tool, nil
}

func wrapToolWith(
	middleware core.ToolMiddleware,
	process core.ProcessView,
	action core.Action,
	tool tools.Tool,
) (wrapped tools.Tool, err error) {
	name, err := extensionName(middleware)
	if err != nil {
		return nil, err
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			err = panicerr.New(fmt.Sprintf("tool middleware %q panicked", name), recovered)
		}
	}()
	return middleware.WrapTool(process, action, tool), nil
}

// agentValidationErrors runs every engine validator and collects all errors
// (each wrapped with the validator's Name for attribution) so Deploy can
// report every problem at once rather than stopping at the first.
func (e *Engine) agentValidationErrors(agent *core.Agent) []error {
	validators := collectExtensions[core.AgentValidator](e.extensions.list)
	var problems []error
	for _, validator := range validators {
		if err := validateAgentWith(validator, agent); err != nil {
			name, nameErr := extensionName(validator)
			if nameErr != nil {
				problems = append(problems, nameErr)
				continue
			}
			problems = append(problems, fmt.Errorf("runtime.Engine.agentValidationErrors: validator %q: %w", name, err))
		}
	}
	return problems
}

func validateAgentWith(validator core.AgentValidator, agent *core.Agent) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = panicerr.New("agent validator panicked", recovered)
		}
	}()
	return validator.Validate(agent)
}

// approvesGoal returns true only when every approver returns
// true (conjunction — any false vetoes). Empty approver list
// trivially approves.
func (p *Process) approvesGoal(approvers []core.GoalApprover, goal *core.Goal) (bool, error) {
	for _, approver := range approvers {
		approved, err := approveGoalWith(approver, p, goal)
		if err != nil {
			return false, err
		}
		if !approved {
			return false, nil
		}
	}
	return true, nil
}

func approveGoalWith(approver core.GoalApprover, process core.ProcessView, goal *core.Goal) (approved bool, err error) {
	name, err := extensionName(approver)
	if err != nil {
		return false, err
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			err = panicerr.New(fmt.Sprintf("goal approver %q panicked", name), recovered)
		}
	}()
	return approver.Approve(process, goal), nil
}

// runToolGroupResolvers walks resolvers in order; the first resolver
// reporting ok=true wins. A resolver returning (ok=false) means "I don't know
// this role, ask the next one"; any error short-circuits.
//
// Resolved groups are rejected when their declared permissions exceed
// what the requirement grants — a sandboxed action can't pick up a
// resolver implementation that quietly upgrades the privilege set.
func runToolGroupResolvers(
	ctx context.Context,
	resolvers []core.ToolGroupResolver,
	requirement core.ToolGroupRequirement,
) (core.ToolGroup, bool, error) {
	if err := requirement.Validate(); err != nil {
		return nil, false, fmt.Errorf("runtime.runToolGroupResolvers: invalid requirement: %w", err)
	}
	for _, resolver := range resolvers {
		name, err := extensionName(resolver)
		if err != nil {
			return nil, false, err
		}
		group, ok, err := resolveToolGroupWith(ctx, resolver, requirement, name)
		if err != nil {
			return nil, false, fmt.Errorf("runtime.runToolGroupResolvers: resolver %q: %w", name, err)
		}
		if !ok {
			if !valueIsNil(group) {
				return nil, false, fmt.Errorf("runtime.runToolGroupResolvers: resolver %q returned a group for a miss", name)
			}
			continue
		}
		if valueIsNil(group) {
			return nil, false, fmt.Errorf("runtime.runToolGroupResolvers: resolver %q matched role %q with a nil group", name, requirement.Role)
		}
		info, err := toolGroupInfo(group, name)
		if err != nil {
			return nil, false, err
		}
		if err := info.Validate(); err != nil {
			return nil, false, fmt.Errorf("runtime.runToolGroupResolvers: resolver %q returned invalid group info: %w", name, err)
		}
		if info.Role != requirement.Role {
			return nil, false, fmt.Errorf("runtime.runToolGroupResolvers: resolver %q matched role %q with group role %q", name, requirement.Role, info.Role)
		}
		if !requirement.Allows(info.Permissions) {
			return nil, false, fmt.Errorf("runtime.runToolGroupResolvers: resolver %q: tool group %q requires permissions %v, allowed %v",
				name, info.Role, info.Permissions, requirement.AllowedPermissions)
		}
		return group, true, nil
	}
	return nil, false, nil
}

func resolveToolGroupWith(
	ctx context.Context,
	resolver core.ToolGroupResolver,
	requirement core.ToolGroupRequirement,
	name string,
) (group core.ToolGroup, ok bool, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = panicerr.New(fmt.Sprintf("tool group resolver %q panicked", name), recovered)
		}
	}()
	return resolver.Resolve(ctx, requirement)
}

func toolGroupInfo(group core.ToolGroup, resolverName string) (info core.ToolGroupInfo, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = panicerr.New(fmt.Sprintf("tool group from resolver %q Info panicked", resolverName), recovered)
		}
	}()
	return group.Info(), nil
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

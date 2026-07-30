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
// import cycle. Valid at engine and process scope. A process-scoped listener
// receives only that process's events. The same listener instance may still be
// called concurrently when registered in multiple scopes and owns its
// synchronization and backpressure policy.
type EventListener interface {
	core.Extension

	OnEvent(ctx context.Context, event event.Event)
}

// SubtreeEventListener explicitly extends a process-scoped [EventListener] to
// every descendant created from that process. Engine-scoped listeners already
// observe every process and do not need this marker.
type SubtreeEventListener interface {
	EventListener
	ObserveSubtree()
}

// ChildAdmitter is the synchronous host admission boundary for a newly built
// child process. The child already has immutable process, parent, and spawn-call
// identity, but Runtime has not published ProcessCreated or executed its first
// tick. Returning nil admits execution; returning an error rejects and removes
// the unpublished child.
//
// Unlike EventListener, this capability may block while the caller coordinates
// external admission work. Implementations must honor ctx. Process-scoped
// registrations take precedence over an engine-scoped fallback, so one child
// is admitted by exactly one authority.
type ChildAdmitter interface {
	core.Extension

	AdmitChild(ctx context.Context, child core.ProcessView) error
}

// The marker is read by type assertion when a child inherits its parent's
// listeners, so a drifted method name would quietly stop propagating instead of
// failing the build. The assertion lives here because event cannot name this
// interface without depending upward on the runtime.
var _ SubtreeEventListener = (*event.NamedSubtreeListener)(nil)

// extensionRegistry is the dedup-aware container the engine uses
// to hold registered extensions. Insertion order is preserved
// (drives onion / wrap chain ordering). Registration returns ordinary errors
// so dynamic host configuration never has to recover from a panic.
type extensionRegistry struct {
	list   []extensionEntry
	byName map[string]struct{}
}

type extensionEntry struct {
	name  string
	value core.Extension
}

type extensionCapability[T any] struct {
	name  string
	value T
}

func newExtensionRegistry() extensionRegistry {
	return extensionRegistry{byName: map[string]struct{}{}}
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
	r.byName[name] = struct{}{}
	r.list = append(r.list, extensionEntry{name: name, value: extension})
	return nil
}

func supportsEngineScope(extension core.Extension) bool {
	return supportsProcessScope(extension) || len(engineOnlyCapabilities(extension)) > 0
}

func supportsProcessScope(extension core.Extension) bool {
	switch extension.(type) {
	case core.ActionMiddleware,
		core.ToolMiddleware,
		core.GoalApprover,
		core.ChatProvider,
		core.InteractionCostProjector,
		core.StopPolicy,
		core.ToolGroupResolver,
		planning.Planner,
		EventListener,
		ChildAdmitter:
		return true
	default:
		return false
	}
}

// engineOnlyCapabilities names the capabilities extension declares that only
// make sense engine-wide. It is the one statement of that set: the scope check
// and the error that lists them both read it, so a capability added here cannot
// be silently accepted at process scope by a predicate that forgot it.
func engineOnlyCapabilities(extension core.Extension) []string {
	var declared []string
	if _, ok := extension.(core.AgentValidator); ok {
		declared = append(declared, "AgentValidator")
	}
	if _, ok := extension.(core.IDGenerator); ok {
		declared = append(declared, "IDGenerator")
	}
	if _, ok := extension.(core.Blackboard); ok {
		declared = append(declared, "Blackboard")
	}
	return declared
}

func validateProcessExtensionScope(extension core.Extension) error {
	if engineOnly := engineOnlyCapabilities(extension); len(engineOnly) > 0 {
		return fmt.Errorf("engine-only capabilities: %s", strings.Join(engineOnly, ", "))
	}
	if supportsProcessScope(extension) {
		return nil
	}
	return errors.New("no process-scoped capability")
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
func collectExtensions[T any](extensions []extensionEntry) []extensionCapability[T] {
	var matched []extensionCapability[T]
	for _, extension := range extensions {
		if capability, ok := extension.value.(T); ok {
			matched = append(matched, extensionCapability[T]{name: extension.name, value: capability})
		}
	}
	return matched
}

// firstExtension returns the first registered implementation of T. Callers
// choose the supplied extension ordering explicitly (for example,
// process-before-engine resolver order).
func firstExtension[T any](extensions []extensionEntry) (extensionCapability[T], bool) {
	for _, extension := range extensions {
		if capability, ok := extension.value.(T); ok {
			return extensionCapability[T]{name: extension.name, value: capability}, true
		}
	}
	return extensionCapability[T]{}, false
}

// lastExtension returns the most-recently-registered extension
// implementing T, or T's zero value when none is registered. Used
// for last-wins singletons (IDGenerator, Blackboard prototype) where
// a process-scope override beats an engine-scope baseline. Planners
// have their own name-based dispatch in [Engine.resolvePlanner].
func lastExtension[T any](extensions []extensionEntry) (extensionCapability[T], bool) {
	for index := len(extensions) - 1; index >= 0; index-- {
		if capability, ok := extensions[index].value.(T); ok {
			return extensionCapability[T]{name: extensions[index].name, value: capability}, true
		}
	}
	return extensionCapability[T]{}, false
}

// runActionChain executes the process's action-middleware onion chain. The first
// registered interceptor is the outermost (matches net/http
// middleware ordering). base is the inner "actually run the action"
// closure invoked once every interceptor has called next().
func (p *Process) runActionChain(
	ctx context.Context,
	action core.ActionDescriptor,
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
	middleware extensionCapability[core.ActionMiddleware],
	process core.ProcessView,
	action core.ActionDescriptor,
	next func() (core.ActionStatus, error),
) (status core.ActionStatus, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			status = core.ActionFailed
			err = panicerr.New(fmt.Sprintf("action middleware %q panicked", middleware.name), recovered)
		}
	}()
	return middleware.value.RunAction(ctx, process, action, next)
}

// wrapTool wraps tool through every supplied decorator in
// registration order. First decorator is innermost; a decorator may
// return its input unchanged to no-op.
func (p *Process) wrapTool(
	toolMiddleware []extensionCapability[core.ToolMiddleware],
	action core.ActionDescriptor,
	tool tools.Tool,
) (tools.Tool, error) {
	for _, middleware := range toolMiddleware {
		wrapped, err := wrapToolWith(middleware, p, action, tool)
		if err != nil {
			return nil, err
		}
		if valueIsNil(wrapped) {
			return nil, fmt.Errorf("tool middleware %q returned nil", middleware.name)
		}
		tool = wrapped
	}
	return tool, nil
}

func wrapToolWith(
	middleware extensionCapability[core.ToolMiddleware],
	process core.ProcessView,
	action core.ActionDescriptor,
	tool tools.Tool,
) (wrapped tools.Tool, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = panicerr.New(fmt.Sprintf("tool middleware %q panicked", middleware.name), recovered)
		}
	}()
	return middleware.value.WrapTool(process, action, tool), nil
}

// agentValidationErrors runs every engine validator and collects all errors
// (each wrapped with the validator's Name for attribution) so Deploy can
// report every problem at once rather than stopping at the first.
func (e *Engine) agentValidationErrors(agent *core.Agent) []error {
	validators := collectExtensions[core.AgentValidator](e.extensions.list)
	var problems []error
	for _, validator := range validators {
		if err := validateAgentWith(validator.value, agent); err != nil {
			problems = append(problems, fmt.Errorf("runtime.Engine.agentValidationErrors: validator %q: %w", validator.name, err))
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
func (p *Process) approvesGoal(approvers []extensionCapability[core.GoalApprover], goal *core.Goal) (bool, error) {
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

func approveGoalWith(approver extensionCapability[core.GoalApprover], process core.ProcessView, goal *core.Goal) (approved bool, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = panicerr.New(fmt.Sprintf("goal approver %q panicked", approver.name), recovered)
		}
	}()
	return approver.value.Approve(process, goal.Descriptor()), nil
}

// runToolGroupResolvers walks resolvers in order; the first resolver
// reporting ok=true wins. A resolver returning (ok=false) means "I don't know
// this role, ask the next one"; any error short-circuits.
func runToolGroupResolvers(
	ctx context.Context,
	resolvers []extensionCapability[core.ToolGroupResolver],
	role string,
) (core.ToolGroup, bool, error) {
	for _, resolver := range resolvers {
		group, ok, err := resolveToolGroupWith(ctx, resolver.value, role, resolver.name)
		if err != nil {
			return nil, false, fmt.Errorf("runtime.runToolGroupResolvers: resolver %q: %w", resolver.name, err)
		}
		if !ok {
			if !valueIsNil(group) {
				return nil, false, fmt.Errorf("runtime.runToolGroupResolvers: resolver %q returned a group for a miss", resolver.name)
			}
			continue
		}
		if valueIsNil(group) {
			return nil, false, fmt.Errorf("runtime.runToolGroupResolvers: resolver %q matched role %q with a nil group", resolver.name, role)
		}
		return group, true, nil
	}
	return nil, false, nil
}

func resolveToolGroupWith(
	ctx context.Context,
	resolver core.ToolGroupResolver,
	role string,
	name string,
) (group core.ToolGroup, ok bool, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = panicerr.New(fmt.Sprintf("tool group resolver %q panicked", name), recovered)
		}
	}()
	return resolver.Resolve(ctx, role)
}

// addEventListenerExtensions adds every extension implementing
// EventListener to the multicast. EventListener satisfies
// [event.Listener] directly.
func addEventListenerExtensions(multicast *event.Multicast, extensions []extensionEntry) {
	for _, extension := range extensions {
		if listener, ok := extension.value.(EventListener); ok {
			multicast.Add(listener)
		}
	}
}

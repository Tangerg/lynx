// Package reflect adapts Go structs into core.Agent definitions via
// reflection. It's the convenience layer for users who prefer
// "annotation style" — methods on a struct become actions and conditions,
// marker methods declare goals.
//
// Reflection happens once at registration time; method dispatch at runtime
// uses the cached reflect.Method.Func, which is one indirection slower than
// a direct call but well below LLM-call noise floor.
package reflect

import (
	"context"
	"errors"
	"fmt"
	stdreflect "reflect"
	"strings"
	"unicode"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/dsl"
)

// Register inspects an instance's exported methods and produces an Agent.
// The struct's "agent" tag (on a private _ field) provides the agent name
// and description; method names follow conventions (see classifyMethod).
//
// Registration fails (returns nil, err) if the supplied value isn't a
// pointer-to-struct or any method has an unsupported signature.
func Register(instance any) (*core.Agent, error) {
	if instance == nil {
		return nil, errors.New("reflect.Register: instance is nil")
	}

	value := stdreflect.ValueOf(instance)
	declaredType := value.Type()
	if declaredType.Kind() != stdreflect.Pointer {
		return nil, fmt.Errorf(
			"reflect.Register: expected pointer to struct, got %s — wrap your value with & or change the receiver",
			declaredType.Kind(),
		)
	}

	elemType := declaredType.Elem()
	if elemType.Kind() != stdreflect.Struct {
		return nil, fmt.Errorf("reflect.Register: expected pointer to struct, got pointer to %s", elemType.Kind())
	}

	meta := parseAgentTag(elemType)
	builder := newBuilderFromMeta(meta)

	if err := registerMethods(builder, declaredType, value); err != nil {
		return nil, err
	}
	return builder.Build(), nil
}

// newBuilderFromMeta seeds the DSL builder with values parsed from the
// "agent" tag. Empty fields skip their setters so defaults remain in
// effect.
func newBuilderFromMeta(meta agentMeta) *dsl.Builder {
	b := dsl.New(meta.name)

	if meta.description != "" {
		b = b.Description(meta.description)
	}
	if meta.provider != "" {
		b = b.Provider(meta.provider)
	}
	if meta.version != "" {
		b = b.Version(meta.version)
	}
	return b
}

// registerMethods walks every method on declaredType and, based on its
// shape, attaches it to the builder as an action / condition / goal.
func registerMethods(builder *dsl.Builder, declaredType stdreflect.Type, receiver stdreflect.Value) error {
	for i := 0; i < declaredType.NumMethod(); i++ {
		method := declaredType.Method(i)

		switch classifyMethod(method) {
		case methodAction:
			action, err := wrapAsAction(method, receiver)
			if err != nil {
				return fmt.Errorf("reflect.Register: action %q: %w", method.Name, err)
			}
			builder.Action(action)

		case methodCondition:
			builder.Condition(wrapAsCondition(method, receiver))

		case methodAchievesGoal:
			goal, err := invokeGoalFactory(method, receiver)
			if err != nil {
				return fmt.Errorf("reflect.Register: goal factory %q: %w", method.Name, err)
			}
			if goal != nil {
				builder.Goal(goal)
			}

		case methodIgnore:
			// Not part of the agent surface — skip.
		}
	}
	return nil
}

// agentMeta is the parsed form of the `agent:"..."` struct tag.
type agentMeta struct {
	name        string
	description string
	provider    string
	version     string
}

// parseAgentTag scans struct fields for the `agent` tag — by convention
// it lives on a private _ field at the top of the struct. The tag uses
// comma-separated key=value pairs; the first un-keyed value (if any) is
// taken as the agent name.
func parseAgentTag(elemType stdreflect.Type) agentMeta {
	meta := agentMeta{name: elemType.Name()}

	for i := 0; i < elemType.NumField(); i++ {
		tag := elemType.Field(i).Tag.Get("agent")
		if tag == "" {
			continue
		}
		applyAgentTag(tag, &meta)
	}
	return meta
}

// applyAgentTag parses one tag value and merges it into meta. Unknown keys
// are ignored.
func applyAgentTag(tag string, meta *agentMeta) {
	for _, part := range strings.Split(tag, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		key, value, hasKey := strings.Cut(part, "=")
		if !hasKey {
			if meta.name == "" {
				meta.name = part
			}
			continue
		}

		switch key {
		case "name":
			meta.name = value
		case "description":
			meta.description = value
		case "provider":
			meta.provider = value
		case "version":
			meta.version = value
		}
	}
}

// methodKind classifies a struct method by its signature.
type methodKind int

const (
	methodIgnore methodKind = iota
	methodAction
	methodCondition
	methodAchievesGoal
)

// classifyMethod inspects (signature, name) to decide what role a method
// plays. Conventions:
//
//	Action:        func(ctx context.Context, in InType) (OutType, error)
//	               func(ctx context.Context, pc *core.ProcessContext, in InType) (OutType, error)
//	Condition:     func(ctx context.Context, oc *core.OperationContext) core.Determination
//	AchievesGoal:  method name starts with "Achieves", returns *core.Goal
func classifyMethod(m stdreflect.Method) methodKind {
	if !m.IsExported() {
		return methodIgnore
	}

	switch {
	case isAchievesGoalSignature(m):
		return methodAchievesGoal
	case isConditionSignature(m.Type):
		return methodCondition
	case isActionSignature(m.Type):
		return methodAction
	default:
		return methodIgnore
	}
}

func isAchievesGoalSignature(m stdreflect.Method) bool {
	if !strings.HasPrefix(m.Name, "Achieves") {
		return false
	}

	mt := m.Type
	if mt.NumIn() != 1 || mt.NumOut() != 1 {
		return false
	}

	out := mt.Out(0)
	return out.Kind() == stdreflect.Pointer && out.Elem().Name() == "Goal"
}

func isConditionSignature(mt stdreflect.Type) bool {
	if mt.NumIn() != 3 || mt.NumOut() != 1 {
		return false
	}
	if mt.In(1) != typeOfContext {
		return false
	}
	if !isPointerToNamedType(mt.In(2), "OperationContext") {
		return false
	}
	return mt.Out(0).Name() == "Determination"
}

func isActionSignature(mt stdreflect.Type) bool {
	if mt.NumIn() < 2 || mt.NumOut() != 2 {
		return false
	}
	if mt.In(1) != typeOfContext {
		return false
	}
	return implementsError(mt.Out(1))
}

func isPointerToNamedType(t stdreflect.Type, name string) bool {
	return t.Kind() == stdreflect.Pointer && t.Elem().Name() == name
}

var (
	typeOfContext = stdreflect.TypeOf((*context.Context)(nil)).Elem()
	typeOfError   = stdreflect.TypeOf((*error)(nil)).Elem()
)

func implementsError(t stdreflect.Type) bool {
	return t.Implements(typeOfError)
}

// wrapAsAction builds a core.Action from a struct method. The method is
// invoked through reflection, but only the (input, output) types are
// inspected; the rest of the framework — planning, execution, retries —
// runs on the cached metadata.
func wrapAsAction(method stdreflect.Method, receiver stdreflect.Value) (core.Action, error) {
	inputIndex, hasProcessContext, err := classifyActionParams(method.Type)
	if err != nil {
		return nil, err
	}

	inType := method.Type.In(inputIndex)
	outType := method.Type.Out(0)

	meta := core.ActionMetadata{
		Name:    snakeCase(method.Name),
		Inputs:  []core.IoBinding{{Name: core.DefaultBindingName, Type: core.TypeFullName(inType)}},
		Outputs: []core.IoBinding{{Name: core.DefaultBindingName, Type: core.TypeFullName(outType)}},
		QoS:     core.DefaultActionQos(),
	}
	meta.Preconditions, meta.Effects = computePreconditionsAndEffects(meta)

	return &reflectedAction{
		metadata: meta,
		invoke:   buildReflectInvoker(method, receiver, inputIndex, hasProcessContext),
	}, nil
}

// classifyActionParams validates the action method's parameter shape and
// returns the position of the typed input and whether a *ProcessContext
// parameter is present (which the dispatcher must thread through).
func classifyActionParams(mt stdreflect.Type) (inputIndex int, hasProcessContext bool, err error) {
	switch mt.NumIn() {
	case 3:
		// (receiver, ctx, In)
		return 2, false, nil
	case 4:
		if !isPointerToNamedType(mt.In(2), "ProcessContext") {
			return 0, false, fmt.Errorf(
				"unexpected parameter at position 2: got %s, want *core.ProcessContext",
				mt.In(2),
			)
		}
		return 3, true, nil
	default:
		return 0, false, fmt.Errorf(
			"unsupported parameter count: got %d, want 3 or 4 (including the receiver)",
			mt.NumIn(),
		)
	}
}

// buildReflectInvoker captures the method+receiver and returns a closure
// that performs the reflective call. The closure is the only thing the hot
// path executes per tick.
func buildReflectInvoker(
	method stdreflect.Method,
	receiver stdreflect.Value,
	inputIndex int,
	hasProcessContext bool,
) func(ctx context.Context, pc *core.ProcessContext, in any) (any, error) {
	return func(ctx context.Context, pc *core.ProcessContext, in any) (any, error) {
		args := make([]stdreflect.Value, method.Type.NumIn())
		args[0] = receiver
		args[1] = stdreflect.ValueOf(ctx)

		if hasProcessContext {
			args[2] = stdreflect.ValueOf(pc)
			args[3] = stdreflect.ValueOf(in)
		} else {
			args[inputIndex] = stdreflect.ValueOf(in)
		}

		results := method.Func.Call(args)

		var err error
		if !results[1].IsNil() {
			err = results[1].Interface().(error)
		}
		return results[0].Interface(), err
	}
}

// reflectedAction is the runtime adapter for a reflectively-discovered
// method. It mirrors what TypedAction does for the generic constructor.
type reflectedAction struct {
	metadata core.ActionMetadata
	invoke   func(ctx context.Context, pc *core.ProcessContext, in any) (any, error)
}

func (a *reflectedAction) Metadata() core.ActionMetadata { return a.metadata }

func (a *reflectedAction) Execute(ctx context.Context, pc *core.ProcessContext) core.ActionStatus {
	input, ok := loadReflectInput(pc.Blackboard, a.metadata.Inputs)
	if !ok {
		pc.RecordPanic(fmt.Errorf(
			"action %q: required input not on blackboard (binding=%s)",
			a.metadata.Name, a.metadata.Inputs[0],
		))
		return core.ActionFailed
	}

	output, err := a.invoke(ctx, pc, input)
	if err != nil {
		pc.RecordPanic(err)
		return core.ActionFailed
	}

	a.writeOutput(pc.Blackboard, output)
	return core.ActionSucceeded
}

// writeOutput stores the produced value on the blackboard. The default
// binding goes through Bind() so the dual-binding behavior (default name +
// type-derived name) kicks in.
func (a *reflectedAction) writeOutput(bb core.Blackboard, output any) {
	if len(a.metadata.Outputs) == 0 {
		bb.Bind(output)
		return
	}

	binding := a.metadata.Outputs[0]
	if binding.IsDefault() {
		bb.Bind(output)
		return
	}
	bb.Set(binding.Name, output)
}

// loadReflectInput uses the stored binding to pull the input from the
// blackboard. We don't have a generic In to assert against here, so we
// rely on the binding's typeName to filter.
func loadReflectInput(bb core.Blackboard, inputs []core.IoBinding) (any, bool) {
	if len(inputs) == 0 {
		return nil, true
	}

	binding := inputs[0]
	return bb.GetValue(binding.Name, binding.Type)
}

// computePreconditionsAndEffects mirrors the typed-action helper. We can't
// import the unexported core helper, so we duplicate the rule. The two
// implementations stay in lock-step — drift is a bug.
func computePreconditionsAndEffects(meta core.ActionMetadata) (core.EffectSpec, core.EffectSpec) {
	pre := core.EffectSpec{}
	eff := core.EffectSpec{}

	for _, in := range meta.Inputs {
		pre[in.String()] = core.True
	}
	for _, out := range meta.Outputs {
		eff[out.String()] = core.True
	}

	if !meta.CanRerun {
		pre[core.HasRunKey(meta.Name)] = core.False
	}
	eff[core.HasRunKey(meta.Name)] = core.True

	return pre, eff
}

// wrapAsCondition adapts a (ctx, *OperationContext) Determination method.
func wrapAsCondition(method stdreflect.Method, receiver stdreflect.Value) core.Condition {
	name := snakeCase(method.Name)

	return core.NewCondition(name, func(ctx context.Context, oc *core.OperationContext) core.Determination {
		args := []stdreflect.Value{receiver, stdreflect.ValueOf(ctx), stdreflect.ValueOf(oc)}
		results := method.Func.Call(args)
		return results[0].Interface().(core.Determination)
	})
}

// invokeGoalFactory calls a marker method to obtain a *core.Goal.
func invokeGoalFactory(method stdreflect.Method, receiver stdreflect.Value) (*core.Goal, error) {
	results := method.Func.Call([]stdreflect.Value{receiver})
	if len(results) != 1 {
		return nil, fmt.Errorf("expected 1 return value, got %d", len(results))
	}

	goal, ok := results[0].Interface().(*core.Goal)
	if !ok {
		return nil, fmt.Errorf("expected *core.Goal, got %T", results[0].Interface())
	}
	return goal, nil
}

// snakeCase converts CamelCase identifiers to snake_case. Used to derive
// action names from method names — "ClassifyIntent" → "classify_intent".
func snakeCase(s string) string {
	if s == "" {
		return s
	}

	var b strings.Builder
	b.Grow(len(s) + 4)
	for i, r := range s {
		if i > 0 && unicode.IsUpper(r) {
			b.WriteByte('_')
		}
		b.WriteRune(unicode.ToLower(r))
	}
	return b.String()
}

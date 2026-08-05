package agent2

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"
)

type planningSpikeInput struct {
	Goal string `json:"goal"`
}

type planningSpikeOutput struct {
	Built bool `json:"built"`
}

type planningSpikeWorld struct {
	Material bool `json:"material"`
	Built    bool `json:"built"`
}

type planningSpikeState struct {
	Phase  string             `json:"phase"`
	Goal   string             `json:"goal"`
	World  planningSpikeWorld `json:"world"`
	Plan   []string           `json:"plan,omitempty"`
	Action int                `json:"action,omitempty"`
}

type planningSpikeEffect struct {
	Version uint16 `json:"version"`
	Kind    string `json:"kind"`
	Action  string `json:"action,omitempty"`
}

type planningSpikeSignal struct {
	Version uint16             `json:"version"`
	Kind    string             `json:"kind"`
	World   planningSpikeWorld `json:"world"`
}

type planningSpikeAction struct {
	name       string
	applicable func(planningSpikeWorld) bool
	apply      func(planningSpikeWorld) planningSpikeWorld
}

type planningSpikeDefinition struct {
	descriptor Descriptor
	actions    []planningSpikeAction
}

func newPlanningSpikeDefinition(t *testing.T) *planningSpikeDefinition {
	t.Helper()
	inputSchema, err := SchemaFor[planningSpikeInput]()
	if err != nil {
		t.Fatal(err)
	}
	outputSchema, err := SchemaFor[planningSpikeOutput]()
	if err != nil {
		t.Fatal(err)
	}
	descriptor, err := NewDescriptor(DescriptorConfig{
		Name:         "planning.spike",
		Description:  "Validates the candidate Planning execution contracts.",
		Version:      "0.1.0",
		InputSchema:  inputSchema,
		OutputSchema: outputSchema,
	})
	if err != nil {
		t.Fatal(err)
	}
	return &planningSpikeDefinition{
		descriptor: descriptor,
		actions: []planningSpikeAction{
			{
				name:       "collect",
				applicable: func(world planningSpikeWorld) bool { return !world.Material },
				apply: func(world planningSpikeWorld) planningSpikeWorld {
					world.Material = true
					return world
				},
			},
			{
				name:       "build",
				applicable: func(world planningSpikeWorld) bool { return world.Material && !world.Built },
				apply: func(world planningSpikeWorld) planningSpikeWorld {
					world.Built = true
					return world
				},
			},
		},
	}
}

func (definition *planningSpikeDefinition) Descriptor() Descriptor { return definition.descriptor }

func (definition *planningSpikeDefinition) Start(input Input) (Execution, error) {
	if err := definition.descriptor.ValidateInput(input); err != nil {
		return nil, err
	}
	value, err := DecodeInput[planningSpikeInput](input)
	if err != nil {
		return nil, err
	}
	if value.Goal != "built" {
		return nil, errors.New("unsupported goal")
	}
	return &planningSpikeExecution{
		actions: definition.actions,
		state:   planningSpikeState{Phase: "observe", Goal: value.Goal},
	}, nil
}

func (definition *planningSpikeDefinition) Restore(state ExecutionState) (Execution, error) {
	if state.Kind() != definition.descriptor.Name() || state.SchemaVersion() != 1 {
		return nil, errors.New("unsupported planning state")
	}
	value, err := decodeJSON[planningSpikeState](state.Payload())
	if err != nil {
		return nil, err
	}
	if value.Goal != "built" || !validPlanningSpikePhase(value.Phase) || value.Action < 0 || value.Action > len(value.Plan) {
		return nil, errors.New("invalid planning state")
	}
	return &planningSpikeExecution{actions: definition.actions, state: value}, nil
}

func validPlanningSpikePhase(phase string) bool {
	switch phase {
	case "observe", "plan", "act", "reobserve":
		return true
	default:
		return false
	}
}

type planningSpikeExecution struct {
	actions []planningSpikeAction
	state   planningSpikeState
}

func (execution *planningSpikeExecution) Step(ctx context.Context, signals []Signal) (Transition, error) {
	if err := ctx.Err(); err != nil {
		return Transition{}, err
	}
	switch execution.state.Phase {
	case "observe":
		if len(signals) != 0 {
			return Transition{}, errors.New("observe phase does not accept signals")
		}
		execution.state.Phase = "plan"
		return planningSpikeContinue(0, planningSpikeEffect{Version: 1, Kind: "observe"})

	case "plan":
		world, err := planningSpikeWorldFromSignals(signals)
		if err != nil {
			return Transition{}, err
		}
		execution.state.World = world
		if world.Built {
			return planningSpikeComplete(uint32(len(signals)), world)
		}
		plan, err := execution.plan(world)
		if err != nil {
			return Transition{}, err
		}
		execution.state.Plan = plan
		execution.state.Action = 0
		execution.state.Phase = "act"
		return planningSpikeContinue(uint32(len(signals)), planningSpikeEffect{Version: 1, Kind: "action", Action: plan[0]})

	case "act":
		world, err := planningSpikeWorldFromSignals(signals)
		if err != nil {
			return Transition{}, err
		}
		execution.state.World = world
		execution.state.Action++
		if execution.state.Action < len(execution.state.Plan) {
			return planningSpikeContinue(uint32(len(signals)), planningSpikeEffect{Version: 1, Kind: "action", Action: execution.state.Plan[execution.state.Action]})
		}
		execution.state.Phase = "reobserve"
		return planningSpikeContinue(uint32(len(signals)), planningSpikeEffect{Version: 1, Kind: "observe"})

	case "reobserve":
		world, err := planningSpikeWorldFromSignals(signals)
		if err != nil {
			return Transition{}, err
		}
		execution.state.World = world
		if !world.Built {
			execution.state.Phase = "plan"
			execution.state.Plan = nil
			execution.state.Action = 0
			return planningSpikeContinue(uint32(len(signals)), planningSpikeEffect{Version: 1, Kind: "observe"})
		}
		return planningSpikeComplete(uint32(len(signals)), world)

	default:
		return Transition{}, errors.New("invalid planning phase")
	}
}

func (execution *planningSpikeExecution) plan(world planningSpikeWorld) ([]string, error) {
	plan := make([]string, 0, len(execution.actions))
	projected := world
	for _, action := range execution.actions {
		if !action.applicable(projected) {
			continue
		}
		plan = append(plan, action.name)
		projected = action.apply(projected)
		if projected.Built {
			return plan, nil
		}
	}
	return nil, errors.New("goal is unreachable")
}

func planningSpikeWorldFromSignals(signals []Signal) (planningSpikeWorld, error) {
	if len(signals) != 1 {
		return planningSpikeWorld{}, errors.New("planning phase requires one settlement signal")
	}
	value, err := decodeJSON[planningSpikeSignal](signals[0].Payload())
	if err != nil {
		return planningSpikeWorld{}, err
	}
	if value.Version != 1 || value.Kind != "world" {
		return planningSpikeWorld{}, errors.New("invalid planning settlement")
	}
	return value.World, nil
}

func planningSpikeContinue(consumed uint32, value planningSpikeEffect) (Transition, error) {
	payload, err := json.Marshal(value)
	if err != nil {
		return Transition{}, err
	}
	effect, err := NewDispatcherEffect(payload)
	if err != nil {
		return Transition{}, err
	}
	return Continue(consumed, effect)
}

func planningSpikeComplete(consumed uint32, world planningSpikeWorld) (Transition, error) {
	output, err := EncodeOutput(planningSpikeOutput{Built: world.Built})
	if err != nil {
		return Transition{}, err
	}
	return Complete(consumed, output)
}

func (execution *planningSpikeExecution) Snapshot() (ExecutionState, error) {
	payload, err := json.Marshal(execution.state)
	if err != nil {
		return ExecutionState{}, err
	}
	return NewExecutionState("planning.spike", 1, payload)
}

type planningSpikeDispatcher struct {
	world   planningSpikeWorld
	actions map[string]planningSpikeAction
}

func newPlanningSpikeDispatcher(definition *planningSpikeDefinition) *planningSpikeDispatcher {
	actions := make(map[string]planningSpikeAction, len(definition.actions))
	for _, action := range definition.actions {
		actions[action.name] = action
	}
	return &planningSpikeDispatcher{actions: actions}
}

func (dispatcher *planningSpikeDispatcher) Dispatch(effectID EffectID, effect Effect) (Settlement, error) {
	value, err := decodeJSON[planningSpikeEffect](effect.Payload())
	if err != nil || value.Version != 1 {
		return Settlement{}, errors.New("invalid planning Effect")
	}
	switch value.Kind {
	case "observe":
	case "action":
		action, ok := dispatcher.actions[value.Action]
		if !ok || !action.applicable(dispatcher.world) {
			return Settlement{}, fmt.Errorf("action %q is not applicable", value.Action)
		}
		dispatcher.world = action.apply(dispatcher.world)
	default:
		return Settlement{}, fmt.Errorf("unknown planning Effect %q", value.Kind)
	}
	payload, err := json.Marshal(planningSpikeSignal{Version: 1, Kind: "world", World: dispatcher.world})
	if err != nil {
		return Settlement{}, err
	}
	return NewSettlement(effectID, SettlementStatusSucceeded, payload)
}

func TestPlanningSpikeValidatesTwoActionObservePlanActReobserve(t *testing.T) {
	definition := newPlanningSpikeDefinition(t)
	typed, err := NewTyped[planningSpikeInput, planningSpikeOutput](definition)
	if err != nil {
		t.Fatal(err)
	}
	execution, err := typed.Start(planningSpikeInput{Goal: "built"})
	if err != nil {
		t.Fatal(err)
	}
	dispatcher := newPlanningSpikeDispatcher(definition)

	observe := mustStep(t, execution, nil, TransitionKindContinue)
	observation := dispatchPlanningSpike(t, dispatcher, 1, observe.Effects()[0])
	stateBeforePlan, err := execution.Snapshot()
	if err != nil {
		t.Fatal(err)
	}

	firstPlanExecution, err := definition.Restore(stateBeforePlan)
	if err != nil {
		t.Fatal(err)
	}
	secondPlanExecution, err := definition.Restore(stateBeforePlan)
	if err != nil {
		t.Fatal(err)
	}
	firstAction := mustStep(t, firstPlanExecution, []Signal{observation}, TransitionKindContinue)
	secondAction := mustStep(t, secondPlanExecution, []Signal{observation}, TransitionKindContinue)
	assertEquivalentCandidateStep(t, firstPlanExecution, firstAction, secondPlanExecution, secondAction)
	execution = firstPlanExecution

	collected := dispatchPlanningSpike(t, dispatcher, 2, firstAction.Effects()[0])
	build := mustStep(t, execution, []Signal{collected}, TransitionKindContinue)
	built := dispatchPlanningSpike(t, dispatcher, 3, build.Effects()[0])
	reobserve := mustStep(t, execution, []Signal{built}, TransitionKindContinue)
	reobserved := dispatchPlanningSpike(t, dispatcher, 4, reobserve.Effects()[0])
	completed := mustStep(t, execution, []Signal{reobserved}, TransitionKindComplete)
	output, ok := completed.Output()
	if !ok {
		t.Fatal("Planning spike completed without Output")
	}
	decoded, err := typed.DecodeOutput(output)
	if err != nil {
		t.Fatal(err)
	}
	if !decoded.Built {
		t.Fatalf("Planning Output = %+v", decoded)
	}
	state, err := execution.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if state.Kind() != "planning.spike" || !json.Valid(state.Payload()) {
		t.Fatalf("Planning state envelope = kind %q payload %s", state.Kind(), state.Payload())
	}
}

func dispatchPlanningSpike(t *testing.T, dispatcher *planningSpikeDispatcher, step uint64, effect Effect) Signal {
	t.Helper()
	settlement, err := dispatcher.Dispatch(mustEffectID(fmt.Sprintf("process:planning:step:%d:effect:0", step)), effect)
	if err != nil {
		t.Fatal(err)
	}
	return mustSignal(t, fmt.Sprintf("signal:planning:%d", step), WaitID{}, time.Unix(int64(step), 0), settlement.Payload())
}

func assertEquivalentCandidateStep(t *testing.T, firstExecution Execution, first Transition, secondExecution Execution, second Transition) {
	t.Helper()
	firstTransition, err := json.Marshal(first)
	if err != nil {
		t.Fatal(err)
	}
	secondTransition, err := json.Marshal(second)
	if err != nil {
		t.Fatal(err)
	}
	firstState, err := firstExecution.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	secondState, err := secondExecution.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if string(firstTransition) != string(secondTransition) || string(firstState.Payload()) != string(secondState.Payload()) {
		t.Fatalf("same state and Signals produced different candidates:\n%s\n%s\n%s\n%s", firstTransition, secondTransition, firstState.Payload(), secondState.Payload())
	}
}

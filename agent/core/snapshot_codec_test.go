package core_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/Tangerg/lynx/agent/core"
)

type snapshotInput struct {
	Text string
}

type snapshotOutput struct {
	Count int
}

type typedNilSnapshotAction struct{}

func (*typedNilSnapshotAction) Metadata() core.ActionMetadata {
	panic("typed nil action metadata must not be called")
}

func (*typedNilSnapshotAction) Execute(context.Context, *core.ProcessContext) (core.ActionStatus, error) {
	return core.ActionSucceeded, nil
}

type panickingSnapshotAction struct{ cause error }

func (a panickingSnapshotAction) Metadata() core.ActionMetadata { panic(a.cause) }
func (panickingSnapshotAction) Execute(context.Context, *core.ProcessContext) (core.ActionStatus, error) {
	return core.ActionSucceeded, nil
}

func snapshotAgent() *core.Agent {
	action := core.NewAction("count", func(_ context.Context, _ *core.ProcessContext, input snapshotInput) (snapshotOutput, error) {
		return snapshotOutput{Count: len(input.Text)}, nil
	}, core.ActionConfig{})
	return core.NewAgent(core.AgentConfig{
		Name:             "snapshot",
		Actions:          []core.Action{action},
		Goals:            []*core.Goal{core.NewOutputGoal[snapshotOutput](core.GoalConfig{})},
		SnapshotBindings: []core.Binding{core.NewBinding[*snapshotInput]("pointer_input")},
	})
}

func TestAgentBlackboardCodecRoundTrip(t *testing.T) {
	agent := snapshotAgent()
	var named core.Bindings
	named.Set("input", snapshotInput{Text: "lynx"})
	named.Set("count", 4)
	objects := []any{snapshotOutput{Count: 4}, "done", &snapshotInput{Text: "pointer"}}

	taggedNamed, taggedObjects, err := agent.EncodeBlackboard(named, objects)
	if err != nil {
		t.Fatalf("EncodeBlackboard: %v", err)
	}
	decodedNamed, decodedObjects, err := agent.DecodeBlackboard(taggedNamed, taggedObjects)
	if err != nil {
		t.Fatalf("DecodeBlackboard: %v", err)
	}
	for name, want := range named.All() {
		got, ok := decodedNamed.Get(name)
		if !ok || !reflect.DeepEqual(got, want) {
			t.Fatalf("binding %q = %#v, want %#v", name, got, want)
		}
	}
	if !reflect.DeepEqual(decodedObjects, objects) {
		t.Fatalf("objects = %#v, want %#v", decodedObjects, objects)
	}
}

func TestAgentBlackboardCodecRejectsNilReceiver(t *testing.T) {
	var agent *core.Agent
	if _, _, err := agent.EncodeBlackboard(core.Bindings{}, nil); err == nil {
		t.Fatal("EncodeBlackboard accepted nil agent")
	}
	if _, _, err := agent.DecodeBlackboard(nil, nil); err == nil {
		t.Fatal("DecodeBlackboard accepted nil agent")
	}
}

func TestSnapshotCodecContainsInvalidActionMetadata(t *testing.T) {
	var typedNil *typedNilSnapshotAction
	for _, action := range []core.Action{
		typedNil,
		panickingSnapshotAction{cause: errors.New("metadata panic")},
	} {
		definition := core.NewAgent(core.AgentConfig{
			Name: "invalid-snapshot-action", Actions: []core.Action{action},
			Goals: []*core.Goal{core.NewGoal(core.GoalConfig{Name: "goal"})},
		})
		codec := definition.SnapshotCodec()
		if err := codec.ValidateDeclaredType(snapshotInput{}); err == nil {
			t.Fatal("invalid action metadata unexpectedly declared snapshotInput")
		}
	}
}

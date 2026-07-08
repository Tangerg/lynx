package turn

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Tangerg/lynx/agent/event"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/hooks"
)

func TestTurnLifecycle_SubagentHooks(t *testing.T) {
	rec := &recordHookCommands{}
	bound := hooks.NewBound([]hooks.Hook{
		{Event: hooks.SubagentStart, Command: "record", Source: "test"},
		{Event: hooks.SubagentStop, Command: "record", Source: "test"},
	}, hooks.NewRunner(rec, nil))
	lifecycle := &turnLifecycle{
		rootID:    "root",
		sessionID: "sess",
		cwd:       "/work",
		hooks:     bound,
	}
	listener := lifecycle.listener("turn")

	listener.OnEvent(context.Background(), event.ProcessCreated{BaseEvent: event.NewBaseEvent("root")})
	listener.OnEvent(context.Background(), event.ProcessCreated{BaseEvent: event.NewBaseEvent("child")})
	listener.OnEvent(context.Background(), event.ProcessCompleted{BaseEvent: event.NewBaseEvent("child")})

	if len(rec.inputs) != 2 {
		t.Fatalf("hook inputs = %d, want 2: %#v", len(rec.inputs), rec.inputs)
	}
	if rec.inputs[0].Event != hooks.SubagentStart || rec.inputs[0].Subagent.ProcessID != "child" {
		t.Fatalf("start input = %+v", rec.inputs[0])
	}
	if rec.inputs[1].Event != hooks.SubagentStop || rec.inputs[1].Reason != "process_completed" {
		t.Fatalf("stop input = %+v", rec.inputs[1])
	}
}

type recordHookCommands struct {
	inputs []hooks.Input
}

func (r *recordHookCommands) RunHookCommand(_ context.Context, req hooks.CommandRequest) hooks.CommandResult {
	var in hooks.Input
	_ = json.Unmarshal(req.Stdin, &in)
	r.inputs = append(r.inputs, in)
	return hooks.CommandResult{}
}

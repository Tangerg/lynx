package agentexec

import (
	"context"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/chatclient"
)

type startFailureInput struct {
	Value int
}

type startFailureOutput struct {
	Value int
}

type namedStartExtension string

func (e namedStartExtension) Name() string { return string(e) }

func TestEngineStartTurnReturnsProcessCreationErrorsSynchronously(t *testing.T) {
	tests := []struct {
		name      string
		arrange   func(*Engine)
		request   func() TurnRequest
		wantError string
	}{
		{
			name: "duplicate process extension name",
			request: func() TurnRequest {
				return TurnRequest{
					Message:       "hello",
					Observer:      &recordingObserver{},
					EventListener: namedStartExtension("tool-observer"),
				}
			},
			wantError: `duplicate name "tool-observer"`,
		},
		{
			name: "agent requests unregistered planner",
			arrange: func(engine *Engine) {
				engine.agent = startFailureAgent("unknown-planner", "missing-planner")
			},
			request:   func() TurnRequest { return TurnRequest{Message: "hello"} },
			wantError: `planner "missing-planner" which is not registered`,
		},
		{
			name: "agent definition cannot deploy",
			arrange: func(engine *Engine) {
				engine.agent = core.NewAgent(core.AgentConfig{
					Name:    "invalid-start-agent",
					Actions: []core.Action{nil},
					Goals:   []*core.Goal{core.NewGoal(core.GoalConfig{Name: "done"})},
				})
			},
			request:   func() TurnRequest { return TurnRequest{Message: "hello"} },
			wantError: "action at index 0 is nil",
		},
		{
			name: "active deployment conflicts with definition",
			arrange: func(engine *Engine) {
				engine.agent = startFailureAgent("chat-agent", "")
			},
			request:   func() TurnRequest { return TurnRequest{Message: "hello"} },
			wantError: "deployment conflict",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			engine := newStartFailureEngine(t)
			runtimeEngine := engine.runtime
			if test.arrange != nil {
				test.arrange(engine)
			}

			request := test.request()
			observer, _ := request.Observer.(*recordingObserver)
			process, err := engine.StartTurn(context.Background(), request)
			if err == nil {
				t.Fatal("StartTurn() error = nil, want process creation error")
			}
			if process != nil {
				t.Fatalf("StartTurn() process = %T, want nil", process)
			}
			if !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("StartTurn() error = %v, want detail %q", err, test.wantError)
			}
			if processes := runtimeEngine.Processes(); len(processes) != 0 {
				t.Fatalf("registered processes = %d, want 0 after create failure", len(processes))
			}
			if observer != nil && (len(observer.starts()) != 0 || len(observer.ends()) != 0 || len(observer.deltas()) != 0) {
				t.Fatal("observer received callbacks for a process that was never created")
			}
		})
	}
}

func newStartFailureEngine(t *testing.T) *Engine {
	t.Helper()

	model := newStreamingStubModel("unused")
	client, err := chatclient.New(model)
	if err != nil {
		t.Fatalf("chatclient.New: %v", err)
	}
	engine, err := New(context.Background(), Config{ChatClient: client})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return engine
}

func startFailureAgent(name, planner string) *core.Agent {
	return agent.New(agent.AgentConfig{
		Name:        name,
		Version:     "1.0.0",
		PlannerName: planner,
		Actions: []agent.Action{
			agent.NewAction("finish", func(_ context.Context, _ *core.ProcessContext, input startFailureInput) (startFailureOutput, error) {
				return startFailureOutput(input), nil
			}, core.ActionConfig{}),
		},
		Goals: []*agent.Goal{
			agent.NewOutputGoal[startFailureOutput](core.GoalConfig{Description: "done"}),
		},
	})
}

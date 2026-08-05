package turn

import (
	"testing"
	"time"

	"github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/accounting"
)

func TestTurnObserverProjectsAdmittedChildOntoExactExecutorSource(t *testing.T) {
	state := newPreparingTurnState(t.Context(), Handle{
		SessionID: "session",
		TurnID:    "turn",
	})
	t.Cleanup(state.cancel)
	observer := &turnObserver{
		controller:       &controller{},
		st:               state,
		projectChildRuns: true,
	}
	startedAt := time.Date(2026, time.July, 30, 10, 0, 0, 0, time.UTC)
	child := agentexec.ChildProcess{
		ProcessRef: agentexec.ProcessRef{
			ID:          "child",
			ParentID:    "root",
			SpawnCallID: "call-task",
		},
		StartedAt: startedAt,
	}

	observer.OnMessageDelta(child.ProcessRef, "answer")
	observer.OnReasoningDelta(child.ProcessRef, "thinking")
	observer.OnUsage(child.ProcessRef, agentexec.UsageProgress{
		Usage: accounting.TokenUsage{
			PromptTokens:     8,
			CompletionTokens: 2,
		},
		CostUSD:       0.25,
		Steps:         1,
		ContextTokens: 8,
	})
	observer.OnToolCallStart(child.ProcessRef, "child-call", "provider-call", "shell", `{"command":"pwd","description":"Print the working directory"}`)
	observer.OnToolCallEnd(child.ProcessRef, "child-call", "shell", `{"command":"pwd","description":"Print the working directory"}`, "/workspace", nil, nil, nil)
	observer.OnChildProcessEnd(agentexec.ChildCompletion{
		Process:    child,
		Status:     core.StatusCompleted,
		StopReason: agent.InteractionStopNone,
		Usage: accounting.TokenUsage{
			PromptTokens:     8,
			CompletionTokens: 2,
		},
		UsageByModel: []accounting.ModelUsage{{
			Model: "model",
			TokenUsage: accounting.TokenUsage{
				PromptTokens:     8,
				CompletionTokens: 2,
			},
			CostUSD: 0.25,
			Calls:   1,
		}},
		CostUSD:     0.25,
		Steps:       1,
		CompletedAt: startedAt.Add(2 * time.Second),
	})

	payloads := make([]runs.ExecutorPayload, 0, 6)
	for range 6 {
		event := <-state.events
		if event.Source != (runs.ExecutorSource{
			ProcessID:   child.ID,
			ParentID:    child.ParentID,
			SpawnCallID: child.SpawnCallID,
		}) {
			t.Fatalf("event source = %+v, want child %+v", event.Source, child.ProcessRef)
		}
		payloads = append(payloads, event.Payload)
	}
	if _, ok := payloads[0].(runs.MessageDelta); !ok {
		t.Fatalf("payload[0] = %T, want MessageDelta", payloads[0])
	}
	if _, ok := payloads[1].(runs.ReasoningDelta); !ok {
		t.Fatalf("payload[1] = %T, want ReasoningDelta", payloads[1])
	}
	if _, ok := payloads[2].(runs.UsageReported); !ok {
		t.Fatalf("payload[2] = %T, want UsageReported", payloads[2])
	}
	if _, ok := payloads[3].(runs.ToolCallStarted); !ok {
		t.Fatalf("payload[3] = %T, want ToolCallStarted", payloads[3])
	}
	if _, ok := payloads[4].(runs.ToolCallFinished); !ok {
		t.Fatalf("payload[4] = %T, want ToolCallFinished", payloads[4])
	}
	end, ok := payloads[5].(runs.SegmentEnded)
	if !ok {
		t.Fatalf("payload[5] = %T, want TurnEnd", payloads[5])
	}
	if end.Reason != execution.OutcomeCompleted ||
		end.Duration != 2*time.Second ||
		end.Usage == nil ||
		end.Usage.Tokens.PromptTokens != 8 ||
		end.Usage.Steps != 1 ||
		len(end.Usage.ByModel) != 1 {
		t.Fatalf("child TurnEnd = %+v", end)
	}
}

func TestTurnObserverDoesNotProjectUnadmittedSDKChildren(t *testing.T) {
	for _, test := range []struct {
		name    string
		enabled bool
		process agentexec.ProcessRef
	}{
		{
			name:    "feature disabled",
			process: agentexec.ProcessRef{ID: "child", ParentID: "root", SpawnCallID: "call-task"},
		},
		{
			name:    "direct SDK child",
			enabled: true,
			process: agentexec.ProcessRef{ID: "child", ParentID: "root"},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			state := newPreparingTurnState(t.Context(), Handle{
				SessionID: "session",
				TurnID:    "turn",
			})
			t.Cleanup(state.cancel)
			observer := &turnObserver{
				controller:       &controller{},
				st:               state,
				projectChildRuns: test.enabled,
			}
			observer.OnMessageDelta(test.process, "must stay internal")
			observer.OnUsage(test.process, agentexec.UsageProgress{
				Usage:         accounting.TokenUsage{PromptTokens: 1},
				Steps:         1,
				ContextTokens: 1,
			})
			observer.OnChildProcessEnd(agentexec.ChildCompletion{
				Process: agentexec.ChildProcess{
					ProcessRef: test.process,
					StartedAt:  time.Now(),
				},
				Status:      core.StatusCompleted,
				CompletedAt: time.Now(),
			})
			if got := len(state.events); got != 0 {
				t.Fatalf("projected %d events for unadmitted child", got)
			}
		})
	}
}

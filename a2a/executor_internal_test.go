package a2a

import (
	"context"
	"iter"
	"strings"
	"testing"

	sdka2a "github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2asrv"
)

type nilSequenceAgent struct{}

func (nilSequenceAgent) Run(context.Context, string) iter.Seq2[string, error] { return nil }

func TestExecutorTurnsNilAgentSequenceIntoFailedTask(t *testing.T) {
	exec, err := newExecutor(nilSequenceAgent{})
	if err != nil {
		t.Fatal(err)
	}
	execCtx := &a2asrv.ExecutorContext{
		TaskID:    "task-1",
		ContextID: "context-1",
		Message:   sdka2a.NewMessage(sdka2a.MessageRoleUser, sdka2a.NewTextPart("hello")),
	}

	var final *sdka2a.TaskStatusUpdateEvent
	for event, eventErr := range exec.Execute(t.Context(), execCtx) {
		if eventErr != nil {
			t.Fatalf("Execute event error: %v", eventErr)
		}
		if status, ok := event.(*sdka2a.TaskStatusUpdateEvent); ok {
			final = status
		}
	}
	if final == nil || final.Status.State != sdka2a.TaskStateFailed || final.Status.Message == nil {
		t.Fatalf("final event = %#v, want failed status with message", final)
	}
	if detail := textOfParts(final.Status.Message.Parts); !strings.Contains(detail, "nil output sequence") {
		t.Fatalf("failure detail = %q", detail)
	}
}

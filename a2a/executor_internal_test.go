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

type chunkedAgent []string

func (chunks chunkedAgent) Run(context.Context, string) iter.Seq2[string, error] {
	return func(yield func(string, error) bool) {
		for _, chunk := range chunks {
			if !yield(chunk, nil) {
				return
			}
		}
	}
}

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

func TestExecutorStreamsOneArtifact(t *testing.T) {
	exec, err := newExecutor(chunkedAgent{"hello", " ", "world"})
	if err != nil {
		t.Fatal(err)
	}
	execCtx := &a2asrv.ExecutorContext{
		TaskID:    "task-1",
		ContextID: "context-1",
		Message:   sdka2a.NewMessage(sdka2a.MessageRoleUser, sdka2a.NewTextPart("hello")),
	}

	var updates []*sdka2a.TaskArtifactUpdateEvent
	for event, eventErr := range exec.Execute(t.Context(), execCtx) {
		if eventErr != nil {
			t.Fatalf("Execute event error: %v", eventErr)
		}
		if update, ok := event.(*sdka2a.TaskArtifactUpdateEvent); ok {
			updates = append(updates, update)
		}
	}
	if len(updates) != 3 {
		t.Fatalf("artifact updates = %d, want 3", len(updates))
	}
	artifactID := updates[0].Artifact.ID
	if artifactID == "" || updates[0].Append {
		t.Fatalf("first artifact update = %#v, want a new artifact", updates[0])
	}
	var text strings.Builder
	for i, update := range updates {
		if update.Artifact.ID != artifactID {
			t.Fatalf("artifact update %d ID = %q, want %q", i, update.Artifact.ID, artifactID)
		}
		if i > 0 && !update.Append {
			t.Fatalf("artifact update %d Append = false, want true", i)
		}
		text.WriteString(textOfParts(update.Artifact.Parts))
	}
	if got := text.String(); got != "hello world" {
		t.Fatalf("artifact text = %q, want %q", got, "hello world")
	}
}

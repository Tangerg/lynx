package server

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/application/admission"
	"github.com/Tangerg/lynx/app/runtime/internal/application/queries"
	"github.com/Tangerg/lynx/app/runtime/internal/application/sessions"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
)

// fakeInterruptReader backs the query coordinator's interrupt read for the
// ListOpenInterrupts wire-projection test.
type fakeInterruptReader struct {
	sessionID string
	pending   []interrupts.Pending
	err       error
}

func (r *fakeInterruptReader) List(_ context.Context, sessionID string) ([]interrupts.Pending, error) {
	r.sessionID = sessionID
	return r.pending, r.err
}

func (r *fakeInterruptReader) Get(_ context.Context, runID string) (interrupts.Pending, bool, error) {
	for _, pending := range r.pending {
		if pending.RunID == runID {
			return pending, true, r.err
		}
	}
	return interrupts.Pending{}, false, r.err
}

func TestSessionStatesPreservesInterruptReadFailure(t *testing.T) {
	want := errors.New("interrupt store unavailable")
	reader := &fakeInterruptReader{err: want}
	coordinator := sessions.New(sessions.Dependencies{Interrupts: reader, Admissions: new(admission.Gate)})
	if _, err := coordinator.SessionStates(t.Context(), []string{"ses_1", "ses_2"}); !errors.Is(err, want) {
		t.Fatalf("SessionStates error = %v, want interrupt read failure", err)
	}
}

func TestSessionStatesDoNotQueryInterruptsForActiveRun(t *testing.T) {
	reader := &fakeInterruptReader{err: errors.New("must not be read")}
	gate := &admission.Gate{}
	if _, ok := gate.AcquireSession("ses_1"); !ok {
		t.Fatal("AcquireSession rejected an empty registry")
	}
	coordinator := sessions.New(sessions.Dependencies{Interrupts: reader, Admissions: gate})
	states, err := coordinator.SessionStates(t.Context(), []string{"ses_1"})
	if err != nil || states["ses_1"] != sessions.SessionRunning {
		t.Fatalf("SessionStates = (%q, %v), want running", states["ses_1"], err)
	}
}

func TestListOpenInterruptsProjectsToWire(t *testing.T) {
	created := time.Date(2026, 7, 5, 11, 0, 0, 0, time.UTC)
	arguments, err := tool.ArgumentsFromMap(map[string]any{"command": "go test ./..."})
	if err != nil {
		t.Fatalf("tool arguments: %v", err)
	}
	reader := &fakeInterruptReader{pending: []interrupts.Pending{
		{
			RunID:     "run_waiting",
			SessionID: "ses_1",
			Interrupts: []transcript.Interrupt{{
				ItemID: "item_1", Kind: transcript.ApprovalInterrupt,
				Approval: &transcript.Approval{
					Tool: transcript.ToolInvocation{Name: "shell", Arguments: arguments},
					Risk: tool.RiskHigh, Reason: "Runs commands in the workspace.", Rememberable: true,
				},
			}},
			CreatedAt: created,
		},
	}}
	s := &Server{queries: queries.New(queries.Dependencies{Interrupts: reader})}

	got, err := s.ListOpenInterrupts(context.Background(), protocol.ListOpenInterruptsRequest{SessionID: "ses_1"})
	if err != nil {
		t.Fatalf("list open interrupts: %v", err)
	}
	if reader.sessionID != "ses_1" {
		t.Fatalf("read session = %q, want ses_1", reader.sessionID)
	}
	if len(got.Data) != 1 {
		t.Fatalf("open interrupts = %+v, want one typed record", got.Data)
	}
	open := got.Data[0]
	if open.RunID != "run_waiting" || open.SessionID != "ses_1" || !open.CreatedAt.Equal(created) || len(open.Interrupts) != 1 {
		t.Fatalf("wire open interrupt = %+v", open)
	}
	interrupt := open.Interrupts[0]
	if interrupt.Type != protocol.InterruptApproval || interrupt.ItemID != "item_1" || interrupt.Payload == nil || interrupt.Payload.Tool == nil || !interrupt.Payload.Rememberable {
		t.Fatalf("wire interrupt = %+v, want typed approval payload", interrupt)
	}
	if interrupt.Payload.Tool.Name != "shell" || interrupt.Payload.Tool.Arguments["command"] != "go test ./..." {
		t.Fatalf("wire interrupt tool = %+v", interrupt.Payload.Tool)
	}
	if interrupt.Payload.Risk != protocol.ApprovalRiskHigh || interrupt.Payload.Reason != "Runs commands in the workspace." {
		t.Fatalf("wire interrupt risk/reason = %q/%q", interrupt.Payload.Risk, interrupt.Payload.Reason)
	}
}

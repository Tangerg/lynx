package server

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec/turn"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
)

// resumeOKTurns is a turn dispatcher whose Resume succeeds and whose Cancel is a
// no-op — enough to carry ResumeRun past the interrupt consume + turn resume so
// the failing continuation Start is what's under test.
type resumeOKTurns struct{ turnRuntime }

func (resumeOKTurns) Resume(context.Context, turn.TurnHandle, interrupts.Resolution, []string) error {
	return nil
}
func (resumeOKTurns) Cancel(context.Context, turn.TurnHandle) error { return nil }
func (resumeOKTurns) ProcessID(_ context.Context, handle turn.TurnHandle) (string, error) {
	return handle.TurnID, nil
}

// TestResumeRun_KeepsInterruptOpenWhenStartFails proves ownership ordering: the
// continuation must durably open before its parked decision is delivered. A
// pre-opening Start failure therefore leaves the interrupt untouched and retryable
// without a compensation write.
func TestResumeRun_KeepsInterruptOpenWhenStartFails(t *testing.T) {
	s, rt := rollbackHarness(t)
	rt.turns = resumeOKTurns{}
	ctx := context.Background()
	sess, _ := rt.sess.Create(ctx, "s", "/w")

	if err := rt.interrupts.Put(ctx, interrupts.Pending{
		RunID:     "run_1",
		SessionID: sess.ID,
		TurnID:    "turn_parked",
		ProcessID: "turn_parked",
		Provider:  "openai",
		Model:     "gpt",
		Interrupts: []transcript.Interrupt{{
			ItemID: "item_1",
			Kind:   transcript.ApprovalInterrupt,
			Approval: &transcript.Approval{
				Tool: transcript.ToolInvocation{Name: "shell"},
			},
		}},
	}); err != nil {
		t.Fatalf("seed interrupt: %v", err)
	}

	// Close the run coordinator so continuation admission fails before opening.
	s.coordinator.Close()

	if _, _, err := s.ResumeRun(ctx, protocol.ResumeRunRequest{
		RunID: "run_1",
		Responses: []protocol.InterruptResponse{{
			ItemID: "item_1",
			Response: protocol.InterruptResponseValue{
				Type: protocol.InterruptResponseApproval, Decision: protocol.ApprovalApprove,
			},
		}},
	}); err == nil {
		t.Fatal("ResumeRun must surface the failed continuation Start")
	}

	// No compensation is needed: the opening transaction never consumed it.
	if _, found, err := rt.interrupts.Get(ctx, "run_1"); err != nil || !found {
		t.Fatalf("interrupt changed after rejected resume Start (found=%v err=%v)", found, err)
	}
}

func TestResumeRunRejectsMissingAndUnknownItemCoverage(t *testing.T) {
	s, rt := rollbackHarness(t)
	ctx := t.Context()
	sess, _ := rt.sess.Create(ctx, "s", "/w")
	pending := interrupts.Pending{
		RunID: "run_coverage", SessionID: sess.ID, TurnID: "turn_parked", ProcessID: "turn_parked",
		Interrupts: []transcript.Interrupt{{
			ItemID: "item_open",
			Kind:   transcript.ApprovalInterrupt,
			Approval: &transcript.Approval{
				Tool: transcript.ToolInvocation{Name: "shell"},
			},
		}},
	}
	if err := rt.interrupts.Put(ctx, pending); err != nil {
		t.Fatalf("seed interrupt: %v", err)
	}

	if _, _, err := s.ResumeRun(ctx, protocol.ResumeRunRequest{RunID: pending.RunID}); !errors.Is(err, protocol.ErrInvalidParams) ||
		!errors.Is(err, runs.ErrInvalidInterruptResponse) {
		t.Fatalf("empty responses error = %v, want invalid_params wrapping ErrInvalidInterruptResponse", err)
	}
	if _, _, err := s.ResumeRun(ctx, protocol.ResumeRunRequest{
		RunID: pending.RunID,
		Responses: []protocol.InterruptResponse{{
			ItemID: "item_unknown",
			Response: protocol.InterruptResponseValue{
				Type: protocol.InterruptResponseApproval, Decision: protocol.ApprovalApprove,
			},
		}},
	}); !errors.Is(err, protocol.ErrInterruptNotOpen) {
		t.Fatalf("unknown item error = %v, want interrupt_not_open", err)
	}
	if _, found, err := rt.interrupts.Get(ctx, pending.RunID); err != nil || !found {
		t.Fatalf("invalid responses consumed interrupt (found=%v err=%v)", found, err)
	}
}

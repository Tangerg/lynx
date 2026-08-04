package server

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec/turn"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelref"
)

// resumeOKTurns is a turn dispatcher whose Resume succeeds and whose Cancel is a
// no-op — enough to carry ResumeRun past the interrupt consume + turn resume so
// the failing continuation Start is what's under test.
type resumeOKTurns struct{ turnRuntime }

func (resumeOKTurns) Resume(context.Context, turn.Handle, []agentexec.SuspensionAnswer, []execution.InterruptKind) error {
	return nil
}
func (resumeOKTurns) Cancel(context.Context, turn.Handle) error { return nil }
func (resumeOKTurns) ProcessID(_ context.Context, handle turn.Handle) (string, error) {
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

	pending := serverPending(
		"run_1",
		sess.ID,
		"turn_parked",
		"turn_parked",
		[]transcript.Interrupt{{
			ItemID: "item_1",
			Kind:   execution.ApprovalInterrupt,
			Approval: &transcript.Approval{
				Tool: transcript.ToolInvocation{Name: "shell"},
			},
		}},
		time.Unix(1, 0).UTC(),
	)
	pending.Continuations[0].ModelSelection = mustResumeSelection(t, "openai", "gpt")
	if err := rt.interrupts.Open(ctx, pending); err != nil {
		t.Fatalf("seed interrupt: %v", err)
	}

	// Stop the run coordinator so continuation admission fails before opening.
	shutdown, ok := s.runs.(interface {
		BeginShutdown()
		AwaitShutdown(context.Context) error
	})
	if !ok {
		t.Fatal("test run coordinator does not expose shutdown lifecycle")
	}
	shutdown.BeginShutdown()
	if err := shutdown.AwaitShutdown(ctx); err != nil {
		t.Fatalf("shutdown run coordinator: %v", err)
	}

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

func mustResumeSelection(t testing.TB, provider, model string) modelref.Selection {
	t.Helper()
	selection, err := modelref.New(provider, model)
	if err != nil {
		t.Fatalf("modelref.New(%q, %q): %v", provider, model, err)
	}
	return selection
}

func TestResumeRunRejectsMissingAndUnknownItemCoverage(t *testing.T) {
	s, rt := rollbackHarness(t)
	ctx := withClientCapabilities(protocol.ClientCapabilities{
		InterruptTypes: []protocol.InterruptType{protocol.InterruptApproval},
	})
	sess, _ := rt.sess.Create(ctx, "s", "/w")
	pending := serverPending(
		"run_coverage",
		sess.ID,
		"turn_parked",
		"turn_parked",
		[]transcript.Interrupt{{
			ItemID: "item_open",
			Kind:   execution.ApprovalInterrupt,
			Approval: &transcript.Approval{
				Tool: transcript.ToolInvocation{Name: "shell"},
			},
		}},
		time.Unix(1, 0).UTC(),
	)
	if err := rt.interrupts.Open(ctx, pending); err != nil {
		t.Fatalf("seed interrupt: %v", err)
	}

	if _, _, err := s.ResumeRun(ctx, protocol.ResumeRunRequest{RunID: pending.RootRunID}); !errors.Is(err, protocol.ErrInvalidParams) ||
		!errors.Is(err, runs.ErrInvalidInterruptResponse) {
		t.Fatalf("empty responses error = %v, want invalid_params wrapping ErrInvalidInterruptResponse", err)
	}
	if _, _, err := s.ResumeRun(ctx, protocol.ResumeRunRequest{
		RunID: pending.RootRunID,
		Responses: []protocol.InterruptResponse{{
			ItemID: "item_unknown",
			Response: protocol.InterruptResponseValue{
				Type: protocol.InterruptResponseApproval, Decision: protocol.ApprovalApprove,
			},
		}},
	}); !errors.Is(err, protocol.ErrInterruptNotOpen) {
		t.Fatalf("unknown item error = %v, want interrupt_not_open", err)
	}
	if _, _, err := s.ResumeRun(ctx, protocol.ResumeRunRequest{
		RunID: pending.RootRunID,
		Responses: []protocol.InterruptResponse{{
			ItemID: "item_open",
			Response: protocol.InterruptResponseValue{
				Type: protocol.InterruptResponseApproval, Decision: protocol.ApprovalApprove,
				Remember: &protocol.RememberScope{Scope: protocol.RememberSession},
			},
		}},
	}); !errors.Is(err, protocol.ErrInvalidParams) || !errors.Is(err, runs.ErrInvalidInterruptResponse) {
		t.Fatalf("remembering one-off approval error = %v, want invalid params", err)
	}
	if _, found, err := rt.interrupts.Get(ctx, pending.RootRunID); err != nil || !found {
		t.Fatalf("invalid responses consumed interrupt (found=%v err=%v)", found, err)
	}
}

func TestQuestionAnswerParamsErrorPreservesExactAnswerPath(t *testing.T) {
	err := questionAnswerParamsError(
		[]protocol.InterruptResponse{{ItemID: "item_other"}, {ItemID: "item_question"}},
		&runs.QuestionAnswerError{ItemID: "item_question", Index: 2, Detail: "unknown choice"},
	)
	if !errors.Is(err, protocol.ErrInvalidParams) {
		t.Fatalf("error = %v, want invalid_params", err)
	}
	var constraint *protocol.ConstraintError
	if !errors.As(err, &constraint) {
		t.Fatalf("error = %v, want ConstraintError", err)
	}
	want := protocol.FieldError{
		Field: "responses[1].response.answers[2]", Detail: "unknown choice",
	}
	if len(constraint.Fields) != 1 || constraint.Fields[0] != want {
		t.Fatalf("fields = %#v, want %#v", constraint.Fields, want)
	}
}

package server

import (
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
)

// Why a run stopped is a domain fact, and a sentence explaining it to a person
// is the client's to write in the reader's language. Presentation sits between
// the two and owns neither: it maps the detail the domain reported, including
// the absence of one. Supplying a default here made the same failure read one
// way through this path and another through the Artifact encoder, and shipped
// English no translator could see.
func TestPresentationDoesNotAuthorOutcomeOrProblemDetail(t *testing.T) {
	maxBudget := execution.OutcomeMaxBudget
	if outcome := presentOutcome(transcript.Run{Outcome: &maxBudget}); outcome.Detail != "" {
		t.Fatalf("budget outcome detail = %q, want the domain's silence preserved", outcome.Detail)
	}

	problem := presentProblem(&transcript.Problem{Kind: transcript.RunLostProblem, Scope: transcript.RunProblem})
	if problem == nil || problem.Type != protocol.ProblemRunLost || problem.Detail != "" {
		t.Fatalf("run-lost problem = %+v, want the type alone", problem)
	}

	canceled := execution.OutcomeCanceled
	spoken := presentOutcome(transcript.Run{Outcome: &canceled, Detail: "user asked to stop"})
	if spoken.Detail != "user asked to stop" {
		t.Fatalf("canceled outcome detail = %q, want it verbatim", spoken.Detail)
	}

	raw := presentProblem(&transcript.Problem{Kind: transcript.ToolFailedProblem, Scope: transcript.ToolProblem, Detail: "exit status 2"})
	if raw == nil || raw.Detail != "exit status 2" {
		t.Fatalf("raw tool problem = %+v, want it verbatim", raw)
	}
}

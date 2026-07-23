package server

import (
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
)

func TestPresentationOwnsStaticRunAndProblemDetails(t *testing.T) {
	maxBudget := execution.OutcomeMaxBudget
	outcome := presentOutcome(transcript.Run{Outcome: &maxBudget})
	if outcome.Detail != "reached the configured budget" {
		t.Fatalf("budget outcome detail = %q", outcome.Detail)
	}

	problem := presentProblem(&transcript.Problem{Kind: transcript.RunLostProblem, Scope: transcript.RunProblem})
	if problem == nil || problem.Type != protocol.ProblemRunLost || problem.Detail != "run process state is unavailable" {
		t.Fatalf("run-lost problem = %+v", problem)
	}

	raw := presentProblem(&transcript.Problem{Kind: transcript.ToolFailedProblem, Scope: transcript.ToolProblem, Detail: "exit status 2"})
	if raw == nil || raw.Detail != "exit status 2" {
		t.Fatalf("raw tool problem = %+v", raw)
	}
}

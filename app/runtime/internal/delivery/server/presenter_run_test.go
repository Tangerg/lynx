package server

import (
	"testing"

	"github.com/Tangerg/scope/app/runtime/internal/domain/run"
	"github.com/Tangerg/scope/app/runtime/internal/domain/tool"
	runfixture "github.com/Tangerg/scope/app/runtime/internal/testsupport/runfixture"
	"github.com/Tangerg/scope/app/runtime/protocol"
)

// Why a run stopped is a domain fact, and a sentence explaining it to a person
// is the client's to write in the reader's language. Presentation sits between
// the two and owns neither: it maps the detail the domain reported, including
// the absence of one. Supplying a default here made the same failure read one
// way through this path and another through the Artifact encoder, and shipped
// English no translator could see.
func TestPresentationDoesNotAuthorOutcomeOrFailureDetail(t *testing.T) {
	maxBudget := run.OutcomeMaxBudget
	if outcome := presentOutcome(runfixture.MustRestore(run.Snapshot{Outcome: &maxBudget})); outcome.Detail != "" {
		t.Fatalf("budget outcome detail = %q, want the domain's silence preserved", outcome.Detail)
	}

	problem := presentRunFailure(&run.Failure{Kind: run.FailureLost})
	if problem == nil || problem.Type != protocol.ProblemRunLost || problem.Detail != "" {
		t.Fatalf("run-lost problem = %+v, want the type alone", problem)
	}

	canceled := run.OutcomeCanceled
	spoken := presentOutcome(runfixture.MustRestore(run.Snapshot{Outcome: &canceled, Detail: "user asked to stop"}))
	if spoken.Detail != "user asked to stop" {
		t.Fatalf("canceled outcome detail = %q, want it verbatim", spoken.Detail)
	}

	raw := presentToolFailure(&tool.Failure{Kind: tool.FailureExecution, Detail: "exit status 2"})
	if raw == nil || raw.Detail != "exit status 2" {
		t.Fatalf("raw tool problem = %+v, want it verbatim", raw)
	}
	canceledTool := presentToolFailure(&tool.Failure{Kind: tool.FailureCanceled})
	if canceledTool == nil || canceledTool.Type != protocol.ProblemToolCanceled {
		t.Fatalf("canceled tool problem = %+v", canceledTool)
	}
}

func TestPresentRunCarriesDurablePromptFootprint(t *testing.T) {
	value := runfixture.MustRestore(run.Snapshot{ContextTokens: 87_900})
	if got := presentRun(value).ContextTokens; got != 87_900 {
		t.Fatalf("presented contextTokens = %d, want 87900", got)
	}
}

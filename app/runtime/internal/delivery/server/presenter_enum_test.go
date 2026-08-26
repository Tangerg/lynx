package server

import (
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/interrupt"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/run"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
	runfixture "github.com/Tangerg/lynx/app/runtime/internal/testsupport/runfixture"
	"github.com/Tangerg/lynx/app/runtime/protocol"
)

func TestPresentersRejectUnknownDomainEnums(t *testing.T) {
	mustPanic(t, func() { presentItemStatus(transcript.ItemStatus("invalid")) })
	mustPanic(t, func() { presentItemKind(transcript.ItemKind("unknown")) })
	mustPanic(t, func() { presentContent(transcript.ContentBlock{Kind: transcript.ContentKind("invalid")}) })
	mustPanic(t, func() {
		presentQuestion(transcript.Question{Fields: []transcript.QuestionField{{Kind: transcript.QuestionFieldKind("invalid")}}})
	})
	mustPanic(t, func() { presentDelta(runs.ItemDelta{Kind: runs.ItemDeltaKind("invalid")}) })
	mustPanic(t, func() { presentRun(runfixture.MustRestore(run.Snapshot{State: run.State("invalid")})) })
	mustPanic(t, func() { presentOutcome(runfixture.MustRestore(run.Snapshot{State: run.Running})) })
	mustPanic(t, func() { presentRunFailure(&run.Failure{Kind: run.FailureKind("invalid")}) })
	mustPanic(t, func() { presentToolFailure(&tool.Failure{Kind: tool.FailureKind("invalid")}) })
	mustPanic(t, func() { presentInterrupts([]transcript.Interrupt{{Kind: interrupt.Kind("invalid")}}) })
}

func TestRunOutcomeProjectionIsExhaustive(t *testing.T) {
	tests := []struct {
		domain   run.Outcome
		wire     protocol.RunOutcomeType
		artifact protocol.ArtifactOutcomeType
	}{
		{run.OutcomeCompleted, protocol.OutcomeCompleted, protocol.ArtifactOutcomeCompleted},
		{run.OutcomeCanceled, protocol.OutcomeCanceled, protocol.ArtifactOutcomeCanceled},
		{run.OutcomeTimedOut, protocol.OutcomeTimedOut, protocol.ArtifactOutcomeTimedOut},
		{run.OutcomeFailed, protocol.OutcomeFailed, protocol.ArtifactOutcomeFailed},
		{run.OutcomeMaxBudget, protocol.OutcomeMaxBudget, protocol.ArtifactOutcomeMaxBudget},
		{run.OutcomeMaxSteps, protocol.OutcomeMaxSteps, protocol.ArtifactOutcomeMaxSteps},
		{run.OutcomeLost, protocol.OutcomeLost, protocol.ArtifactOutcomeLost},
	}
	for _, test := range tests {
		t.Run(test.domain.String(), func(t *testing.T) {
			outcome := test.domain
			if got := presentOutcome(runfixture.MustRestore(run.Snapshot{Outcome: &outcome})); got.Type != test.wire {
				t.Fatalf("presentOutcome type = %q, want %q", got.Type, test.wire)
			}
			encoded, err := artifactOutcomeType(test.domain)
			if err != nil || encoded != test.artifact {
				t.Fatalf("artifactOutcomeType = (%q, %v), want (%q, nil)", encoded, err, test.artifact)
			}
			decoded, err := portableOutcomeFromArtifact("outcome.type", encoded)
			if err != nil || decoded != test.domain {
				t.Fatalf("portableOutcomeFromArtifact = (%s, %v), want (%s, nil)", decoded, err, test.domain)
			}
		})
	}
}

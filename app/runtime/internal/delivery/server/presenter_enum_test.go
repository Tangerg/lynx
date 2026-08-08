package server

import (
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/interrupt"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/run"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
)

func TestPresentersRejectUnknownDomainEnums(t *testing.T) {
	mustPanic(t, func() { presentItemStatus(transcript.ItemStatus(99)) })
	mustPanic(t, func() { presentItemKind(transcript.ItemKind(99)) })
	mustPanic(t, func() { presentContent(transcript.ContentBlock{Kind: transcript.ContentKind(99)}) })
	mustPanic(t, func() {
		presentQuestion(transcript.Question{Fields: []transcript.QuestionField{{Kind: transcript.QuestionFieldKind(99)}}})
	})
	mustPanic(t, func() { presentDelta(runs.ItemDelta{Kind: runs.ItemDeltaKind(99)}) })
	mustPanic(t, func() { presentRun(transcript.Run{State: run.RunState(99)}) })
	mustPanic(t, func() { presentOutcome(transcript.Run{State: run.Completed, Outcome: nil}) })
	mustPanic(t, func() { presentProblem(&transcript.Problem{Kind: transcript.ProblemKind(99)}) })
	mustPanic(t, func() { presentInterrupts([]transcript.Interrupt{{Kind: interrupt.Kind(99)}}) })
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
			if got := presentOutcome(transcript.Run{Outcome: &outcome}); got.Type != test.wire {
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

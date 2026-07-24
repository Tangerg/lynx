package server

import (
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
)

func TestPresentersRejectUnknownDomainEnums(t *testing.T) {
	mustPanic(t, func() { presentItemStatus(transcript.ItemStatus(99)) })
	mustPanic(t, func() { presentItemKind(transcript.ItemKind(99)) })
	mustPanic(t, func() { presentContent(transcript.ContentBlock{Kind: transcript.ContentKind(99)}) })
	mustPanic(t, func() { presentPlanSteps([]transcript.PlanStep{{Status: "future"}}) })
	mustPanic(t, func() {
		presentQuestion(transcript.Question{Fields: []transcript.QuestionField{{Kind: transcript.QuestionFieldKind(99)}}})
	})
	mustPanic(t, func() { presentDelta(runs.ItemDelta{Kind: runs.ItemDeltaKind(99)}) })
	mustPanic(t, func() { presentRun(transcript.Run{State: execution.RunState(99)}) })
	mustPanic(t, func() { presentOutcome(transcript.Run{State: execution.Completed, Outcome: nil}) })
	mustPanic(t, func() { presentProblem(&transcript.Problem{Kind: transcript.ProblemKind(99)}) })
	mustPanic(t, func() { presentInterrupts([]transcript.Interrupt{{Kind: transcript.InterruptKind(99)}}) })
}

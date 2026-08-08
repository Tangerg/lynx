package server

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/run"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
)

type cancelRunUseCaseStub struct {
	runUseCases
	result  runs.CancelResult
	err     error
	command runs.CancelCommand
}

func (s *cancelRunUseCaseStub) Cancel(_ context.Context, command runs.CancelCommand) (runs.CancelResult, error) {
	s.command = command
	return s.result, s.err
}

func TestCancelRunPresentsCommittedRootSnapshot(t *testing.T) {
	outcome := run.OutcomeCanceled
	useCases := &cancelRunUseCaseStub{result: runs.CancelResult{Run: transcript.Run{
		ID: "run_1", SessionID: "ses_1", State: run.Canceled,
		Outcome: &outcome, Detail: "user stopped",
	}}}
	server := &Server{runs: useCases}

	result, err := server.CancelRun(t.Context(), protocol.CancelRunRequest{
		RunID: "run_1", Reason: "user stopped",
	})
	if err != nil {
		t.Fatalf("CancelRun: %v", err)
	}
	if result.Type != protocol.CancelRunRoot ||
		result.Run.ID != "run_1" ||
		result.Run.Status != protocol.RunStatusFinished ||
		result.Run.Outcome == nil ||
		result.Run.Outcome.Type != protocol.OutcomeCanceled {
		t.Fatalf("CancelRun result = %+v, want root run_1 finished/canceled", result)
	}
	if useCases.command.AllowChildRun {
		t.Fatal("Minimal Profile unexpectedly authorized child cancellation")
	}
}

func TestCancelRunPassesNegotiatedChildAuthorityWithoutBlockingARoot(t *testing.T) {
	outcome := run.OutcomeCanceled
	useCases := &cancelRunUseCaseStub{result: runs.CancelResult{Run: transcript.Run{
		ID: "run_1", SessionID: "ses_1", State: run.Canceled, Outcome: &outcome,
	}}}
	server := &Server{runs: useCases}
	ctx := withClientCapabilities(protocol.ClientCapabilities{
		Features: map[string]protocol.FeaturePreference{
			protocol.FeatureSubagents: {Enabled: true},
		},
	})

	result, err := server.CancelRun(ctx, protocol.CancelRunRequest{RunID: "run_1"})
	if err != nil || result == nil || result.Type != protocol.CancelRunRoot {
		t.Fatalf("CancelRun(root) = (%+v, %v), want a successful root cancellation", result, err)
	}
	if !useCases.command.AllowChildRun {
		t.Fatal("negotiated subagents did not authorize exact child cancellation")
	}
}

func TestCancelRunMapsFinishedToTheSharedLifecycleError(t *testing.T) {
	server := &Server{runs: &cancelRunUseCaseStub{err: runs.ErrRunFinished}}

	result, err := server.CancelRun(t.Context(), protocol.CancelRunRequest{RunID: "run_1"})
	if result != nil || !errors.Is(err, protocol.ErrRunFinished) {
		t.Fatalf("CancelRun = (%+v, %v), want nil/ErrRunFinished", result, err)
	}
}

func TestCancelRunNamesTheCapabilityNeededForAChild(t *testing.T) {
	server := &Server{runs: &cancelRunUseCaseStub{err: runs.ErrChildRunNotAllowed}}

	result, err := server.CancelRun(t.Context(), protocol.CancelRunRequest{RunID: "run_child"})
	if result != nil || !errors.Is(err, protocol.ErrCapabilityNotNeg) {
		t.Fatalf("CancelRun = (%+v, %v), want nil/capability_not_negotiated", result, err)
	}
	gap, ok := errors.AsType[*protocol.CapabilityGap](err)
	if !ok || len(gap.Requirements) != 1 ||
		gap.Requirements[0].Type != protocol.RequirementFeature ||
		gap.Requirements[0].Name != protocol.FeatureSubagents {
		t.Fatalf("capability gap = %+v, want feature/subagents", gap)
	}
}

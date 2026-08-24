package agentexec

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelref"
	corechat "github.com/Tangerg/lynx/core/chat"
)

type passthroughRunWorkingContext struct{}

func (passthroughRunWorkingContext) ComposeWorkingContext(
	_ context.Context,
	input runs.WorkingContextInput,
) ([]corechat.Message, error) {
	return input.Seed, nil
}

func mustNewRunCoordinator(t *testing.T, deps runs.Dependencies) *runs.Coordinator {
	t.Helper()
	starter := any(deps.RootStarts)
	fillRunDependency(t, &deps.RootCancellation, starter, "root cancellation requester")
	fillRunDependency(t, &deps.Continuation, starter, "waiting execution continuer")
	fillRunDependency(t, &deps.WaitingRestorer, starter, "waiting execution restorer")
	fillRunDependency(t, &deps.Steering, starter, "running execution steerer")
	fillRunDependency(t, &deps.RunningSubtreeCanceler, starter, "running subtree canceler")
	fillRunDependency(t, &deps.WaitingSubtreeCancellationPreparer, starter, "waiting subtree cancellation preparer")
	if deps.WorkingContexts == nil {
		deps.WorkingContexts = passthroughRunWorkingContext{}
	}
	sessions := any(deps.Session.Reader)
	fillRunDependency(t, &deps.Session.Creator, sessions, "session creator")
	fillRunDependency(t, &deps.Session.ActiveRuns, sessions, "active run reader")
	fillRunDependency(t, &deps.Session.Interrupts, sessions, "pending interrupt reader")
	fillRunDependency(t, &deps.Session.Terminations, sessions, "run termination committer")
	projection := any(deps.Projection.Openings)
	fillRunDependency(t, &deps.Projection.ChildStarts, projection, "child run start committer")
	if deps.Projection.ResumeClaims == nil {
		if value, ok := projection.(runs.ResumeClaimCommitter); ok {
			deps.Projection.ResumeClaims = value
		} else {
			deps.Projection.ResumeClaims = inertRunProjection{}
		}
	}
	fillRunDependency(t, &deps.Projection.Events, projection, "event committer")
	fillRunDependency(t, &deps.Projection.Barriers, projection, "tree barrier committer")
	fillRunDependency(t, &deps.Projection.Checkpoints, projection, "waiting checkpoint reader")
	if deps.Projection.WaitingSubtreeCancellations == nil {
		if value, ok := projection.(runs.WaitingSubtreeCancellationCommitter); ok {
			deps.Projection.WaitingSubtreeCancellations = value
		} else {
			deps.Projection.WaitingSubtreeCancellations = inertRunProjection{}
		}
	}
	fillRunDependency(t, &deps.Projection.Workspace, projection, "workspace notifier")
	fillRunDependency(t, &deps.Projection.Finalizer, projection, "segment finalizer")
	fillRunDependency(t, &deps.Runs, projection, "run projection")
	fillRunDependency(t, &deps.Items, projection, "item projection")
	coordinator, err := runs.NewCoordinator(deps)
	if err != nil {
		t.Fatalf("construct Run coordinator: %v", err)
	}
	return coordinator
}

func testDefaultSelection() modelref.Selection {
	selection, err := modelref.New("test-provider", "test-model")
	if err != nil {
		panic(err)
	}
	return selection
}

type inertRunProjection struct{}

func (inertRunProjection) ClaimResume(context.Context, runs.ResumeClaimCommit) (runs.ClaimedResume, error) {
	return runs.ClaimedResume{}, errors.New("inert Run projection")
}

func (inertRunProjection) CommitWaitingSubtreeCancellation(
	context.Context,
	runs.WaitingSubtreeCancellationCommit,
) (runs.WaitingSubtreeCancellationResult, error) {
	return runs.WaitingSubtreeCancellationResult{}, errors.New("inert Run projection")
}

func fillRunDependency[T any](t *testing.T, target *T, source any, name string) {
	t.Helper()
	if any(*target) != nil {
		return
	}
	value, ok := source.(T)
	if !ok {
		t.Fatalf("test fixture source does not implement %s", name)
	}
	*target = value
}

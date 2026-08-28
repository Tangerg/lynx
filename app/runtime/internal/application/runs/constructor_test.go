package runs

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/scope/app/runtime/internal/application/sessionadmission"
	"github.com/Tangerg/scope/app/runtime/internal/domain/modelref"
	corechat "github.com/Tangerg/scope/core/chat"
)

func TestNewCoordinatorRejectsMalformedDependencies(t *testing.T) {
	var typedNilStarter *fakeExecutionPorts
	tests := []struct {
		name string
		deps Dependencies
		want string
	}{
		{name: "empty", deps: Dependencies{}, want: "root execution starter"},
		{name: "typed nil", deps: Dependencies{RootStarts: typedNilStarter}, want: "root execution starter"},
		{name: "invalid retention", deps: Dependencies{Retention: Retention{MaxEvents: -1, MaxBytes: 1}}, want: "retention budgets"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := NewCoordinator(test.deps); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("NewCoordinator error = %v, want %q", err, test.want)
			}
		})
	}
}

type passthroughWorkingContext struct{}

func (passthroughWorkingContext) ComposeWorkingContext(
	_ context.Context,
	input WorkingContextInput,
) ([]corechat.Message, error) {
	return input.Seed, nil
}

type acceptAllModelInputs struct{}

func (acceptAllModelInputs) AdmitSelection(modelref.Selection) error                 { return nil }
func (acceptAllModelInputs) AdmitInput(modelref.Selection, []corechat.Message) error { return nil }

// mustNewCoordinator completes dependencies unrelated to a focused test. The
// production constructor remains strict; tests opt into explicit inert doubles
// instead of manufacturing partially valid Coordinators.
func mustNewCoordinator(deps Dependencies) *Coordinator {
	control := &fakeExecutionPorts{}
	observer := &fakeExecutor{}
	sessions := &fakeRunSessions{}
	effects := &fakeEffects{}
	if deps.RootStarts == nil {
		deps.RootStarts = control
	}
	if deps.Observations == nil {
		deps.Observations = observer
	}
	if deps.Releases == nil {
		deps.Releases = control
	}
	if deps.RootCancellation == nil {
		deps.RootCancellation = control
	}
	if deps.Conversation == nil {
		deps.Conversation = emptyConversationReader{}
	}
	if deps.WorkingContexts == nil {
		deps.WorkingContexts = passthroughWorkingContext{}
	}
	if deps.Models == nil {
		deps.Models = acceptAllModelInputs{}
	}
	if deps.Continuation == nil {
		deps.Continuation = control
	}
	if deps.WaitingRestorer == nil {
		deps.WaitingRestorer = control
	}
	if deps.Steering == nil {
		deps.Steering = control
	}
	if deps.RunningSubtreeCanceler == nil {
		deps.RunningSubtreeCanceler = control
	}
	if deps.WaitingSubtreeCancellationPreparer == nil {
		deps.WaitingSubtreeCancellationPreparer = control
	}
	if deps.Session.Reader == nil {
		deps.Session.Reader = sessions
	}
	if deps.Session.Creator == nil {
		deps.Session.Creator = sessions
	}
	if deps.Session.ActiveRuns == nil {
		deps.Session.ActiveRuns = sessions
	}
	if deps.Session.Interrupts == nil {
		deps.Session.Interrupts = sessions
	}
	if deps.Session.Terminations == nil {
		deps.Session.Terminations = sessions
	}
	if deps.Projection.Openings == nil {
		deps.Projection.Openings = effects
	}
	if deps.Projection.ChildStarts == nil {
		deps.Projection.ChildStarts = effects
	}
	if deps.Projection.ResumeClaims == nil {
		deps.Projection.ResumeClaims = effects
	}
	if deps.Projection.Events == nil {
		deps.Projection.Events = effects
	}
	if deps.Projection.Barriers == nil {
		deps.Projection.Barriers = effects
	}
	if deps.Projection.Checkpoints == nil {
		deps.Projection.Checkpoints = effects
	}
	if deps.Projection.WaitingSubtreeCancellations == nil {
		deps.Projection.WaitingSubtreeCancellations = effects
	}
	if deps.Projection.Workspace == nil {
		deps.Projection.Workspace = effects
	}
	if deps.Projection.Finalizer == nil {
		deps.Projection.Finalizer = effects
	}
	if deps.Runs == nil {
		deps.Runs = &fakeRunProjection{}
	}
	if deps.Items == nil {
		deps.Items = &fakeItemProjection{}
	}
	if deps.Admissions == nil {
		deps.Admissions = new(sessionadmission.Gate)
	}
	if deps.NewRunID == nil {
		deps.NewRunID = func() string { return "run_test" }
	}
	if deps.NewSegmentID == nil {
		deps.NewSegmentID = func() string { return "segment_test" }
	}
	if deps.Now == nil {
		deps.Now = func() time.Time { return time.Unix(1, 0).UTC() }
	}
	coordinator, err := NewCoordinator(deps)
	if err != nil {
		panic(err)
	}
	return coordinator
}

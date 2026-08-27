package schedules

import (
	"context"
	"testing"

	"github.com/Tangerg/scope/app/runtime/internal/application/runs"
	"github.com/Tangerg/scope/app/runtime/internal/domain/modelref"
	"github.com/Tangerg/scope/app/runtime/internal/domain/schedule"
)

type fakeRunStarter struct {
	cmd      runs.StartCommand
	canceled chan struct{}
}

func (f *fakeRunStarter) Start(ctx context.Context, cmd runs.StartCommand) (runs.StartResult, error) {
	f.cmd = cmd
	context.AfterFunc(ctx, func() { close(f.canceled) })
	return runs.StartResult{SessionID: "ses_scheduled", RunID: "run_scheduled"}, nil
}

func TestRunLauncherUsesApplicationRunEntry(t *testing.T) {
	runStarter := &fakeRunStarter{canceled: make(chan struct{})}
	launcher := NewRunLauncher(runStarter, "/default")

	startedRun, err := launcher.StartScheduledRun(context.Background(), schedule.Occurrence{Schedule: schedule.Schedule{
		ID: "sch_1", Instructions: "summarize", ModelSelection: mustScheduleSelection("p", "m"),
	}})
	if err != nil {
		t.Fatalf("StartScheduledRun: %v", err)
	}
	if startedRun.SessionID != "ses_scheduled" || startedRun.RunID != "run_scheduled" {
		t.Fatalf("started Run=%+v", startedRun)
	}
	if runStarter.cmd.DefaultWorkspacePath != "/default" || runStarter.cmd.NewSessionTitle != "" {
		t.Fatalf("command defaults = %+v", runStarter.cmd)
	}
	if len(runStarter.cmd.Input) != 1 || runStarter.cmd.Input[0].Text != "summarize" || runStarter.cmd.ModelSelection.Provider() != "p" || runStarter.cmd.ModelSelection.Model() != "m" {
		t.Fatalf("command mapping = %+v", runStarter.cmd)
	}
	<-runStarter.canceled
}

func mustScheduleSelection(provider, model string) modelref.Selection {
	selection, err := modelref.New(provider, model)
	if err != nil {
		panic(err)
	}
	return selection
}

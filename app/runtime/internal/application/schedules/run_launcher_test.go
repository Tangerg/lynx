package schedules

import (
	"context"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/schedule"
)

type fakeRunUseCases struct {
	cmd      runs.StartCommand
	canceled chan struct{}
}

func (f *fakeRunUseCases) Start(ctx context.Context, cmd runs.StartCommand) (runs.StartResult, error) {
	f.cmd = cmd
	context.AfterFunc(ctx, func() { close(f.canceled) })
	return runs.StartResult{SessionID: "ses_scheduled"}, nil
}

func TestRunLauncherUsesApplicationRunEntry(t *testing.T) {
	useCases := &fakeRunUseCases{canceled: make(chan struct{})}
	var fired string
	launcher := NewRunLauncher(useCases, "/default", func(schedule.Schedule) runs.ProjectorFactory {
		return func(runs.ProjectorContext, runs.SegmentView) runs.Projector { return nil }
	}, func(id string) { fired = id })

	sessionID, err := launcher.StartScheduledRun(context.Background(), schedule.Schedule{
		ID: "sch_1", Prompt: "summarize", Provider: "p", Model: "m",
	})
	if err != nil {
		t.Fatalf("StartScheduledRun: %v", err)
	}
	if sessionID != "ses_scheduled" || fired != "sch_1" {
		t.Fatalf("session=%q fired=%q", sessionID, fired)
	}
	if useCases.cmd.DefaultCwd != "/default" || useCases.cmd.NewSessionTitle != "Scheduled run" {
		t.Fatalf("command defaults = %+v", useCases.cmd)
	}
	if useCases.cmd.Message != "summarize" || useCases.cmd.Provider != "p" || useCases.cmd.Model != "m" {
		t.Fatalf("command mapping = %+v", useCases.cmd)
	}
	<-useCases.canceled
}

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
	return runs.StartResult{SessionID: "ses_scheduled", RunID: "run_scheduled"}, nil
}

func TestRunLauncherUsesApplicationRunEntry(t *testing.T) {
	useCases := &fakeRunUseCases{canceled: make(chan struct{})}
	var fired string
	launcher := NewRunLauncher(useCases, "/default", func(id string) { fired = id })

	handle, err := launcher.StartScheduledRun(context.Background(), schedule.Schedule{
		ID: "sch_1", Prompt: "summarize", Provider: "p", Model: "m",
	})
	if err != nil {
		t.Fatalf("StartScheduledRun: %v", err)
	}
	if handle.SessionID != "ses_scheduled" || handle.RunID != "run_scheduled" || fired != "sch_1" {
		t.Fatalf("handle=%+v fired=%q", handle, fired)
	}
	if useCases.cmd.DefaultCwd != "/default" || useCases.cmd.NewSessionTitle != "" {
		t.Fatalf("command defaults = %+v", useCases.cmd)
	}
	if len(useCases.cmd.Input) != 1 || useCases.cmd.Input[0].Text != "summarize" || useCases.cmd.Provider != "p" || useCases.cmd.Model != "m" {
		t.Fatalf("command mapping = %+v", useCases.cmd)
	}
	<-useCases.canceled
}

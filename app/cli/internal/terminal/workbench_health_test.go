package terminal

import (
	"errors"
	"testing"

	"github.com/Tangerg/oolong/components/kit"

	"github.com/Tangerg/lynx/app/cli/internal/settings"
)

func TestWorkbenchProblemsArePrioritizedAndClearedIndependently(t *testing.T) {
	application := &app{
		status: newStatusView(kit.Dark(), kit.Unicode(), settings.Default().RunOptions()),
	}
	application.status.active("working")
	application.reportWorkbenchIssue(workbenchDraft, errors.New("draft could not be saved"))
	application.reportWorkbenchIssue(workbenchResumeOutbox, errors.New("decision could not be settled"))
	application.reportWorkbenchIssue(workbenchCancellationOwnership, errors.New("canceled ownership could not be settled"))
	if got := application.status.problem; got != "workbench: canceled ownership could not be settled" {
		t.Fatalf("canceled ownership problem was not prioritized; got %q", got)
	}

	application.reportWorkbenchIssue(workbenchCancellationOwnership, nil)
	if got := application.status.problem; got != "workbench: decision could not be settled" {
		t.Fatalf("prioritized workbench problem = %q", got)
	}

	application.reportWorkbenchIssue(workbenchResumeOutbox, nil)
	if got := application.status.problem; got != "workbench: draft could not be saved" {
		t.Fatalf("clearing resume problem also cleared draft problem; got %q", got)
	}

	application.reportWorkbenchIssue(workbenchDraft, nil)
	if got := application.status.problem; got != "" {
		t.Fatalf("cleared workbench problems left status %q", got)
	}
	if got := application.status.doing; got != "working" {
		t.Fatalf("workbench problem replaced the underlying runtime status with %q", got)
	}
}

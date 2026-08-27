package terminal

import (
	"errors"
	"testing"

	"github.com/Tangerg/oolong/components/kit"

	"github.com/Tangerg/scope/app/cli/internal/settings"
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

func TestEnteringASessionDropsOnlyProjectionOwnedWorkbenchProblems(t *testing.T) {
	health := workbenchHealth{}
	health.update(workbenchCancellationOwnership, errors.New("old cancellation"))
	health.update(workbenchResumeOutbox, errors.New("old resume"))
	health.update(workbenchRunOutbox, errors.New("old run"))
	health.update(workbenchSteerOutbox, errors.New("old steer"))
	health.update(workbenchDraft, errors.New("old draft"))
	health.update(workbenchHistory, errors.New("history unavailable"))

	health.enterSession()
	if got := health.problem(); got != "workbench: history unavailable" {
		t.Fatalf("health after session replacement = %q", got)
	}
	for concern := range workbenchSessionConcernCount {
		if problem := health.problems[concern]; problem != "" {
			t.Errorf("session concern %d retained %q", concern, problem)
		}
	}
}

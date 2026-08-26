package workbench

import (
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
)

func TestStorePersistsRunAndResumeReplayOwnership(t *testing.T) {
	directory := t.TempDir()
	store, err := Open(directory, Config{})
	if err != nil {
		t.Fatal(err)
	}
	startGuard := ReplayGuard{Namespace: "runtime-a", Until: time.Now().UTC().Add(time.Hour)}
	cancelGuard := ReplayGuard{Namespace: "runtime-a", Until: startGuard.Until.Add(time.Minute)}
	start := agent.StartRun{
		CommandID: "cli_88888888888888888888888888888888", SessionID: "ses_1",
		Message: agent.Message{Text: "persist guards"},
	}
	if stagePendingRunErr := store.StagePendingRun(PendingRun{State: PendingRunQueued, Command: start}); stagePendingRunErr != nil {
		t.Fatal(stagePendingRunErr)
	}
	if markPendingRunDispatchingErr := store.MarkPendingRunDispatching(start.SessionID, start.CommandID, startGuard); markPendingRunDispatchingErr != nil {
		t.Fatal(markPendingRunDispatchingErr)
	}
	cancelID, err := store.MarkPendingRunCanceling(start.SessionID, start.CommandID, cancelGuard)
	if err != nil {
		t.Fatal(err)
	}
	resumeGuard := ReplayGuard{Namespace: "runtime-a", Until: cancelGuard.Until.Add(time.Minute)}
	approval := agent.Approval{
		RunID: "run_2", ItemID: "approval_1", Title: "Proceed?",
		Tool: &agent.ToolCall{Kind: agent.ToolShell, Name: "shell", Status: agent.ToolRunning},
	}
	resume := PendingResume{
		Command: agent.ResumeRun{
			CommandID: "cli_99999999999999999999999999999999", RunID: approval.RunID,
			Answers: []agent.InterruptAnswer{{
				ItemID: approval.ItemID, Answer: agent.ApprovalAnswer{Decision: agent.ApprovalDeny},
			}},
		},
		Interactions: []agent.Interaction{approval}, Replay: resumeGuard,
	}
	if stagePendingResumeErr := store.StagePendingResume("ses_2", resume); stagePendingResumeErr != nil {
		t.Fatal(stagePendingResumeErr)
	}

	reopened, err := Open(directory, Config{})
	if err != nil {
		t.Fatal(err)
	}
	runs := reopened.PendingRuns(start.SessionID)
	if len(runs) != 1 || runs[0].Replay != startGuard || runs[0].CancelReplay != cancelGuard ||
		runs[0].CancelCommandID != cancelID {
		t.Fatalf("reopened run ownership = %+v", runs)
	}
	pendingResume, found := reopened.PendingResume("ses_2")
	if !found || pendingResume.Replay != resumeGuard {
		t.Fatalf("reopened resume ownership = %+v, found %t", pendingResume, found)
	}
}

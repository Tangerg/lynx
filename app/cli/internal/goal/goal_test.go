package goal

import "testing"

func TestGoalLifecycleValuesRejectAmbiguousState(t *testing.T) {
	active := Goal{SessionID: "ses_1", Objective: "finish the task", Status: Active}
	if err := active.Validate(); err != nil {
		t.Fatal(err)
	}
	active.Reason = &Reason{Code: StoppedByUser}
	if err := active.Validate(); err == nil {
		t.Fatal("active goal with a stop reason was accepted")
	}
	paused := Goal{SessionID: "ses_1", Objective: "finish the task", Status: Paused}
	if err := paused.Validate(); err == nil {
		t.Fatal("paused goal without a reason was accepted")
	}
	completing := Goal{SessionID: "ses_1", Objective: "finish the task", Status: Completing}
	if err := completing.Validate(); err != nil {
		t.Fatal(err)
	}
	completing.Reason = &Reason{Code: RunNotCompleted}
	if err := completing.Validate(); err == nil {
		t.Fatal("completing goal with a stop reason was accepted")
	}
	if err := (Start{SessionID: "ses_1", Objective: "finish", Budget: Budget{MaxRuns: -1}}).Validate(); err == nil {
		t.Fatal("negative goal budget was accepted")
	}
}

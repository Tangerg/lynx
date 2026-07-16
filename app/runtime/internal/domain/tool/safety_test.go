package tool

import "testing"

func TestTaskIsSafeOrchestration(t *testing.T) {
	if got := SafetyClassFor("task"); got != SafetyClassSafe {
		t.Fatalf("SafetyClassFor(task) = %v, want safe", got)
	}
}

package admission

import "testing"

func TestGateHoldsSessionThroughRunMaintenance(t *testing.T) {
	var gate Gate
	releaseOpening, ok := gate.AcquireSession("ses_1")
	if !ok {
		t.Fatal("opening admission was rejected")
	}
	gate.OpenRun("run_1", "ses_1", "/repo")
	if got := gate.ActiveSessionWithCwd("/repo"); got != "ses_1" {
		t.Fatalf("active cwd session = %q, want ses_1", got)
	}

	releaseMaintenance, ok := gate.BeginMaintenance("run_1")
	if !ok {
		t.Fatal("terminal maintenance did not acquire the run")
	}
	releaseOpening()
	if !gate.ActiveSession("ses_1") {
		t.Fatal("opening release erased the maintenance claim")
	}
	if _, ok := gate.AcquireSession("ses_1"); ok {
		t.Fatal("new admission crossed the maintenance boundary")
	}
	if got := gate.ActiveSessionWithCwd("/repo"); got != "" {
		t.Fatalf("completed run still owns cwd for session %q", got)
	}

	releaseMaintenance()
	if gate.ActiveSession("ses_1") {
		t.Fatal("maintenance release left the session active")
	}
}

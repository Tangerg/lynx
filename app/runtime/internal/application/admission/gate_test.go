package admission

import "testing"

func TestGateHoldsSessionThroughRunMaintenance(t *testing.T) {
	var gate Gate
	releaseOpening, ok := gate.AcquireSession("ses_1")
	if !ok {
		t.Fatal("opening admission was rejected")
	}
	gate.OpenRun("run_1", "ses_1", "/repo")
	if _, ok := gate.AcquireWorkingTreeMutation("/repo"); ok {
		t.Fatal("live run did not block a working-tree mutation")
	}

	releaseMaintenance, ok := gate.BeginMaintenance("run_1")
	if !ok {
		t.Fatal("terminal maintenance did not acquire the run")
	}
	releaseOpening()
	if !gate.ActiveSessions()["ses_1"] {
		t.Fatal("opening release erased the maintenance claim")
	}
	if _, ok := gate.AcquireSession("ses_1"); ok {
		t.Fatal("new admission crossed the maintenance boundary")
	}
	mutationRelease, ok := gate.AcquireWorkingTreeMutation("/repo")
	if !ok {
		t.Fatal("completed run still owns the working tree")
	}
	mutationRelease()

	releaseMaintenance()
	if gate.ActiveSessions()["ses_1"] {
		t.Fatal("maintenance release left the session active")
	}
}

func TestGateExcludesWorkingTreeRunAdmissionsAndMutations(t *testing.T) {
	var gate Gate
	const cwd = "/repo"

	releaseFirst, ok := gate.AcquireWorkingTreeRun(cwd)
	if !ok {
		t.Fatal("first run admission was rejected")
	}
	releaseSecond, ok := gate.AcquireWorkingTreeRun(cwd)
	if !ok {
		t.Fatal("second run admission was rejected")
	}
	if _, ok := gate.AcquireWorkingTreeMutation(cwd); ok {
		t.Fatal("mutation admission crossed pending run admissions")
	}

	releaseFirst()
	releaseFirst()
	if _, ok := gate.AcquireWorkingTreeMutation(cwd); ok {
		t.Fatal("duplicate release consumed another run's admission")
	}
	releaseSecond()

	releaseMutation, ok := gate.AcquireWorkingTreeMutation(cwd)
	if !ok {
		t.Fatal("mutation admission was rejected after run admissions released")
	}
	if _, ok := gate.AcquireWorkingTreeRun(cwd); ok {
		t.Fatal("run admission crossed working-tree mutation")
	}
	releaseMutation()
	if _, ok := gate.AcquireWorkingTreeRun(""); !ok {
		t.Fatal("empty working tree must not require a claim")
	}
}

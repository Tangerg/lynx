package admission

import "testing"

func TestGateHoldsSessionThroughRunMaintenance(t *testing.T) {
	var gate Gate
	opening, ok := gate.AcquireRun("ses_1", "/repo")
	if !ok {
		t.Fatal("opening admission was rejected")
	}
	if !opening.Admit("run_1") {
		t.Fatal("opening admission did not become live")
	}
	if _, ok := gate.AcquireWorkingTreeMutation("/repo"); ok {
		t.Fatal("live run did not block a working-tree mutation")
	}

	releaseMaintenance, ok := gate.BeginMaintenance("run_1")
	if !ok {
		t.Fatal("terminal maintenance did not acquire the run")
	}
	if !gate.ActiveSessions()["ses_1"] {
		t.Fatal("maintenance release erased the session claim")
	}
	if _, ok := gate.AcquireSession("ses_1"); ok {
		t.Fatal("new admission crossed the maintenance boundary")
	}
	if _, ok := gate.AcquireWorkingTreeMutation("/repo"); ok {
		t.Fatal("terminal maintenance did not retain the working tree")
	}

	releaseMaintenance()
	if gate.ActiveSessions()["ses_1"] {
		t.Fatal("maintenance release left the session active")
	}
	mutationRelease, ok := gate.AcquireWorkingTreeMutation("/repo")
	if !ok {
		t.Fatal("maintenance release left the working tree busy")
	}
	mutationRelease()
}

func TestGateExcludesWorkingTreeRunAdmissionsAndMutations(t *testing.T) {
	var gate Gate
	const cwd = "/repo"

	first, ok := gate.AcquireRun("ses_1", cwd)
	if !ok {
		t.Fatal("first run admission was rejected")
	}
	second, ok := gate.AcquireRun("ses_2", cwd)
	if !ok {
		t.Fatal("second run admission was rejected")
	}
	if _, ok := gate.AcquireWorkingTreeMutation(cwd); ok {
		t.Fatal("mutation admission crossed pending run admissions")
	}

	first.Release()
	first.Release()
	if _, ok := gate.AcquireWorkingTreeMutation(cwd); ok {
		t.Fatal("duplicate release consumed another run's admission")
	}
	second.Release()

	releaseMutation, ok := gate.AcquireWorkingTreeMutation(cwd)
	if !ok {
		t.Fatal("mutation admission was rejected after run admissions released")
	}
	if _, ok := gate.AcquireRun("ses_3", cwd); ok {
		t.Fatal("run admission crossed working-tree mutation")
	}
	releaseMutation()
	if admission, ok := gate.AcquireRun("ses_3", ""); !ok {
		t.Fatal("empty working tree must not require a claim")
	} else {
		admission.Release()
	}
}

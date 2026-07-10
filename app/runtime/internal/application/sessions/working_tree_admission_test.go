package sessions

import "testing"

func TestWorkingTreeGateExcludesRunsAndMutations(t *testing.T) {
	var gate WorkingTreeGate
	const cwd = "/repo"

	runAdmission, ok := gate.ClaimRun(cwd)
	if !ok {
		t.Fatal("run admission must claim an idle cwd")
	}
	if _, ok := gate.ClaimMutation(cwd); ok {
		t.Fatal("mutation admission must wait for run admissions")
	}
	runAdmission.Release()

	mutationAdmission, ok := gate.ClaimMutation(cwd)
	if !ok {
		t.Fatal("mutation admission must claim an idle cwd")
	}
	if _, ok := gate.ClaimRun(cwd); ok {
		t.Fatal("run admission must wait for mutation admission")
	}
	mutationAdmission.Release()

	if _, ok := gate.ClaimRun(cwd); !ok {
		t.Fatal("run admission must claim again after mutation release")
	}
}

func TestWorkingTreeGateAllowsEmptyCwd(t *testing.T) {
	var gate WorkingTreeGate

	mutationAdmission, ok := gate.ClaimMutation("")
	if !ok {
		t.Fatal("empty cwd mutation must be admitted")
	}
	runAdmission, ok := gate.ClaimRun("")
	if !ok {
		t.Fatal("empty cwd run must be admitted")
	}
	mutationAdmission.Release()
	runAdmission.Release()
}

func TestWorkingTreeAdmissionReleaseIsIdempotentAcrossCopies(t *testing.T) {
	var gate WorkingTreeGate
	const cwd = "/repo"
	first, ok := gate.ClaimRun(cwd)
	if !ok {
		t.Fatal("first run admission rejected")
	}
	second, ok := gate.ClaimRun(cwd)
	if !ok {
		t.Fatal("second run admission rejected")
	}
	copyOfFirst := first
	first.Release()
	copyOfFirst.Release()
	if _, ok := gate.ClaimMutation(cwd); ok {
		t.Fatal("duplicate release consumed the second run admission")
	}
	second.Release()
	mutation, ok := gate.ClaimMutation(cwd)
	if !ok {
		t.Fatal("mutation rejected after both distinct admissions released")
	}
	mutation.Release()
}

package sessionadmission

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestGateHoldsSessionThroughMaintenance(t *testing.T) {
	var gate Gate
	opening, ok := gate.AcquireRun("ses_1", "/repo")
	if !ok {
		t.Fatal("opening admission was rejected")
	}
	if !opening.Admit("run_1") {
		t.Fatal("opening admission did not become live")
	}
	if _, mutationOk := gate.AcquireWorkingTreeMutation("/repo"); mutationOk {
		t.Fatal("live run did not block a working-tree mutation")
	}

	releaseMaintenance, ok := gate.BeginMaintenance("run_1")
	if !ok {
		t.Fatal("terminal maintenance did not acquire the run")
	}
	if !gate.ActiveSessions()["ses_1"] {
		t.Fatal("maintenance release erased the session claim")
	}
	if _, sessionOk := gate.AcquireSession("ses_1"); sessionOk {
		t.Fatal("new admission crossed the maintenance boundary")
	}
	if _, mutationOk := gate.AcquireWorkingTreeMutation("/repo"); mutationOk {
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
	if _, mutationOk := gate.AcquireWorkingTreeMutation(cwd); mutationOk {
		t.Fatal("mutation admission crossed pending run admissions")
	}

	first.Release()
	first.Release()
	if _, mutationOk := gate.AcquireWorkingTreeMutation(cwd); mutationOk {
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

func TestWaitRunStartableIncludesTerminalMaintenance(t *testing.T) {
	var gate Gate
	opening, ok := gate.AcquireRun("ses_1", "/repo")
	if !ok || !opening.Admit("run_1") {
		t.Fatal("admit run")
	}
	releaseMaintenance, ok := gate.BeginMaintenance("run_1")
	if !ok {
		t.Fatal("begin maintenance")
	}

	done := make(chan error, 1)
	go func() { done <- gate.WaitRunStartable(t.Context(), "ses_1", "/repo") }()
	select {
	case err := <-done:
		t.Fatalf("WaitRunStartable returned inside maintenance: %v", err)
	default:
	}
	releaseMaintenance()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("WaitRunStartable: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("WaitRunStartable did not observe maintenance release")
	}
}

func TestWaitRunStartableIncludesPendingRun(t *testing.T) {
	var gate Gate
	opening, ok := gate.AcquireRun("ses_1", "/repo")
	if !ok {
		t.Fatal("acquire pending Run")
	}

	done := make(chan error, 1)
	go func() { done <- gate.WaitRunStartable(t.Context(), "ses_1", "/repo") }()
	select {
	case err := <-done:
		t.Fatalf("WaitRunStartable returned while Run was pending: %v", err)
	default:
	}
	opening.Release()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("WaitRunStartable: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("WaitRunStartable did not observe pending Run release")
	}
}

func TestWaitRunStartableIncludesWorkingTreeMutation(t *testing.T) {
	var gate Gate
	release, ok := gate.AcquireWorkingTreeMutation("/repo")
	if !ok {
		t.Fatal("acquire working-tree mutation")
	}

	done := make(chan error, 1)
	go func() { done <- gate.WaitRunStartable(t.Context(), "ses_1", "/repo") }()
	select {
	case err := <-done:
		t.Fatalf("WaitRunStartable returned inside working-tree mutation: %v", err)
	default:
	}
	release()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("WaitRunStartable: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("WaitRunStartable did not observe working-tree mutation release")
	}
}

func TestWaitRunStartableIsContextBounded(t *testing.T) {
	var gate Gate
	release, ok := gate.AcquireSession("ses_1")
	if !ok {
		t.Fatal("acquire session")
	}
	defer release()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := gate.WaitRunStartable(ctx, "ses_1", "/repo"); !errors.Is(err, context.Canceled) {
		t.Fatalf("WaitRunStartable error = %v, want context canceled", err)
	}
}

package agentexec

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestChildRunAdmitterPreservesFirstClassChildIdentity(t *testing.T) {
	startedAt := time.Unix(10, 20)
	var admitted ChildProcess
	admitter := childRunAdmitter{admit: func(_ context.Context, child ChildProcess) error {
		admitted = child
		return nil
	}}
	process := observedProcess{
		id:          "process_child",
		parentID:    "process_root",
		spawnCallID: "call_delegate",
		startedAt:   startedAt,
	}

	if err := admitter.AdmitChild(t.Context(), process); err != nil {
		t.Fatalf("AdmitChild: %v", err)
	}
	if admitted.ProcessRef != processRef(process) || !admitted.StartedAt.Equal(startedAt) {
		t.Fatalf("admitted child = %+v, want exact process identity and start time", admitted)
	}
}

func TestChildRunAdmitterBypassesDirectRuntimeChild(t *testing.T) {
	called := false
	admitter := childRunAdmitter{admit: func(_ context.Context, _ ChildProcess) error {
		called = true
		return nil
	}}
	process := observedProcess{
		id:        "process_sdk_child",
		parentID:  "process_root",
		startedAt: time.Unix(10, 20),
	}

	if err := admitter.AdmitChild(t.Context(), process); err != nil {
		t.Fatalf("AdmitChild: %v", err)
	}
	if called {
		t.Fatal("direct Agent Runtime child crossed the application Run boundary")
	}
}

func TestChildRunAdmitterReturnsApplicationRejection(t *testing.T) {
	rejection := errors.New("child run rejected")
	admitter := childRunAdmitter{admit: func(_ context.Context, _ ChildProcess) error {
		return rejection
	}}
	process := observedProcess{
		id:          "process_child",
		parentID:    "process_root",
		spawnCallID: "call_delegate",
		startedAt:   time.Unix(10, 20),
	}

	if err := admitter.AdmitChild(t.Context(), process); !errors.Is(err, rejection) {
		t.Fatalf("AdmitChild error = %v, want application rejection", err)
	}
}

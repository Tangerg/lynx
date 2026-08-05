package runs

import (
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
)

func TestRegistryRemovesCompletedRun(t *testing.T) {
	var r registry
	started := time.Unix(42, 0).UTC()
	handle := &handle{}
	r.Open(Record{ID: "run_1", SessionID: "ses_1", CWD: "/repo", CreatedAt: started}, handle)

	e, ok := r.Get("run_1")
	if !ok || e.record.CreatedAt != started || e.handle != handle {
		t.Fatalf("entry = %+v, ok=%v", e, ok)
	}

	closed, ok := r.Remove("run_1")
	if !ok || closed.handle != handle {
		t.Fatalf("removed entry = %+v, ok=%v", closed, ok)
	}
	if _, ok := r.Get("run_1"); ok {
		t.Fatal("removed run remains live")
	}
}

func TestRegistryCancelReason(t *testing.T) {
	var r registry
	r.Open(Record{ID: "run_1", SessionID: "ses_1"}, nil)
	e, ok := r.MarkCancel("run_1", "user asked")
	if !ok {
		t.Fatal("mark cancel must find the run")
	}
	if e.record.CancelReason != "user asked" {
		t.Fatalf("cancel reason = %q", e.record.CancelReason)
	}
	if _, ok := r.MarkCancel("missing", "x"); ok {
		t.Fatal("mark cancel must miss unknown runs")
	}
}

func TestRegistryOwnsRunCapabilities(t *testing.T) {
	var registry registry
	capabilities := execution.RunCapabilities{
		InterruptKinds: []execution.InterruptKind{execution.ApprovalInterrupt},
	}
	registry.Open(Record{ID: "run_1", Capabilities: capabilities}, nil)
	capabilities.InterruptKinds[0] = execution.QuestionInterrupt

	first, ok := registry.Get("run_1")
	if !ok || first.record.Capabilities.InterruptKinds[0] != execution.ApprovalInterrupt {
		t.Fatalf("stored capabilities followed caller mutation: %+v", first.record.Capabilities)
	}
	first.record.Capabilities.InterruptKinds[0] = execution.QuestionInterrupt

	second, ok := registry.Get("run_1")
	if !ok || second.record.Capabilities.InterruptKinds[0] != execution.ApprovalInterrupt {
		t.Fatalf("Get leaked stored capabilities ownership: %+v", second.record.Capabilities)
	}
}

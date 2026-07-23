package runs

import (
	"slices"
	"testing"
	"time"
)

func TestRegistryListIsStableByCreationAndRunIdentity(t *testing.T) {
	var registry registry
	createdAt := time.Unix(42, 0).UTC()
	registry.Open(Record{ID: "run_b", CreatedAt: createdAt}, nil)
	registry.Open(Record{ID: "run_c", CreatedAt: createdAt.Add(time.Second)}, nil)
	registry.Open(Record{ID: "run_a", CreatedAt: createdAt}, nil)

	records := registry.List()
	ids := make([]string, len(records))
	for index, record := range records {
		ids[index] = record.ID
	}
	if want := []string{"run_a", "run_b", "run_c"}; !slices.Equal(ids, want) {
		t.Fatalf("List IDs = %v, want %v", ids, want)
	}
}

func TestRegistryRemovesCompletedRun(t *testing.T) {
	var r registry
	started := time.Unix(42, 0).UTC()
	handle := &handle{}
	r.Open(Record{ID: "run_1", SessionID: "ses_1", Cwd: "/repo", CreatedAt: started}, handle)

	e, ok := r.Get("run_1")
	if !ok || e.record.CreatedAt != started || e.handle != handle {
		t.Fatalf("entry = %+v, ok=%v", e, ok)
	}

	closed, ok := r.Remove("run_1")
	if !ok || closed.handle != handle {
		t.Fatalf("removed entry = %+v, ok=%v", closed, ok)
	}
	if r.Contains("run_1") {
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

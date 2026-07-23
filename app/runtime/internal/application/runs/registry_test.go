package runs

import (
	"slices"
	"testing"
	"time"
)

func TestRegistryListIsStableByCreationAndRunIdentity(t *testing.T) {
	var registry Registry[struct{}]
	createdAt := time.Unix(42, 0).UTC()
	registry.Open(Record{ID: "run_b", CreatedAt: createdAt}, struct{}{})
	registry.Open(Record{ID: "run_c", CreatedAt: createdAt.Add(time.Second)}, struct{}{})
	registry.Open(Record{ID: "run_a", CreatedAt: createdAt}, struct{}{})

	entries := registry.List()
	ids := make([]string, len(entries))
	for index, entry := range entries {
		ids[index] = entry.Record.ID
	}
	if want := []string{"run_a", "run_b", "run_c"}; !slices.Equal(ids, want) {
		t.Fatalf("List IDs = %v, want %v", ids, want)
	}
}

func TestRegistryRemovesCompletedRun(t *testing.T) {
	var r Registry[int]
	started := time.Unix(42, 0).UTC()
	r.Open(Record{ID: "run_1", SessionID: "ses_1", Cwd: "/repo", CreatedAt: started}, 7)

	e, ok := r.Get("run_1")
	if !ok || e.Record.CreatedAt != started || e.Payload != 7 {
		t.Fatalf("entry = %+v, ok=%v", e, ok)
	}

	closed, ok := r.Remove("run_1")
	if !ok || closed.Payload != 7 {
		t.Fatalf("removed entry = %+v, ok=%v", closed, ok)
	}
	if r.Contains("run_1") {
		t.Fatal("removed run remains live")
	}
}

func TestRegistryCancelReason(t *testing.T) {
	var r Registry[struct{}]
	r.Open(Record{ID: "run_1", SessionID: "ses_1"}, struct{}{})
	e, ok := r.MarkCancel("run_1", "user asked")
	if !ok {
		t.Fatal("mark cancel must find the run")
	}
	if e.Record.CancelReason != "user asked" {
		t.Fatalf("cancel reason = %q", e.Record.CancelReason)
	}
	if _, ok := r.MarkCancel("missing", "x"); ok {
		t.Fatal("mark cancel must miss unknown runs")
	}
}

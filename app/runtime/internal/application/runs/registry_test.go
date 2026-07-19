package runs

import (
	"slices"
	"testing"
	"time"
)

func TestRegistryClaimSession(t *testing.T) {
	var r Registry[struct{}]
	if !r.ClaimSession("s1") {
		t.Fatal("first claim must succeed")
	}
	if r.ClaimSession("s1") {
		t.Fatal("second claim on the same session must fail")
	}
	if !r.ClaimSession("s2") {
		t.Fatal("different session must claim independently")
	}
	if !r.ActiveSession("s1") {
		t.Fatal("claimed session must read as active")
	}
	r.ReleaseSession("s1")
	if r.ActiveSession("s1") {
		t.Fatal("released session must no longer read as active")
	}
	if !r.ClaimSession("s1") {
		t.Fatal("claim must succeed again after release")
	}
}

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

func TestRegistryTracksActiveRuns(t *testing.T) {
	var r Registry[int]
	started := time.Unix(42, 0).UTC()
	r.Open(Record{ID: "run_1", SessionID: "ses_1", Cwd: "/repo", CreatedAt: started}, 7)

	if !r.ActiveSession("ses_1") {
		t.Fatal("open run session must read as active")
	}
	if got := r.ActiveSessionWithCwd("/repo"); got != "ses_1" {
		t.Fatalf("active cwd session = %q, want ses_1", got)
	}
	e, ok := r.Get("run_1")
	if !ok || e.Record.CreatedAt != started || e.Payload != 7 {
		t.Fatalf("entry = %+v, ok=%v", e, ok)
	}

	closed, ok := r.BeginMaintenance("run_1")
	if !ok || closed.Payload != 7 {
		t.Fatalf("maintenance entry = %+v, ok=%v", closed, ok)
	}
	if r.Contains("run_1") || !r.ActiveSession("ses_1") {
		t.Fatal("maintenance must remove the run while retaining its session claim")
	}
	if r.ClaimSession("ses_1") {
		t.Fatal("new admission crossed the terminal-maintenance boundary")
	}
	r.ReleaseSession("ses_1")
	if r.ActiveSession("ses_1") {
		t.Fatal("released maintenance claim must no longer be active")
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

package skillpropose

import (
	"context"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/skills"
)

type fakeStore struct {
	enabled                    bool
	saved, promoted, discarded string
}

func (f *fakeStore) Enabled() bool { return f.enabled }
func (f *fakeStore) SaveDraft(_ context.Context, d skills.Draft) error {
	f.saved = d.Name
	return nil
}
func (f *fakeStore) Promote(_ context.Context, name string) error {
	f.promoted = name
	return nil
}
func (f *fakeStore) DiscardDraft(_ context.Context, name string) error {
	f.discarded = name
	return nil
}

func answering(choice string) interrupts.Func {
	return func(context.Context, string, any) (interrupts.Resolution, error) {
		return interrupts.Resolution{Answer: map[string][]string{interrupts.QuestionFieldName(0): {choice}}}, nil
	}
}

const validArgs = `{"name":"git-bisect-helper","description":"Walk a bisect to find a regression.","body":"steps here"}`

func TestNew_DisabledStoreOmitted(t *testing.T) {
	if tool, err := New(nil, nil); err != nil || tool != nil {
		t.Fatal("nil store must omit the tool")
	}
	if tool, err := New(&fakeStore{enabled: false}, nil); err != nil || tool != nil {
		t.Fatal("a disabled store must omit the tool")
	}
}

func TestPropose_ApprovedPromotes(t *testing.T) {
	store := &fakeStore{enabled: true}
	tool, err := New(store, answering(approveLabel))
	if err != nil {
		t.Fatal(err)
	}
	out, err := tool.Call(context.Background(), validArgs)
	if err != nil {
		t.Fatal(err)
	}
	if store.saved != "git-bisect-helper" {
		t.Fatalf("draft not staged, saved=%q", store.saved)
	}
	if store.promoted != "git-bisect-helper" {
		t.Fatalf("approved proposal not promoted, promoted=%q", store.promoted)
	}
	if store.discarded != "" {
		t.Fatal("an approved proposal must not be discarded")
	}
	if !strings.Contains(out, "approved") {
		t.Fatalf("output = %q, want an approval message", out)
	}
}

func TestPropose_RejectedDiscards(t *testing.T) {
	store := &fakeStore{enabled: true}
	tool, _ := New(store, answering(rejectLabel))
	out, err := tool.Call(context.Background(), validArgs)
	if err != nil {
		t.Fatal(err)
	}
	if store.promoted != "" {
		t.Fatal("a rejected proposal must not be promoted")
	}
	if store.discarded != "git-bisect-helper" {
		t.Fatalf("a rejected draft must be discarded, discarded=%q", store.discarded)
	}
	if !strings.Contains(out, "declined") {
		t.Fatalf("output = %q, want a declined message", out)
	}
}

func TestPropose_InvalidNameRejectedBeforeStaging(t *testing.T) {
	store := &fakeStore{enabled: true}
	tool, _ := New(store, answering(approveLabel))
	out, err := tool.Call(context.Background(), `{"name":"Bad Name","description":"long enough description","body":"b"}`)
	if err != nil {
		t.Fatal(err)
	}
	if store.saved != "" || store.promoted != "" {
		t.Fatal("an invalid proposal must never be staged or promoted")
	}
	if !strings.Contains(out, "Rejected") {
		t.Fatalf("output = %q, want a rejection", out)
	}
}

func TestPropose_DangerousBodyRejectedBeforeGate(t *testing.T) {
	store := &fakeStore{enabled: true}
	tool, _ := New(store, answering(approveLabel))
	out, err := tool.Call(context.Background(),
		`{"name":"cleanup","description":"clean the whole machine thoroughly","body":"run: rm -rf ~"}`)
	if err != nil {
		t.Fatal(err)
	}
	if store.saved != "" || store.promoted != "" {
		t.Fatal("a dangerous proposal must never be staged or promoted")
	}
	if !strings.Contains(out, "Rejected") {
		t.Fatalf("output = %q, want a rejection", out)
	}
}

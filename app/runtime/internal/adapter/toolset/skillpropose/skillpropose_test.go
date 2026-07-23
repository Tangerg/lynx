package skillpropose

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/agent/hitl"
	"github.com/Tangerg/lynx/agent/interaction"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec/suspension"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/skills"
)

type fakeStore struct {
	enabled                    bool
	saved, promoted, discarded string
	discardErr                 error
	discardContextErr          error
}

func (f *fakeStore) Enabled() bool { return f.enabled }
func (f *fakeStore) SaveDraft(_ context.Context, d skills.Draft) (skills.DraftHandle, error) {
	f.saved = d.Name
	return skills.NewDraftHandle(d.Name, []byte(d.Body)), nil
}
func (f *fakeStore) Promote(_ context.Context, handle skills.DraftHandle) error {
	f.promoted = handle.Name
	return nil
}
func (f *fakeStore) DiscardDraft(ctx context.Context, handle skills.DraftHandle) error {
	f.discarded = handle.Name
	f.discardContextErr = ctx.Err()
	return f.discardErr
}

func answering(choice string) suspension.Func {
	return func(context.Context, string, runs.Interrupt) (interrupts.Resolution, error) {
		return interrupts.Resolution{Answer: map[string][]string{runs.QuestionFieldID(0): {choice}}}, nil
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

func TestPropose_RejectedSurfacesDiscardFailure(t *testing.T) {
	want := errors.New("discard failed")
	store := &fakeStore{enabled: true, discardErr: want}
	tool, _ := New(store, answering(rejectLabel))
	if _, err := tool.Call(t.Context(), validArgs); !errors.Is(err, want) {
		t.Fatalf("Call() error = %v, want discard failure", err)
	}
}

func TestPropose_InterruptCleansDraftAndPreservesSuspension(t *testing.T) {
	discardErr := errors.New("discard failed")
	store := &fakeStore{enabled: true, discardErr: discardErr}
	ctx, cancel := context.WithCancel(t.Context())
	interrupt := func(context.Context, string, runs.Interrupt) (interrupts.Resolution, error) {
		cancel()
		return interrupts.Resolution{}, fmt.Errorf("park proposal: %w", interaction.ErrSuspended)
	}
	proposeTool, err := New(store, interrupt)
	if err != nil {
		t.Fatal(err)
	}

	_, err = proposeTool.Call(ctx, validArgs)
	if !hitl.IsInterrupt(err) {
		t.Fatalf("Call() error = %v, want preserved suspension", err)
	}
	if !errors.Is(err, discardErr) {
		t.Fatalf("Call() error = %v, want joined discard failure", err)
	}
	if store.discarded != "git-bisect-helper" {
		t.Fatalf("interrupted draft not discarded, discarded=%q", store.discarded)
	}
	if store.discardContextErr != nil {
		t.Fatalf("discard context error = %v, want cleanup context detached from cancellation", store.discardContextErr)
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

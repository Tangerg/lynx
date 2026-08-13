package sessiondeletion

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
	"github.com/Tangerg/lynx/app/cli/internal/retry"
	"github.com/Tangerg/lynx/app/cli/internal/workbench"
)

type deletionRuntimeStub struct {
	deleteErr   error
	readErr     error
	deletes     int
	reads       int
	afterDelete func()
}

func (runtime *deletionRuntimeStub) DeleteSession(context.Context, agent.DeleteSession) error {
	runtime.deletes++
	err := runtime.deleteErr
	if runtime.afterDelete != nil {
		runtime.afterDelete()
	}
	return err
}

func TestRecoverDoesNotReplayADeletionIntoAnotherRuntimeStore(t *testing.T) {
	store, err := workbench.Open(t.TempDir(), workbench.Config{})
	if err != nil {
		t.Fatal(err)
	}
	request := agent.DeleteSession{
		CommandID: "cli_77777777777777777777777777777777", SessionID: "ses_1",
	}
	if err := store.StageSessionDeletion(request, workbench.ReplayGuard{
		Namespace: "runtime-a", Until: time.Now().UTC().Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	runtime := new(deletionRuntimeStub)
	err = Recover(t.Context(), runtime, store, ReplayWindow{
		Namespace: "runtime-b", Retention: time.Hour,
	}, retry.Backoff{})
	if err == nil {
		t.Fatal("cross-store deletion recovery unexpectedly succeeded")
	}
	if runtime.deletes != 0 || runtime.reads != 0 {
		t.Fatalf("cross-store recovery performed deletes=%d reads=%d", runtime.deletes, runtime.reads)
	}
	if pending, found := store.PendingSessionDeletion(request.SessionID); !found || pending.CommandID != request.CommandID {
		t.Fatalf("preserved deletion = %+v, found %t", pending, found)
	}
}

func TestDeletionReplayGuaranteeExpiresAtItsDeadline(t *testing.T) {
	deadline := time.Date(2026, 8, 13, 10, 1, 0, 0, time.UTC)
	guard := workbench.ReplayGuard{Namespace: "runtime-a", Until: deadline}
	window := ReplayWindow{
		Namespace: "runtime-a", Retention: time.Minute,
		Now: func() time.Time { return deadline },
	}
	if replaySafe(guard, window) {
		t.Fatal("deletion replay remained safe at its retention deadline")
	}
}

func (runtime *deletionRuntimeStub) GetSession(context.Context, string) (agent.SessionSnapshot, error) {
	runtime.reads++
	return agent.SessionSnapshot{}, runtime.readErr
}

func TestRecoverRetiresAnExpiredDeletionProvenByTheOwningRuntime(t *testing.T) {
	store, err := workbench.Open(t.TempDir(), workbench.Config{})
	if err != nil {
		t.Fatal(err)
	}
	request := agent.DeleteSession{
		CommandID: "cli_99999999999999999999999999999999", SessionID: "ses_1",
	}
	deadline := time.Now().UTC().Add(-time.Second)
	if err := store.StageSessionDeletion(request, workbench.ReplayGuard{
		Namespace: "runtime-a", Until: deadline,
	}); err != nil {
		t.Fatal(err)
	}
	runtime := &deletionRuntimeStub{readErr: agent.ErrSessionNotFound}
	err = Recover(t.Context(), runtime, store, ReplayWindow{
		Namespace: "runtime-a", Retention: time.Hour,
	}, retry.Backoff{})
	if err != nil {
		t.Fatal(err)
	}
	if runtime.deletes != 0 || runtime.reads != 1 {
		t.Fatalf("settled recovery performed deletes=%d reads=%d", runtime.deletes, runtime.reads)
	}
	if pending, found := store.PendingSessionDeletion(request.SessionID); found {
		t.Fatalf("settled deletion remains durable: %+v", pending)
	}
}

func TestExecuteConfirmsAnExpiredDeletionProvenByTheOwningRuntime(t *testing.T) {
	store, err := workbench.Open(t.TempDir(), workbench.Config{})
	if err != nil {
		t.Fatal(err)
	}
	request := agent.DeleteSession{
		CommandID: "cli_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", SessionID: "ses_1",
	}
	if err := store.StageSessionDeletion(request, workbench.ReplayGuard{
		Namespace: "runtime-a", Until: time.Now().UTC().Add(-time.Second),
	}); err != nil {
		t.Fatal(err)
	}
	runtime := &deletionRuntimeStub{readErr: agent.ErrSessionNotFound}
	result, err := Execute(t.Context(), runtime, store, request.SessionID, ReplayWindow{
		Namespace: "runtime-a", Retention: time.Hour,
	}, retry.Backoff{})
	if err != nil || result.Outcome != Confirmed || result.Request != request {
		t.Fatalf("settlement = %+v, %v", result, err)
	}
	if runtime.deletes != 0 || runtime.reads != 1 {
		t.Fatalf("settled execution performed deletes=%d reads=%d", runtime.deletes, runtime.reads)
	}
}

func TestExecuteRejectsAnExpiredDeletionWhenTheSessionStillExists(t *testing.T) {
	store, err := workbench.Open(t.TempDir(), workbench.Config{})
	if err != nil {
		t.Fatal(err)
	}
	request := agent.DeleteSession{
		CommandID: "cli_abababababababababababababababab", SessionID: "ses_1",
	}
	if err := store.StageSessionDeletion(request, workbench.ReplayGuard{
		Namespace: "runtime-a", Until: time.Now().UTC().Add(-time.Second),
	}); err != nil {
		t.Fatal(err)
	}
	runtime := new(deletionRuntimeStub)
	result, err := Execute(t.Context(), runtime, store, request.SessionID, ReplayWindow{
		Namespace: "runtime-a", Retention: time.Hour,
	}, retry.Backoff{})
	if err != nil || result.Outcome != Rejected || result.Request != request {
		t.Fatalf("settlement = %+v, %v", result, err)
	}
	if runtime.deletes != 0 || runtime.reads != 1 {
		t.Fatalf("rejected execution performed deletes=%d reads=%d", runtime.deletes, runtime.reads)
	}
}

func TestSettlePreservesDeletionRejectedByAnotherRuntimeStore(t *testing.T) {
	runtime := &deletionRuntimeStub{deleteErr: agent.ErrCommandStoreMismatch}
	request := agent.DeleteSession{
		CommandID: "cli_66666666666666666666666666666666", SessionID: "ses_1",
	}
	deadline := time.Now().UTC().Add(time.Hour)
	window := ReplayWindow{Namespace: "runtime-a", Retention: time.Hour}
	outcome, err := Settle(t.Context(), runtime, request, workbench.ReplayGuard{
		Namespace: "runtime-a", Until: deadline,
	}, window, retry.Backoff{})
	if outcome != Unknown || !errors.Is(err, agent.ErrCommandStoreMismatch) {
		t.Fatalf("store mismatch settlement = outcome %v, error %v", outcome, err)
	}
	if runtime.reads != 0 {
		t.Fatalf("store mismatch consulted %d projections from the wrong store", runtime.reads)
	}
}

func TestRecoverRejectsAnUncommittedDeletionWhenReplayExpires(t *testing.T) {
	store, err := workbench.Open(t.TempDir(), workbench.Config{})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Date(2026, 8, 13, 10, 1, 0, 0, time.UTC)
	request := agent.DeleteSession{
		CommandID: "cli_88888888888888888888888888888888", SessionID: "ses_1",
	}
	if err := store.StageSessionDeletion(request, workbench.ReplayGuard{
		Namespace: "runtime-a", Until: deadline,
	}); err != nil {
		t.Fatal(err)
	}
	now := deadline.Add(-time.Nanosecond)
	window := ReplayWindow{
		Namespace: "runtime-a", Retention: time.Minute, Now: func() time.Time { return now },
	}
	runtime := &deletionRuntimeStub{deleteErr: agent.ErrDisconnected}
	runtime.afterDelete = func() {
		now = deadline
		runtime.deleteErr = nil
	}
	if err := Recover(t.Context(), runtime, store, window, retry.Backoff{}); err != nil {
		t.Fatal(err)
	}
	if runtime.deletes != 1 || runtime.reads != 1 {
		t.Fatalf("expired recovery performed deletes=%d reads=%d", runtime.deletes, runtime.reads)
	}
	if pending, found := store.PendingSessionDeletion(request.SessionID); found {
		t.Fatalf("rejected deletion remains durable: %+v", pending)
	}
}

func TestRecoverConvergesADeletionCommittedAsReplayExpires(t *testing.T) {
	store, err := workbench.Open(t.TempDir(), workbench.Config{})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Date(2026, 8, 13, 10, 1, 0, 0, time.UTC)
	request := agent.DeleteSession{
		CommandID: "cli_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", SessionID: "ses_1",
	}
	if err := store.StageSessionDeletion(request, workbench.ReplayGuard{
		Namespace: "runtime-a", Until: deadline,
	}); err != nil {
		t.Fatal(err)
	}
	now := deadline.Add(-time.Nanosecond)
	window := ReplayWindow{
		Namespace: "runtime-a", Retention: time.Minute, Now: func() time.Time { return now },
	}
	runtime := &deletionRuntimeStub{deleteErr: agent.ErrDisconnected}
	runtime.afterDelete = func() {
		now = deadline
		runtime.readErr = agent.ErrSessionNotFound
	}
	if err := Recover(t.Context(), runtime, store, window, retry.Backoff{}); err != nil {
		t.Fatal(err)
	}
	if runtime.deletes != 1 || runtime.reads != 1 {
		t.Fatalf("converged recovery performed deletes=%d reads=%d", runtime.deletes, runtime.reads)
	}
	if pending, found := store.PendingSessionDeletion(request.SessionID); found {
		t.Fatalf("converged deletion remains durable: %+v", pending)
	}
}

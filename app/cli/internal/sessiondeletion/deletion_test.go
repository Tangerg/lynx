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
	deleteErr error
	deletes   int
	reads     int
}

func (runtime *deletionRuntimeStub) DeleteSession(context.Context, agent.DeleteSession) error {
	runtime.deletes++
	return runtime.deleteErr
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

func (runtime *deletionRuntimeStub) GetSession(context.Context, string) (agent.SessionSnapshot, error) {
	runtime.reads++
	return agent.SessionSnapshot{}, nil
}

func TestSettlePreservesDeletionRejectedByAnotherRuntimeStore(t *testing.T) {
	runtime := &deletionRuntimeStub{deleteErr: agent.ErrCommandStoreMismatch}
	request := agent.DeleteSession{
		CommandID: "cli_66666666666666666666666666666666", SessionID: "ses_1",
	}
	outcome, err := Settle(t.Context(), runtime, request, retry.Backoff{})
	if outcome != Unknown || !errors.Is(err, agent.ErrCommandStoreMismatch) {
		t.Fatalf("store mismatch settlement = outcome %v, error %v", outcome, err)
	}
	if runtime.reads != 0 {
		t.Fatalf("store mismatch consulted %d projections from the wrong store", runtime.reads)
	}
}

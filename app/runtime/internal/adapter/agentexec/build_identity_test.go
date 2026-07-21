package agentexec

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Tangerg/lynx/agent/core"
	agentruntime "github.com/Tangerg/lynx/agent/runtime"
	"github.com/Tangerg/lynx/chatclient"
)

const (
	testBuildID      = "sha256:0000000000000000000000000000000000000000000000000000000000000000"
	alternateBuildID = "sha256:1111111111111111111111111111111111111111111111111111111111111111"
)

func TestNewRequiresContentBuildIdentityForDurableRuntime(t *testing.T) {
	client, err := chatclient.New(newStreamingStubModel("done"))
	if err != nil {
		t.Fatalf("chatclient.New: %v", err)
	}

	tests := []struct {
		name   string
		config Config
		want   string
	}{
		{
			name:   "missing",
			config: Config{ChatClient: client, ProcessStore: newJSONProcessStore()},
			want:   "BuildID",
		},
		{
			name:   "development fallback",
			config: Config{ChatClient: client, ProcessStore: newJSONProcessStore(), BuildID: "dev"},
			want:   "BuildID",
		},
		{
			name: "non fail process policy",
			config: Config{
				ChatClient:            client,
				ProcessStore:          newJSONProcessStore(),
				BuildID:               testBuildID,
				SnapshotFailurePolicy: agentruntime.SnapshotFailureReportOnly,
			},
			want: "SnapshotFailurePolicy",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := New(context.Background(), test.config)
			if err == nil {
				t.Fatalf("New error = nil, want %s rejection", test.want)
			}
			if !strings.Contains(err.Error(), test.want) {
				t.Fatalf("New error = %v, want detail %q", err, test.want)
			}
		})
	}
}

func TestRestoreTurnMissingSnapshotIsStateLoss(t *testing.T) {
	client, err := chatclient.New(newStreamingStubModel("done"))
	if err != nil {
		t.Fatalf("chatclient.New: %v", err)
	}
	engine, err := New(t.Context(), Config{
		ChatClient:   client,
		ProcessStore: newJSONProcessStore(),
		BuildID:      testBuildID,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	process, err := engine.RestoreTurn(t.Context(), "missing", RestoreTurnRequest{})
	if process != nil || !errors.Is(err, ErrProcessSnapshotLost) || !errors.Is(err, core.ErrSnapshotNotFound) {
		t.Fatalf("RestoreTurn = (%T, %v), want snapshot loss wrapping not found", process, err)
	}
}

func TestAutoSnapshotFailureRemainsExecutionFailure(t *testing.T) {
	client, err := chatclient.New(newStreamingStubModel("done"))
	if err != nil {
		t.Fatalf("chatclient.New: %v", err)
	}
	want := errors.New("snapshot unavailable")
	store := &failingProcessStore{err: want}
	engine, err := New(t.Context(), Config{
		ChatClient:   client,
		ProcessStore: store,
		BuildID:      testBuildID,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	process, err := engine.StartTurn(t.Context(), TurnRequest{Message: "hello"})
	if err != nil {
		t.Fatalf("StartTurn: %v", err)
	}
	doneErr := <-process.Done()
	if !errors.Is(doneErr, want) {
		t.Fatalf("Done error = %v, want snapshot failure", doneErr)
	}
	if process.Status() != core.StatusFailed {
		t.Fatalf("process status = %s, want failed", process.Status())
	}
	if store.saves.Load() == 0 {
		t.Fatal("process never attempted an automatic snapshot")
	}
	if errors.Is(doneErr, ErrProcessSnapshotLost) {
		t.Fatalf("active snapshot write failure was misclassified as restore loss: %v", doneErr)
	}
}

type failingProcessStore struct {
	saves atomic.Int32
	err   error
}

func (s *failingProcessStore) Save(context.Context, []core.ProcessSnapshot) error {
	s.saves.Add(1)
	return s.err
}

func (*failingProcessStore) Load(context.Context, string) (core.ProcessSnapshot, error) {
	return core.ProcessSnapshot{}, core.ErrSnapshotNotFound
}

func (*failingProcessStore) List(context.Context) ([]string, error) { return nil, nil }

func (*failingProcessStore) Delete(context.Context, string) error { return nil }

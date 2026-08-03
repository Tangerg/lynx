package agentexec

import (
	"context"
	"errors"
	"iter"
	"testing"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/toolset"
	"github.com/Tangerg/lynx/chatclient"
	"github.com/Tangerg/lynx/core/chat"
)

// refusingStubModel fails every model call, so the turn's action returns an
// error and the process ends in StatusFailed.
type refusingStubModel struct{ err error }

func (*refusingStubModel) DefaultOptions() chat.Options {
	return chat.Options{Model: "refusing-stub"}
}

func (m *refusingStubModel) Call(context.Context, *chat.Request) (*chat.Response, error) {
	return nil, m.err
}

func (m *refusingStubModel) Stream(context.Context, *chat.Request) iter.Seq2[*chat.Response, error] {
	return func(yield func(*chat.Response, error) bool) { yield(nil, m.err) }
}

// TestTurnCompletionCarriesTheProcessFailure pins where a completion's error
// comes from. A segment reports the process-domain failure separately from the
// error it hit while driving, and combining them is the framework's rule. A
// projection that reads only one field compiles, passes every other test, and
// silently turns a real model failure into "failed without an error" at the
// terminal planner.
func TestTurnCompletionCarriesTheProcessFailure(t *testing.T) {
	want := errors.New("model refused the request")
	client, err := chatclient.New(&refusingStubModel{err: want}, chatclient.Config{})
	if err != nil {
		t.Fatalf("chatclient.New: %v", err)
	}
	engine := mustEngineWith(t, client, toolset.BuildConfig{})
	defer engine.Close()

	process, err := engine.StartTurn(t.Context(), TurnRequest{SessionID: "session", Message: "go"})
	if err != nil {
		t.Fatalf("StartTurn: %v", err)
	}
	completion := process.Await()

	if completion.Status != core.StatusFailed {
		t.Fatalf("status = %s, want failed", completion.Status)
	}
	if !errors.Is(completion.Err, want) {
		t.Fatalf("completion error = %v, want the model failure", completion.Err)
	}
}

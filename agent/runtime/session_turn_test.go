package runtime

import (
	"context"
	"errors"
	goruntime "runtime"
	"testing"
	"time"

	"github.com/Tangerg/lynx/agent/core"
)

func TestLocalSessionTurnSequencerSerializesSameSession(t *testing.T) {
	sequencer := newLocalSessionTurnSequencer()
	releaseFirst, err := sequencer.Acquire(t.Context(), "session-1")
	if err != nil {
		t.Fatal(err)
	}

	waitContext, cancelWait := context.WithCancel(t.Context())
	waitResult := make(chan error, 1)
	go func() {
		_, err := sequencer.Acquire(waitContext, "session-1")
		waitResult <- err
	}()
	waitForTurnWaiter(t, sequencer, "session-1")
	select {
	case err := <-waitResult:
		t.Fatalf("same-session Acquire returned before release: %v", err)
	default:
	}
	cancelWait()

	if err := <-waitResult; !errors.Is(err, context.Canceled) {
		t.Fatalf("waiting Acquire error = %v, want context cancellation", err)
	}
	releaseFirst()
	releaseFirst() // The ownership callback is deliberately idempotent.
	if len(sequencer.gates) != 0 {
		t.Fatalf("retained gates = %d, want no idle per-session state", len(sequencer.gates))
	}
}

func waitForTurnWaiter(t *testing.T, sequencer *localSessionTurnSequencer, sessionID string) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for {
		sequencer.mu.Lock()
		gate := sequencer.gates[sessionID]
		waiting := gate != nil && gate.refs == 2
		sequencer.mu.Unlock()
		if waiting {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("same-session Acquire did not begin waiting")
		}
		goruntime.Gosched()
	}
}

func TestLocalSessionTurnSequencerAllowsDifferentSessions(t *testing.T) {
	sequencer := newLocalSessionTurnSequencer()
	releaseFirst, err := sequencer.Acquire(t.Context(), "session-1")
	if err != nil {
		t.Fatal(err)
	}
	defer releaseFirst()

	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	releaseSecond, err := sequencer.Acquire(ctx, "session-2")
	if err != nil {
		t.Fatalf("Acquire different session: %v", err)
	}
	releaseSecond()
}

type nilSessionTurnSequencer struct{}

func (*nilSessionTurnSequencer) Acquire(context.Context, string) (func(), error) {
	return func() {}, nil
}

func TestNewRejectsInvalidSessionTurnConfig(t *testing.T) {
	var typedNil *nilSessionTurnSequencer
	for _, test := range []struct {
		name   string
		config Config
	}{
		{name: "typed nil sequencer", config: Config{SessionTurnSequencer: typedNil}},
		{name: "negative finalize timeout", config: Config{SessionFinalizeTimeout: -time.Second}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := New(test.config); err == nil {
				t.Fatal("New succeeded, want validation error")
			}
		})
	}
}

type brokenSessionTurnSequencer struct{}

func (brokenSessionTurnSequencer) Acquire(context.Context, string) (func(), error) {
	return nil, nil
}

func TestRunInSessionRejectsNilTurnRelease(t *testing.T) {
	engine, err := New(Config{SessionTurnSequencer: brokenSessionTurnSequencer{}})
	if err != nil {
		t.Fatal(err)
	}
	session := core.Session{ID: "session-1"}
	if _, err := engine.RunInSession(t.Context(), nil, &session, core.Bindings{}, core.ProcessOptions{}); err == nil {
		t.Fatal("RunInSession succeeded, want sequencer contract error")
	}
}

package runtime

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"
)

func TestLocalSessionTurnSequencerSerializesSameSession(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
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
		synctest.Wait()
		select {
		case err := <-waitResult:
			t.Fatalf("same-session Acquire returned before release: %v", err)
		default:
		}
		cancelWait()
		synctest.Wait()

		if err := <-waitResult; !errors.Is(err, context.Canceled) {
			t.Fatalf("waiting Acquire error = %v, want context cancellation", err)
		}
		releaseFirst()
		releaseFirst() // The ownership callback is deliberately idempotent.
		if len(sequencer.gates) != 0 {
			t.Fatalf("retained gates = %d, want no idle per-session state", len(sequencer.gates))
		}
	})
}

func TestLocalSessionTurnSequencerGrantsWaitersInArrivalOrder(t *testing.T) {
	type ownership struct {
		index   int
		release func()
		err     error
	}

	synctest.Test(t, func(t *testing.T) {
		sequencer := newLocalSessionTurnSequencer()
		releaseFirst, err := sequencer.Acquire(t.Context(), "session-1")
		if err != nil {
			t.Fatal(err)
		}

		ownerships := make(chan ownership, 3)
		for index := range 3 {
			go func() {
				release, err := sequencer.Acquire(t.Context(), "session-1")
				ownerships <- ownership{index: index, release: release, err: err}
			}()
			// Define arrival order explicitly: each waiter reaches the blocked
			// state before the next goroutine starts.
			synctest.Wait()
		}

		releaseFirst()
		for expected := range 3 {
			synctest.Wait()
			got := <-ownerships
			if got.err != nil {
				t.Fatalf("waiter %d: %v", got.index, got.err)
			}
			if got.index != expected {
				t.Fatalf("granted waiter %d, want %d", got.index, expected)
			}
			got.release()
		}
		if len(sequencer.gates) != 0 {
			t.Fatalf("retained gates = %d, want no idle per-session state", len(sequencer.gates))
		}
	})
}

func TestLocalSessionTurnSequencerAllowsDifferentSessions(t *testing.T) {
	sequencer := newLocalSessionTurnSequencer()
	releaseFirst, err := sequencer.Acquire(t.Context(), "session-1")
	if err != nil {
		t.Fatal(err)
	}
	defer releaseFirst()

	releaseSecond, err := sequencer.Acquire(t.Context(), "session-2")
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
	if _, err := acquireSessionTurn(t.Context(), brokenSessionTurnSequencer{}, "session-1"); err == nil {
		t.Fatal("acquireSessionTurn succeeded, want sequencer contract error")
	}
}

type panickingSessionTurnSequencer struct {
	cause          error
	panicOnRelease bool
}

func (s panickingSessionTurnSequencer) Acquire(context.Context, string) (func(), error) {
	if !s.panicOnRelease {
		panic(s.cause)
	}
	return func() { panic(s.cause) }, nil
}

func TestSessionTurnSequencerPanicsBecomeErrors(t *testing.T) {
	cause := errors.New("sequencer panic")
	if _, err := acquireSessionTurn(t.Context(), panickingSessionTurnSequencer{cause: cause}, "session-1"); !errors.Is(err, cause) {
		t.Fatalf("Acquire error = %v, want panic cause", err)
	}

	release, err := acquireSessionTurn(t.Context(), panickingSessionTurnSequencer{cause: cause, panicOnRelease: true}, "session-1")
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	if err := releaseSessionTurn(release); !errors.Is(err, cause) {
		t.Fatalf("release error = %v, want panic cause", err)
	}
}

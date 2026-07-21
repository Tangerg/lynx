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
		releaseFirst, err := sequencer.acquire(t.Context(), "session-1")
		if err != nil {
			t.Fatal(err)
		}

		waitContext, cancelWait := context.WithCancel(t.Context())
		waitResult := make(chan error, 1)
		go func() {
			_, err := sequencer.acquire(waitContext, "session-1")
			waitResult <- err
		}()
		synctest.Wait()
		select {
		case err := <-waitResult:
			t.Fatalf("same-session acquire returned before release: %v", err)
		default:
		}
		cancelWait()
		synctest.Wait()

		if err := <-waitResult; !errors.Is(err, context.Canceled) {
			t.Fatalf("waiting acquire error = %v, want context cancellation", err)
		}
		releaseFirst()
		releaseFirst()
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
		releaseFirst, err := sequencer.acquire(t.Context(), "session-1")
		if err != nil {
			t.Fatal(err)
		}

		ownerships := make(chan ownership, 3)
		for index := range 3 {
			go func() {
				release, err := sequencer.acquire(t.Context(), "session-1")
				ownerships <- ownership{index: index, release: release, err: err}
			}()
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
	releaseFirst, err := sequencer.acquire(t.Context(), "session-1")
	if err != nil {
		t.Fatal(err)
	}
	defer releaseFirst()

	releaseSecond, err := sequencer.acquire(t.Context(), "session-2")
	if err != nil {
		t.Fatalf("acquire different session: %v", err)
	}
	releaseSecond()
}

func TestNewRejectsNegativeSessionFinalizeTimeout(t *testing.T) {
	if _, err := New(Config{SessionFinalizeTimeout: -time.Second}); err == nil {
		t.Fatal("New succeeded, want validation error")
	}
}

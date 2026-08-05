package runtime

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
)

func TestProcessTreeSequencerSerializesSameKey(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		sequencer := newProcessTreeSequencer()
		releaseFirst, err := sequencer.acquire(t.Context(), "tree-1")
		if err != nil {
			t.Fatal(err)
		}

		waitContext, cancelWait := context.WithCancel(t.Context())
		waitResult := make(chan error, 1)
		go func() {
			_, err := sequencer.acquire(waitContext, "tree-1")
			waitResult <- err
		}()
		synctest.Wait()
		select {
		case err := <-waitResult:
			t.Fatalf("same-tree acquire returned before release: %v", err)
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
			t.Fatalf("retained gates = %d, want no idle per-tree state", len(sequencer.gates))
		}
	})
}

func TestProcessTreeSequencerGrantsWaitersInArrivalOrder(t *testing.T) {
	type ownership struct {
		index   int
		release func()
		err     error
	}

	synctest.Test(t, func(t *testing.T) {
		sequencer := newProcessTreeSequencer()
		releaseFirst, err := sequencer.acquire(t.Context(), "tree-1")
		if err != nil {
			t.Fatal(err)
		}

		ownerships := make(chan ownership, 3)
		for index := range 3 {
			go func() {
				release, err := sequencer.acquire(t.Context(), "tree-1")
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
			t.Fatalf("retained gates = %d, want no idle per-tree state", len(sequencer.gates))
		}
	})
}

// TestProcessTreeSequencerReleaseCannotEvictTheNextOwner pins why releaseFunc wraps
// its release in sync.OnceFunc. Every mutation both defers its release and calls
// it early, so the release runs twice on every path. A second real release would
// find the queue empty — the waiter it just handed the key to is no longer in it
// — delete the gate, and admit the next caller while the current owner still
// believes it holds the key.
func TestProcessTreeSequencerReleaseCannotEvictTheNextOwner(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		sequencer := newProcessTreeSequencer()
		releaseFirst, err := sequencer.acquire(t.Context(), "tree-1")
		if err != nil {
			t.Fatal(err)
		}

		granted := make(chan func(), 1)
		go func() {
			release, err := sequencer.acquire(t.Context(), "tree-1")
			if err == nil {
				granted <- release
			}
		}()
		synctest.Wait()

		releaseFirst()
		synctest.Wait()
		releaseSecond := <-granted

		releaseFirst()

		third := make(chan func(), 1)
		go func() {
			release, err := sequencer.acquire(t.Context(), "tree-1")
			if err == nil {
				third <- release
			}
		}()
		synctest.Wait()
		select {
		case <-third:
			t.Fatal("second release evicted the current owner: a third caller acquired a held key")
		default:
		}

		releaseSecond()
		synctest.Wait()
		releaseThird := <-third
		releaseThird()
		if len(sequencer.gates) != 0 {
			t.Fatalf("retained gates = %d, want no idle per-tree state", len(sequencer.gates))
		}
	})
}

func TestProcessTreeSequencerAllowsDifferentKeys(t *testing.T) {
	sequencer := newProcessTreeSequencer()
	releaseFirst, err := sequencer.acquire(t.Context(), "tree-1")
	if err != nil {
		t.Fatal(err)
	}
	defer releaseFirst()

	releaseSecond, err := sequencer.acquire(t.Context(), "tree-2")
	if err != nil {
		t.Fatalf("acquire different tree: %v", err)
	}
	releaseSecond()
}

func TestProcessTreeSequencerRejectsCanceledIdleAcquire(t *testing.T) {
	sequencer := newProcessTreeSequencer()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	release, err := sequencer.acquire(ctx, "idle")
	if release != nil {
		t.Fatal("canceled acquire returned a release function")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("acquire error = %v, want context cancellation", err)
	}
	if len(sequencer.gates) != 0 {
		t.Fatalf("retained gates = %d, want no canceled acquisition state", len(sequencer.gates))
	}
}

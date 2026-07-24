package runs

import (
	"testing"
	"time"
)

// The tests in this file pin the Journal contract that the channel→iter.Seq
// migration must preserve: durable/terminal losslessness, the live-only drop
// policy, and — the load-bearing one — that an external cancel unblocks a
// subscriber currently waiting for its next event. They are transport-shape
// agnostic (they only range the subscription), so they survive the migration
// unchanged.

// TestJournalDurableLosslessLiveLossyUnderOverflow locks the drop policy: when a
// subscriber is flooded far past its buffering without draining, every durable
// event still arrives in order, while live-only events become a lossy but
// still-ordered subset. The exact surviving live count is deliberately NOT
// asserted — it is an implementation detail of the buffering (the migration
// intentionally collapses the incidental double buffer), whereas "durable never
// drops, live-only may" is the contract.
func TestJournalDurableLosslessLiveLossyUnderOverflow(t *testing.T) {
	j := NewJournal()
	ch, cancel := j.Subscribe("")
	defer cancel()

	// Flood without reading: far more live-only than any buffer can hold, with
	// durable events interleaved throughout.
	const liveTotal = liveHeadroom * 4
	durableSeqs := []int{1, liveTotal / 2, liveTotal + 1}
	appended := 0
	nextDurable := 0
	for i := 1; i <= liveTotal; i++ {
		if nextDurable < len(durableSeqs) && durableSeqs[nextDurable] == i {
			j.Append(ev(durableSeqs[nextDurable], true))
			nextDurable++
		}
		j.Append(ev(1_000_000+i, false)) // live-only
		appended++
	}
	for ; nextDurable < len(durableSeqs); nextDurable++ {
		j.Append(ev(durableSeqs[nextDurable], true))
	}
	j.Close()

	var gotDurable []string
	deliveredLive := 0
	for e := range ch {
		if e.Durable() {
			gotDurable = append(gotDurable, e.Seq)
			continue
		}
		deliveredLive++
	}

	wantDurable := []string{ev(durableSeqs[0], true).Seq, ev(durableSeqs[1], true).Seq, ev(durableSeqs[2], true).Seq}
	if len(gotDurable) != len(wantDurable) {
		t.Fatalf("durable delivered = %d, want %d (durable must be lossless)", len(gotDurable), len(wantDurable))
	}
	for i, seq := range gotDurable {
		if seq != wantDurable[i] {
			t.Fatalf("durable[%d] = %q, want %q (order must hold)", i, seq, wantDurable[i])
		}
	}
	if deliveredLive >= appended {
		t.Fatalf("live-only delivered = %d, want < %d (overflow must drop live-only)", deliveredLive, appended)
	}
}

// TestJournalCancelUnblocksWaitingSubscriber is the load-bearing guard for the
// iter.Seq migration: a subscriber ranging its subscription blocks waiting for
// the next event, and only an external cancel can end it — a consumer blocked
// inside the source cannot break out on its own. If a future refactor drops the
// cancel wakeup, this hangs and the timeout fails it.
func TestJournalCancelUnblocksWaitingSubscriber(t *testing.T) {
	j := NewJournal()
	ch, cancel := j.Subscribe("")

	done := make(chan struct{})
	go func() {
		for range ch { // no events will ever arrive; blocks in the source
		}
		close(done)
	}()

	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("cancel did not unblock a subscriber waiting for its next event")
	}
}

// BenchmarkJournalAppendDrain records the per-event append→deliver cost through
// one subscriber — the Batch 0 baseline for the migration's before/after
// comparison. Live-only events avoid unbounded durable retention.
func BenchmarkJournalAppendDrain(b *testing.B) {
	j := NewJournal()
	ch, cancel := j.Subscribe("")
	defer cancel()
	e := ev(1, false)
	for b.Loop() {
		j.Append(e)
		<-ch
	}
}

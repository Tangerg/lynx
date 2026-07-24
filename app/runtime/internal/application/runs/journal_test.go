package runs

import (
	"fmt"
	"iter"
	"sync"
	"testing"
)

func ev(seq int, durable bool) Event {
	payload := RunEvent(SegmentStarted{})
	if !durable {
		payload = SegmentProgressed{}
	}
	return Event{Seq: fmt.Sprintf("evt_%011d", seq), Payload: payload}
}

func drain(seq iter.Seq[Event]) []string {
	var ids []string
	for e := range seq {
		ids = append(ids, e.Seq)
	}
	return ids
}

// TestJournal_ReplayThenLive: a subscriber gets the durable backlog first, then
// live events, in order.
func TestJournal_ReplayThenLive(t *testing.T) {
	j := NewJournal()
	j.Append(ev(1, true))
	j.Append(ev(2, true))

	seq, cancel := j.Subscribe("")
	defer cancel()
	next, stop := iter.Pull(seq)
	defer stop()

	if got, _ := next(); got.Seq != ev(1, true).Seq {
		t.Fatalf("first = %s, want evt 1", got.Seq)
	}
	if got, _ := next(); got.Seq != ev(2, true).Seq {
		t.Fatalf("second = %s, want evt 2", got.Seq)
	}

	j.Append(ev(3, true)) // live
	if got, _ := next(); got.Seq != ev(3, true).Seq {
		t.Fatalf("live = %s, want evt 3", got.Seq)
	}
}

// TestJournal_SubscribeFromCursor: replay only the backlog strictly after the
// supplied cursor.
func TestJournal_SubscribeFromCursor(t *testing.T) {
	j := NewJournal()
	for i := 1; i <= 3; i++ {
		j.Append(ev(i, true))
	}
	seq, cancel := j.Subscribe(ev(2, true).Seq)
	defer cancel()
	j.Close() // no live; just drain replay

	got := drain(seq)
	if len(got) != 1 || got[0] != ev(3, true).Seq {
		t.Fatalf("replay = %v, want [evt 3]", got)
	}
}

// TestJournal_LiveOnlyNotReplayed: live-only (durable=false) events reach live
// subscribers but are never in a later subscriber's replay.
func TestJournal_LiveOnlyNotReplayed(t *testing.T) {
	j := NewJournal()

	live, cancelLive := j.Subscribe("")
	defer cancelLive()
	nextLive, stopLive := iter.Pull(live)
	defer stopLive()
	j.Append(ev(1, true))
	j.Append(ev(2, false)) // live-only
	if got, _ := nextLive(); got.Seq != ev(1, true).Seq {
		t.Fatal("live missing durable evt 1")
	}
	if got, _ := nextLive(); got.Seq != ev(2, false).Seq {
		t.Fatal("live missing live-only evt 2")
	}

	// A fresh subscriber replays durable only.
	late, cancelLate := j.Subscribe("")
	defer cancelLate()
	j.Close()
	if got := drain(late); len(got) != 1 || got[0] != ev(1, true).Seq {
		t.Fatalf("late replay = %v, want [evt 1] (no live-only)", got)
	}
}

// TestJournal_FanOutN: every subscriber receives each live event.
func TestJournal_FanOutN(t *testing.T) {
	j := NewJournal()
	a, ca := j.Subscribe("")
	defer ca()
	nextA, stopA := iter.Pull(a)
	defer stopA()
	b, cb := j.Subscribe("")
	defer cb()
	nextB, stopB := iter.Pull(b)
	defer stopB()

	j.Append(ev(1, true))
	if got, _ := nextA(); got.Seq != ev(1, true).Seq {
		t.Fatal("subscriber a must receive evt 1")
	}
	if got, _ := nextB(); got.Seq != ev(1, true).Seq {
		t.Fatal("subscriber b must receive evt 1")
	}
}

// TestJournal_CloseEndsStream: Close ends every subscriber stream, and a
// post-close Subscribe replays the backlog then ends.
func TestJournal_CloseEndsStream(t *testing.T) {
	j := NewJournal()
	j.Append(ev(1, true))
	seq, cancel := j.Subscribe("")
	defer cancel()
	next, stop := iter.Pull(seq)
	defer stop()
	if got, _ := next(); got.Seq != ev(1, true).Seq { // drain replay
		t.Fatalf("replay = %s, want evt 1", got.Seq)
	}

	j.Close()
	if _, ok := next(); ok {
		t.Fatal("stream must end on Journal Close")
	}

	post, cancelPost := j.Subscribe("")
	defer cancelPost()
	if got := drain(post); len(got) != 1 || got[0] != ev(1, true).Seq {
		t.Fatalf("post-close replay = %v, want [evt 1] then ended", got)
	}
}

// TestJournal_CancelDetaches: after cancel, the subscriber receives nothing and
// a later Append/Close neither panics nor delivers.
func TestJournal_CancelDetaches(t *testing.T) {
	j := NewJournal()
	seq, cancel := j.Subscribe("")
	cancel()
	if got := drain(seq); len(got) != 0 {
		t.Fatalf("cancel must end the stream, got %v", got)
	}
	j.Append(ev(1, true)) // must not panic (sub gone)
	j.Close()             // must not double-anything
}

func TestJournal_DurableOverflowIsLossless(t *testing.T) {
	j := NewJournal()
	seq, cancel := j.Subscribe("")
	defer cancel()
	const total = liveHeadroom*3 + 17
	for i := 1; i <= total; i++ {
		j.Append(ev(i, true))
	}
	j.Close()

	got := drain(seq)
	if len(got) != total {
		t.Fatalf("durable events = %d, want %d", len(got), total)
	}
	for i, cursor := range got {
		if want := ev(i+1, true).Seq; cursor != want {
			t.Fatalf("durable event[%d] = %q, want %q", i, cursor, want)
		}
	}
}

func TestJournalConcurrentAppendCloseAndCancel(t *testing.T) {
	j := NewJournal()
	seq, cancel := j.Subscribe("")
	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Go(func() {
		<-start
		for i := 1; i <= liveHeadroom*2; i++ {
			j.Append(ev(i, true))
		}
	})
	wg.Go(func() {
		<-start
		j.Close()
	})
	wg.Go(func() {
		<-start
		cancel()
	})
	close(start)
	wg.Wait()
	for range seq { // drain whatever survived the race; must terminate
	}
}

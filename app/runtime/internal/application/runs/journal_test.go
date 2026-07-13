package runs

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

// testEvent is a minimal [Streamable] for exercising the Journal without any wire
// type. Cursors are fixed-width zero-padded so lexical order == numeric order,
// matching the contract Event.Cursor documents.
type testEvent struct {
	cursor   string
	durable  bool
	terminal bool
}

func (e testEvent) Durable() bool  { return e.durable }
func (e testEvent) Terminal() bool { return e.terminal }
func (e testEvent) Cursor() string { return e.cursor }

func ev(seq int, durable bool) testEvent {
	return testEvent{cursor: fmt.Sprintf("evt_%011d", seq), durable: durable}
}

func drain(ch <-chan testEvent) []string {
	var ids []string
	for e := range ch {
		ids = append(ids, e.cursor)
	}
	return ids
}

// TestJournal_ReplayThenLive: a subscriber gets the durable backlog first, then
// live events, in order.
func TestJournal_ReplayThenLive(t *testing.T) {
	j := NewJournal[testEvent]()
	j.Append(ev(1, true))
	j.Append(ev(2, true))

	ch, cancel := j.Subscribe("")
	defer cancel()

	if got := <-ch; got.cursor != ev(1, true).cursor {
		t.Fatalf("first = %s, want evt 1", got.cursor)
	}
	if got := <-ch; got.cursor != ev(2, true).cursor {
		t.Fatalf("second = %s, want evt 2", got.cursor)
	}

	j.Append(ev(3, true)) // live
	if got := <-ch; got.cursor != ev(3, true).cursor {
		t.Fatalf("live = %s, want evt 3", got.cursor)
	}
}

// TestJournal_SubscribeFromCursor: replay only the backlog strictly after the
// supplied cursor.
func TestJournal_SubscribeFromCursor(t *testing.T) {
	j := NewJournal[testEvent]()
	for i := 1; i <= 3; i++ {
		j.Append(ev(i, true))
	}
	ch, cancel := j.Subscribe(ev(2, true).cursor)
	defer cancel()
	j.Close() // no live; just drain replay

	got := drain(ch)
	if len(got) != 1 || got[0] != ev(3, true).cursor {
		t.Fatalf("replay = %v, want [evt 3]", got)
	}
}

// TestJournal_LiveOnlyNotReplayed: live-only (durable=false) events reach live
// subscribers but are never in a later subscriber's replay.
func TestJournal_LiveOnlyNotReplayed(t *testing.T) {
	j := NewJournal[testEvent]()

	live, cancel := j.Subscribe("")
	defer cancel()
	j.Append(ev(1, true))
	j.Append(ev(2, false)) // live-only
	if (<-live).cursor != ev(1, true).cursor {
		t.Fatal("live missing durable evt 1")
	}
	if (<-live).cursor != ev(2, false).cursor {
		t.Fatal("live missing live-only evt 2")
	}

	// A fresh subscriber replays durable only.
	late, cancel2 := j.Subscribe("")
	defer cancel2()
	j.Close()
	if got := drain(late); len(got) != 1 || got[0] != ev(1, true).cursor {
		t.Fatalf("late replay = %v, want [evt 1] (no live-only)", got)
	}
}

// TestJournal_FanOutN: every subscriber receives each live event.
func TestJournal_FanOutN(t *testing.T) {
	j := NewJournal[testEvent]()
	a, ca := j.Subscribe("")
	defer ca()
	b, cb := j.Subscribe("")
	defer cb()

	j.Append(ev(1, true))
	if (<-a).cursor != ev(1, true).cursor || (<-b).cursor != ev(1, true).cursor {
		t.Fatal("both subscribers must receive evt 1")
	}
}

// TestJournal_CloseEndsStream: Close closes every subscriber channel, and a
// post-close Subscribe replays the backlog then closes.
func TestJournal_CloseEndsStream(t *testing.T) {
	j := NewJournal[testEvent]()
	j.Append(ev(1, true))
	ch, cancel := j.Subscribe("")
	defer cancel()
	<-ch // drain replay

	j.Close()
	if _, ok := <-ch; ok {
		t.Fatal("channel must close on Journal Close")
	}

	post, _ := j.Subscribe("")
	if got := drain(post); len(got) != 1 || got[0] != ev(1, true).cursor {
		t.Fatalf("post-close replay = %v, want [evt 1] then closed", got)
	}
}

// TestJournal_CancelDetaches: after cancel, the subscriber stops receiving and a
// later Close doesn't double-close its channel.
func TestJournal_CancelDetaches(t *testing.T) {
	j := NewJournal[testEvent]()
	ch, cancel := j.Subscribe("")
	cancel()
	if _, ok := <-ch; ok {
		t.Fatal("cancel must close the channel")
	}
	j.Append(ev(1, true)) // must not panic (sub gone)
	j.Close()             // must not double-close
}

func TestJournal_DurableOverflowIsLossless(t *testing.T) {
	j := NewJournal[testEvent]()
	ch, cancel := j.Subscribe("")
	defer cancel()
	const total = liveHeadroom*3 + 17
	for i := 1; i <= total; i++ {
		j.Append(ev(i, true))
	}
	j.Close()

	got := drain(ch)
	if len(got) != total {
		t.Fatalf("durable events = %d, want %d", len(got), total)
	}
	for i, cursor := range got {
		if want := ev(i+1, true).cursor; cursor != want {
			t.Fatalf("durable event[%d] = %q, want %q", i, cursor, want)
		}
	}
}

func TestJournalSubscriberTerminalDrainIsBounded(t *testing.T) {
	subscriber := newJournalSubscriber[testEvent](nil)
	for i := 1; i <= liveHeadroom*2; i++ {
		subscriber.enqueue(ev(i, true))
	}
	subscriber.enqueue(testEvent{cursor: "evt_99999999999", durable: true, terminal: true})

	started := time.Now()
	subscriber.finish(20 * time.Millisecond)
	<-subscriber.stopped
	if elapsed := time.Since(started); elapsed > 100*time.Millisecond {
		t.Fatalf("abandoned subscriber exceeded terminal drain budget: %v", elapsed)
	}
}

func TestJournalConcurrentAppendCloseAndCancel(t *testing.T) {
	j := NewJournal[testEvent]()
	ch, cancel := j.Subscribe("")
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
	for range ch {
	}
}

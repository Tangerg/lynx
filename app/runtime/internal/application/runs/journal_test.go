package runs

import (
	"errors"
	"iter"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/component/replaycursor"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
)

const (
	testEpoch     = "epoch_test"
	testRunID     = "run_1"
	testSegmentID = "seg_1"
)

func testJournal() *Journal {
	return newJournal(streamScope{Epoch: testEpoch, RunID: testRunID, SegmentID: testSegmentID}, DefaultRetention)
}

// ev builds a payload-only event. The Journal assigns its position, so a test
// never states a sequence: stating one is exactly the mistake the Journal now
// makes impossible.
func ev(replayable bool) Event {
	if replayable {
		return Event{RunID: testRunID, SegmentID: testSegmentID, Payload: SegmentStarted{}}
	}
	return Event{RunID: testRunID, SegmentID: testSegmentID, Payload: SegmentProgressed{}}
}

// sized builds a replayable event whose serialized payload is at least n bytes, so a
// byte-budget test measures the real charge rather than a fabricated one.
func sized(n int) Event {
	return Event{
		RunID: testRunID, SegmentID: testSegmentID,
		Payload: ItemCompleted{Item: transcript.Item{
			ID:      "item_1",
			Content: []transcript.ContentBlock{{Kind: transcript.TextContent, Text: strings.Repeat("x", n)}},
		}},
	}
}

func cursorAt(sequence uint64) string {
	return replaycursor.Encode(replaycursor.Position{
		Epoch: testEpoch, RunID: testRunID, SegmentID: testSegmentID, Sequence: sequence,
	})
}

func drain(seq iter.Seq[Event]) []uint64 {
	var got []uint64
	for e := range seq {
		got = append(got, e.Sequence)
	}
	return got
}

// A cursorless subscribe is TAIL-ONLY: history belongs to the transcript reads,
// and replaying it here would hand the client the same events twice.
func TestJournal_TailSkipsWhatWasAlreadyPublished(t *testing.T) {
	j := testJournal()
	j.Append(ev(true))
	j.Append(ev(true))

	attached := j.Tail()
	defer attached.Cancel()
	j.Append(ev(true)) // the only event this subscription may see
	j.Close()

	if got := drain(attached.Events); len(got) != 1 || got[0] != 3 {
		t.Fatalf("tail delivered %v, want only the event appended after attaching", got)
	}
}

// The head is captured with the attach, under one lock. Without that, an event
// published in between would be neither in the ack nor in the stream.
func TestJournal_TailReportsTheHeadItAttachedAfter(t *testing.T) {
	j := testJournal()
	if head := j.Tail().HeadCursor; head != "" {
		t.Fatalf("head of an empty stream = %q, want empty", head)
	}
	j.Append(ev(true))
	j.Append(ev(false)) // non-replayable events still take a position

	attached := j.Tail()
	defer attached.Cancel()
	if attached.HeadCursor != cursorAt(2) {
		t.Fatalf("head cursor does not name the last published position")
	}
}

func TestJournalTailFirstSnapshotConvergesAcrossTerminalBoundary(t *testing.T) {
	tests := []struct {
		name               string
		terminalBeforeTail bool
		terminalBeforeRead bool
	}{
		{name: "terminal committed before tail", terminalBeforeTail: true},
		{name: "terminal committed after tail before read", terminalBeforeRead: true},
		{name: "terminal committed after read"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			journal := testJournal()
			durableTerminal := false
			publishTerminal := func() {
				durableTerminal = true
				journal.Append(Event{
					RunID: testRunID, SegmentID: testSegmentID,
					Payload: SegmentFinished{},
				})
			}
			if tt.terminalBeforeTail {
				publishTerminal()
			}
			attached := journal.Tail()
			defer attached.Cancel()
			if tt.terminalBeforeRead {
				publishTerminal()
			}

			foldedTerminal := durableTerminal
			if !tt.terminalBeforeTail && !tt.terminalBeforeRead {
				publishTerminal()
			}
			journal.Close()
			for event := range attached.Events {
				if _, ok := event.Payload.(SegmentFinished); ok {
					foldedTerminal = true
				}
			}
			if !foldedTerminal {
				t.Fatal("tail-first snapshot lost the terminal boundary")
			}
		})
	}
}

func TestJournal_ReplayServesWhatFollowsTheCursorThenTails(t *testing.T) {
	j := testJournal()
	for range 3 {
		j.Append(ev(true))
	}

	attached, err := j.Replay(cursorAt(2))
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	defer attached.Cancel()
	j.Append(ev(true))
	j.Close()

	if got := drain(attached.Events); len(got) != 2 || got[0] != 3 || got[1] != 4 {
		t.Fatalf("replay delivered %v, want [3 4] (backlog then live)", got)
	}
}

// Ephemeral events take a position but are never retained, so a cursor pointing
// at one still resumes correctly — everything replayable after it is served.
func TestJournal_ReplayFromAnEphemeralPositionIsExact(t *testing.T) {
	j := testJournal()
	j.Append(ev(true))  // 1
	j.Append(ev(false)) // 2, not retained
	j.Append(ev(true))  // 3

	attached, err := j.Replay(cursorAt(2))
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	defer attached.Cancel()
	j.Close()
	if got := drain(attached.Events); len(got) != 1 || got[0] != 3 {
		t.Fatalf("replay delivered %v, want [3]", got)
	}
}

func TestJournal_ReplayRefusesCursorsItCannotServe(t *testing.T) {
	other := func(p replaycursor.Position) string { return replaycursor.Encode(p) }
	for name, test := range map[string]struct {
		cursor string
		want   error
	}{
		"damaged": {cursor: "!!!", want: ErrReplayCursorInvalid},
		"another run": {cursor: other(replaycursor.Position{
			Epoch: testEpoch, RunID: "run_other", SegmentID: testSegmentID, Sequence: 1,
		}), want: ErrReplayCursorInvalid},
		// The previous segment of the SAME run: the case a resume creates, and the one
		// a client is most likely to hold.
		"another segment": {cursor: other(replaycursor.Position{
			Epoch: testEpoch, RunID: testRunID, SegmentID: "seg_previous", Sequence: 1,
		}), want: ErrReplayCursorInvalid},
		"ahead of the head": {cursor: cursorAt(99), want: ErrReplayCursorInvalid},
		"another process": {cursor: other(replaycursor.Position{
			Epoch: "epoch_previous", RunID: testRunID, SegmentID: testSegmentID, Sequence: 1,
		}), want: ErrReplayUnavailable},
	} {
		t.Run(name, func(t *testing.T) {
			j := testJournal()
			j.Append(ev(true))
			if _, err := j.Replay(test.cursor); !errors.Is(err, test.want) {
				t.Fatalf("replay err = %v, want %v", err, test.want)
			}
		})
	}
}

// A cursor from a process that has restarted is unavailable rather than invalid,
// even when its run and segment are unknown here: the client did nothing wrong,
// and its remedy is a cold recovery rather than a corrected request.
func TestJournal_ForeignEpochOutranksAForeignScope(t *testing.T) {
	j := testJournal()
	j.Append(ev(true))
	stale := replaycursor.Encode(replaycursor.Position{
		Epoch: "epoch_previous", RunID: "run_other", SegmentID: "seg_other", Sequence: 1,
	})
	if _, err := j.Replay(stale); !errors.Is(err, ErrReplayUnavailable) {
		t.Fatalf("replay err = %v, want ErrReplayUnavailable", err)
	}
}

func TestJournal_EvictionBoundsTheWindowByCount(t *testing.T) {
	j := newJournal(streamScope{Epoch: testEpoch, RunID: testRunID, SegmentID: testSegmentID},
		Retention{MaxEvents: 2, MaxBytes: DefaultRetention.MaxBytes})
	for range 4 {
		j.Append(ev(true))
	}

	// Events 1 and 2 are gone, so a cursor before them cannot be served — that is
	// the difference between "nothing new for you" and "you have missed something".
	if _, err := j.Replay(cursorAt(1)); !errors.Is(err, ErrReplayUnavailable) {
		t.Fatalf("evicted cursor err = %v, want ErrReplayUnavailable", err)
	}
	attached, err := j.Replay(cursorAt(2))
	if err != nil {
		t.Fatalf("cursor at the eviction boundary: %v", err)
	}
	defer attached.Cancel()
	j.Close()
	if got := drain(attached.Events); len(got) != 2 || got[0] != 3 || got[1] != 4 {
		t.Fatalf("boundary replay = %v, want [3 4]", got)
	}
}

// The count budget alone does not bound memory: a handful of events can hold a
// multi-megabyte tool result each.
func TestJournal_EvictionBoundsTheWindowByBytes(t *testing.T) {
	const payload = 4096
	j := newJournal(streamScope{Epoch: testEpoch, RunID: testRunID, SegmentID: testSegmentID},
		Retention{MaxEvents: 1024, MaxBytes: payload * 2})
	for range 4 {
		j.Append(sized(payload))
	}

	j.mu.Lock()
	retained, bytes := len(j.retained), j.retainedBytes
	j.mu.Unlock()
	if retained >= 4 {
		t.Fatalf("retained %d events, want the byte budget to have evicted some", retained)
	}
	if bytes > payload*2 {
		t.Fatalf("retained bytes = %d, want at most %d", bytes, payload*2)
	}
	if _, err := j.Replay(cursorAt(1)); !errors.Is(err, ErrReplayUnavailable) {
		t.Fatalf("evicted cursor err = %v, want ErrReplayUnavailable", err)
	}
}

// TestJournalReplayableLosslessLiveLossyUnderOverflow locks the drop policy: a
// subscriber flooded past its buffering without draining still receives every
// replayable event in order, while live-only events become a lossy but
// still-ordered subset. The surviving live count is deliberately NOT asserted —
// buffer size is an implementation detail; "replayable never drops, live-only
// may" is the contract.
func TestJournalReplayableLosslessLiveLossyUnderOverflow(t *testing.T) {
	j := testJournal()
	attached := j.Tail()
	defer attached.Cancel()

	const liveTotal = liveHeadroom * 4
	for i := 1; i <= liveTotal; i++ {
		if i == 1 || i == liveTotal/2 {
			j.Append(ev(true))
		}
		j.Append(ev(false))
	}
	j.Append(ev(true))
	j.Close()

	var gotReplayable []uint64
	deliveredLive := 0
	for e := range attached.Events {
		if e.Replayable() {
			gotReplayable = append(gotReplayable, e.Sequence)
			continue
		}
		deliveredLive++
	}
	wantReplayable := []uint64{1, uint64(liveTotal/2 + 1), uint64(liveTotal + 3)}
	if len(gotReplayable) != len(wantReplayable) {
		t.Fatalf("replayable delivered = %v, want %v (replayable must be lossless)", gotReplayable, wantReplayable)
	}
	for i := range wantReplayable {
		if gotReplayable[i] != wantReplayable[i] {
			t.Fatalf("replayable[%d] = %d, want %d (order must hold)", i, gotReplayable[i], wantReplayable[i])
		}
	}
	if deliveredLive >= liveTotal {
		t.Fatalf("live-only delivered = %d, want < %d (overflow must drop live-only)", deliveredLive, liveTotal)
	}
}

// A replayable event is never dropped, so a consumer that stops draining is
// disconnected instead. Serving it a stream with a hole would leave it folding a
// state it could not tell was wrong.
func TestJournal_StalledAuthoritativeConsumerIsDisconnected(t *testing.T) {
	j := newJournal(streamScope{Epoch: testEpoch, RunID: testRunID, SegmentID: testSegmentID},
		Retention{MaxEvents: 3, MaxBytes: DefaultRetention.MaxBytes})
	attached := j.Tail()
	defer attached.Cancel()

	for range 5 {
		j.Append(ev(true))
	}
	got := drain(attached.Events)
	if len(got) >= 5 {
		t.Fatalf("stalled subscriber received %v, want the stream to have ended early", got)
	}
	// The run keeps going: a slow client must never stall the agent.
	j.Append(ev(true))
	j.mu.Lock()
	head := j.head
	j.mu.Unlock()
	if head != 6 {
		t.Fatalf("stream head = %d, want 6 (the run must keep publishing)", head)
	}
}

// TestJournalCancelUnblocksWaitingSubscriber guards the external cancellation
// contract: a consumer blocked inside the source cannot stop its own range.
func TestJournalCancelUnblocksWaitingSubscriber(t *testing.T) {
	j := testJournal()
	attached := j.Tail()

	done := make(chan struct{})
	go func() {
		for range attached.Events { // no events will ever arrive; blocks in the source
		}
		close(done)
	}()

	attached.Cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("cancel did not unblock a subscriber waiting for its next event")
	}
}

func TestJournal_LiveOnlyIsNeverReplayed(t *testing.T) {
	j := testJournal()
	live := j.Tail()
	defer live.Cancel()
	next, stop := iter.Pull(live.Events)
	defer stop()
	j.Append(ev(true))
	j.Append(ev(false))
	if got, _ := next(); got.Sequence != 1 {
		t.Fatal("live subscriber missed the replayable event")
	}
	if got, _ := next(); got.Sequence != 2 {
		t.Fatal("live subscriber missed the non-replayable event")
	}

	late, err := j.Replay(cursorAt(1))
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	defer late.Cancel()
	j.Close()
	if got := drain(late.Events); len(got) != 0 {
		t.Fatalf("replay = %v, want no non-replayable events", got)
	}
}

func TestJournal_FanOutN(t *testing.T) {
	j := testJournal()
	a := j.Tail()
	defer a.Cancel()
	nextA, stopA := iter.Pull(a.Events)
	defer stopA()
	b := j.Tail()
	defer b.Cancel()
	nextB, stopB := iter.Pull(b.Events)
	defer stopB()

	j.Append(ev(true))
	if got, _ := nextA(); got.Sequence != 1 {
		t.Fatal("subscriber a must receive the event")
	}
	if got, _ := nextB(); got.Sequence != 1 {
		t.Fatal("subscriber b must receive the event")
	}
}

func TestJournal_CloseEndsStream(t *testing.T) {
	j := testJournal()
	j.Append(ev(true))
	attached, err := j.Replay(cursorAt(1))
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	defer attached.Cancel()
	next, stop := iter.Pull(attached.Events)
	defer stop()

	j.Append(ev(true))
	if got, _ := next(); got.Sequence != 2 {
		t.Fatal("live event was not delivered")
	}
	j.Close()
	if _, ok := next(); ok {
		t.Fatal("stream must end on Journal Close")
	}

	// A subscribe that arrives after the segment ended still gets what it missed,
	// then ends — the window outlives the pump by the width of that race.
	post, err := j.Replay(cursorAt(1))
	if err != nil {
		t.Fatalf("post-close replay: %v", err)
	}
	if got := drain(post.Events); len(got) != 1 || got[0] != 2 {
		t.Fatalf("post-close replay = %v, want [2]", got)
	}
}

func TestJournal_CancelDetaches(t *testing.T) {
	j := testJournal()
	attached := j.Tail()
	attached.Cancel()
	if got := drain(attached.Events); len(got) != 0 {
		t.Fatalf("cancel must end the stream, got %v", got)
	}
	j.Append(ev(true)) // must not panic (sub gone)
	j.Close()          // must not double-anything
}

func TestJournal_EarlyRangeStopDetaches(t *testing.T) {
	j := testJournal()
	attached := j.Tail()
	j.Append(ev(true))

	for range attached.Events {
		break
	}

	j.mu.Lock()
	subscribers := len(j.subs)
	j.mu.Unlock()
	if subscribers != 0 {
		t.Fatalf("subscribers after range stop = %d, want 0", subscribers)
	}

	j.Append(ev(true)) // must not enqueue into an abandoned subscriber
	j.Close()
}

func TestJournalSubscriber_ReusesRoutineQueueAndReleasesBursts(t *testing.T) {
	subscriber := newJournalSubscriber(nil, DefaultRetention)
	subscriber.enqueue(ev(false), 0)
	if _, ok := subscriber.next(); !ok {
		t.Fatal("routine event was not delivered")
	}
	routineCapacity := cap(subscriber.queue)
	if routineCapacity == 0 {
		t.Fatal("routine queue capacity was not retained")
	}

	subscriber.enqueue(ev(false), 0)
	if _, ok := subscriber.next(); !ok {
		t.Fatal("reused queue event was not delivered")
	}
	if got := cap(subscriber.queue); got != routineCapacity {
		t.Fatalf("routine queue capacity = %d, want reused capacity %d", got, routineCapacity)
	}

	for range liveHeadroom * 2 {
		subscriber.enqueue(ev(true), 1)
	}
	for i := range liveHeadroom * 2 {
		if _, ok := subscriber.next(); !ok {
			t.Fatalf("replayable burst ended at event %d", i)
		}
	}
	if subscriber.queue != nil {
		t.Fatalf("oversized drained queue retained capacity %d", cap(subscriber.queue))
	}
}

func TestJournalSubscriber_AbortReleasesQueuedEvents(t *testing.T) {
	subscriber := newJournalSubscriber(nil, DefaultRetention)
	subscriber.enqueue(ev(true), 1)
	subscriber.abort()

	if subscriber.queue != nil {
		t.Fatalf("aborted queue retained capacity %d", cap(subscriber.queue))
	}
	if _, ok := subscriber.next(); ok {
		t.Fatal("aborted subscriber delivered a queued event")
	}
}

// A backlog within the window is lossless however far behind the consumer is.
func TestJournal_ReplayableBacklogWithinTheWindowIsLossless(t *testing.T) {
	j := testJournal()
	attached := j.Tail()
	defer attached.Cancel()
	const total = liveHeadroom*3 + 17
	for range total {
		j.Append(ev(true))
	}
	j.Close()

	got := drain(attached.Events)
	if len(got) != total {
		t.Fatalf("replayable events = %d, want %d", len(got), total)
	}
	for i, sequence := range got {
		if want := uint64(i + 1); sequence != want {
			t.Fatalf("event[%d] = %d, want %d", i, sequence, want)
		}
	}
}

func TestJournalConcurrentAppendCloseAndCancel(t *testing.T) {
	j := testJournal()
	attached := j.Tail()
	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Go(func() {
		<-start
		for range liveHeadroom * 2 {
			j.Append(ev(true))
		}
	})
	wg.Go(func() {
		<-start
		j.Close()
	})
	wg.Go(func() {
		<-start
		attached.Cancel()
	})
	close(start)
	wg.Wait()
	for range attached.Events { // drain whatever survived the race; must terminate
	}
}

// BenchmarkJournalAppendDrain records the steady-state per-event append→deliver
// cost through one subscriber. Live-only events avoid retention.
func BenchmarkJournalAppendDrain(b *testing.B) {
	j := testJournal()
	attached := j.Tail()
	defer attached.Cancel()
	next, stop := iter.Pull(attached.Events)
	defer stop()
	e := ev(false)
	for b.Loop() {
		j.Append(e)
		next()
	}
}

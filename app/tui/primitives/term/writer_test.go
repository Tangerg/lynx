package term

import (
	"bytes"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

// recorder collects everything written to it.
type recorder struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (r *recorder) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.b.Write(p)
}

func (r *recorder) String() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.b.String()
}

// blocker holds writes until it is released, standing in for a terminal that has
// stopped accepting bytes.
type blocker struct {
	release chan struct{}
	writes  chan []byte
}

func newBlocker() *blocker {
	return &blocker{release: make(chan struct{}), writes: make(chan []byte, 16)}
}

func (b *blocker) Write(p []byte) (int, error) {
	<-b.release
	b.writes <- append([]byte(nil), p...)
	return len(p), nil
}

// failing fails after letting a number of writes through.
type failing struct {
	mu    sync.Mutex
	ok    int
	calls int
}

var errBroken = errors.New("terminal went away")

func (f *failing) Write(p []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if f.ok > 0 {
		f.ok--
		return len(p), nil
	}
	return 0, errBroken
}

// short writes one byte at a time, which is what a terminal is allowed to do.
type short struct{ b bytes.Buffer }

func (s *short) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	return s.b.Write(p[:1])
}

func TestFramesReachTheTerminalInOrder(t *testing.T) {
	dst := &recorder{}
	w := NewWriter(dst)

	var last uint64
	for _, frame := range []string{"one", "two", "three"} {
		seq := w.Queue([]byte(frame))
		if seq != last+1 {
			t.Fatalf("sequence = %d, want %d", seq, last+1)
		}
		last = seq
	}
	if !w.Drain(time.Second) {
		t.Fatal("frames never drained")
	}
	if got := dst.String(); got != "onetwothree" {
		t.Fatalf("terminal received %q", got)
	}
	if got := w.Written(); got != 3 {
		t.Fatalf("watermark = %d, want 3", got)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestQueueDoesNotWaitForTheTerminal(t *testing.T) {
	dst := newBlocker()
	w := NewWriter(dst)
	defer dst.releaseAll()

	// The whole reason this type exists: a terminal that is not accepting bytes must
	// not stop the loop that draws.
	done := make(chan uint64, 1)
	go func() { done <- w.Queue([]byte("frame")) }()
	select {
	case seq := <-done:
		if seq != 1 {
			t.Fatalf("sequence = %d, want 1", seq)
		}
	case <-time.After(time.Second):
		t.Fatal("Queue blocked on a terminal that was not accepting bytes")
	}
	if got := w.Written(); got != 0 {
		t.Fatalf("watermark = %d, want nothing written yet", got)
	}
}

func (b *blocker) releaseAll() { close(b.release) }

func TestProgressWakesTheLoopWhenTheWatermarkMoves(t *testing.T) {
	dst := newBlocker()
	w := NewWriter(dst)

	seq := w.Queue([]byte("frame"))
	select {
	case <-w.Progress():
		t.Fatal("progress reported before anything was written")
	case <-time.After(20 * time.Millisecond):
	}

	dst.releaseAll()
	select {
	case <-w.Progress():
	case <-time.After(time.Second):
		t.Fatal("progress never reported")
	}
	if got := w.Written(); got != seq {
		t.Fatalf("watermark = %d, want %d", got, seq)
	}
}

func TestProgressCoalesces(t *testing.T) {
	// A loop that has not noticed the last advance does not need to be told twice,
	// and a bounded signal is what keeps a burst of frames from queueing wake-ups.
	w := NewWriter(&recorder{})
	for range 5 {
		w.Queue([]byte("x"))
	}
	w.Drain(time.Second)
	<-w.Progress()
	select {
	case <-w.Progress():
		t.Fatal("a second wake-up was queued for the same unobserved advance")
	case <-time.After(20 * time.Millisecond):
	}
}

func TestShortWritesAreCompleted(t *testing.T) {
	dst := &short{}
	w := NewWriter(dst)
	w.Queue([]byte("hello"))
	if !w.Drain(time.Second) {
		t.Fatal("never drained")
	}
	if got := dst.b.String(); got != "hello" {
		t.Fatalf("terminal received %q, want the whole frame", got)
	}
}

func TestAFailedWriteIsReportedAndStopsFurtherWrites(t *testing.T) {
	dst := &failing{ok: 1}
	w := NewWriter(dst)

	w.Queue([]byte("first"))
	w.Queue([]byte("second"))
	w.Queue([]byte("third"))
	w.Drain(time.Second)

	if err := w.Err(); !errors.Is(err, errBroken) {
		t.Fatalf("Err = %v, want the write failure", err)
	}
	// A terminal that has failed a write does not recover. Continuing would produce
	// the same error over and over, and a partly-written frame after it.
	if dst.calls > 2 {
		t.Fatalf("wrote %d times, want it to have stopped at the failure", dst.calls)
	}
	if got := w.Written(); got != 1 {
		t.Fatalf("watermark = %d, want only the frame that succeeded", got)
	}
}

func TestDrainCountsFailedFramesAsAccountedFor(t *testing.T) {
	// A broken terminal must not be able to wedge a shutdown.
	w := NewWriter(&failing{})
	w.Queue([]byte("doomed"))
	if !w.Drain(time.Second) {
		t.Fatal("Drain waited for a frame that had already failed")
	}
}

func TestDrainGivesUpOnATerminalThatNeverAccepts(t *testing.T) {
	dst := newBlocker()
	w := NewWriter(dst)
	defer dst.releaseAll()

	w.Queue([]byte("frame"))
	if w.Drain(30 * time.Millisecond) {
		t.Fatal("Drain claimed a frame landed that never did")
	}
}

func TestCloseReportsAbandonedFrames(t *testing.T) {
	dst := newBlocker()
	w := NewWriter(dst)
	defer dst.releaseAll()

	w.Queue([]byte("first"))
	w.Queue([]byte("second"))

	start := time.Now()
	err := w.Close()
	if err == nil {
		t.Fatal("Close hid the frames it abandoned")
	}
	if !errors.Is(err, ErrClosed) {
		t.Fatalf("Close error = %v, want it to be recognisable as ErrClosed", err)
	}
	// A write already inside the terminal cannot be interrupted, so Close returns on
	// its grace period rather than waiting for one that never comes back.
	if elapsed := time.Since(start); elapsed > 2*drainGrace {
		t.Fatalf("Close took %v, want it bounded by the grace period", elapsed)
	}
}

func TestCloseIsIdempotent(t *testing.T) {
	w := NewWriter(&recorder{})
	w.Queue([]byte("frame"))
	first := w.Close()
	second := w.Close()
	if first != nil || second != nil {
		t.Fatalf("Close returned %v then %v, want neither", first, second)
	}
}

func TestQueueAfterCloseIsRefusedRatherThanFatal(t *testing.T) {
	w := NewWriter(&recorder{})
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	// Shutting down while something still wanted to draw is ordinary, and must not
	// take the process down with it.
	seq := w.Queue([]byte("too late"))
	if seq == 0 {
		t.Fatal("a refused frame got no sequence")
	}
	if !w.Drain(time.Second) {
		t.Fatal("a refused frame was never accounted for")
	}
}

func TestManyProducersKeepSequenceAndWriteOrderTogether(t *testing.T) {
	// Sequence order and write order have to agree, or the watermark would release a
	// frame that has not landed.
	dst := &recorder{}
	w := NewWriter(dst)
	const frames = 50
	for i := range frames {
		w.Queue([]byte(strings.Repeat("x", i%7+1)))
	}
	if !w.Drain(2 * time.Second) {
		t.Fatal("never drained")
	}
	if got := w.Written(); got != frames {
		t.Fatalf("watermark = %d, want %d", got, frames)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

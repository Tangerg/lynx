package term

import (
	"errors"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"time"
)

// ErrClosed marks a frame that was handed over after the writer began shutting
// down, or that was still queued when its grace period ran out. Such a frame is
// abandoned rather than written.
var ErrClosed = errors.New("term: writer closed")

// DrainGrace is how long to wait for queued frames to reach the terminal before
// abandoning them. A terminal that has stopped accepting bytes must not be able to
// hold up an exit, and no amount of waiting makes one start accepting them again.
//
// It is what [Writer.Close] waits, and the right answer for anyone else with a
// reason to wait for the terminal to catch up.
const DrainGrace = 250 * time.Millisecond

// Writer writes frames to the terminal from a goroutine of its own.
//
// The reason it exists is that a terminal write can block for a long time — a
// remote session, a suspended emulator, a scrolled-back pager — and a UI loop that
// waits for one stops reading input, which is what an unresponsive terminal
// program actually is.
//
// Progress is reported as a watermark rather than as a stream of results: the only
// question anyone asks is how far the terminal has got, and a counter answers it
// without a queue to keep in order or a consumer to keep up. [Writer.Progress]
// wakes the loop when the watermark moves, and [Writer.Written] says where it is.
//
// Concurrency: [Writer.Queue], [Writer.Drain] and [Writer.Close] belong to the
// goroutine that owns the terminal. The writer's own goroutine is the only other
// participant, and the counters it publishes are read safely from anywhere.
type Writer struct {
	dst io.Writer

	frames chan frame
	// progress holds at most one pending wake-up: a loop that has not yet noticed
	// the last advance does not need to be told twice.
	progress chan struct{}

	queued    atomic.Uint64
	written   atomic.Uint64
	processed atomic.Uint64

	// discarding tells the write goroutine to fail frames instead of writing them,
	// once the grace period has passed.
	discarding atomic.Bool
	// closed guards the frame channel against a send after close. Owner-goroutine
	// state, like the frames it guards.
	closed bool

	mu      sync.Mutex
	failure error

	loopDone  chan struct{}
	closeOnce sync.Once
	closeErr  error
}

// frame is one queued payload with the sequence reserved for it.
type frame struct {
	seq  uint64
	data []byte
}

// NewWriter starts a writer over dst.
func NewWriter(dst io.Writer) *Writer {
	w := &Writer{
		dst:      dst,
		frames:   make(chan frame, 8),
		progress: make(chan struct{}, 1),
		loopDone: make(chan struct{}),
	}
	go w.run()
	return w
}

// Queue takes ownership of a frame and returns the sequence number reserved for
// it. The sequence is reserved before the goroutine can see the frame, so
// [Writer.Queued] already accounts for it when Queue returns.
//
// Queue returns without waiting unless the terminal has fallen further behind than
// the channel can hold. A frame handed over after [Writer.Close] is failed rather
// than written, which is not an error to the caller: shutting down while a frame
// was in flight is ordinary.
func (w *Writer) Queue(data []byte) uint64 {
	seq := w.queued.Add(1)
	if w.closed {
		w.finish(seq, ErrClosed)
		return seq
	}
	w.frames <- frame{seq: seq, data: data}
	return seq
}

// Progress wakes its receiver when the write watermark has moved. It carries no
// value: the watermark is read from [Writer.Written], which is always current.
func (w *Writer) Progress() <-chan struct{} { return w.progress }

// Queued is the highest sequence handed to the writer.
func (w *Writer) Queued() uint64 { return w.queued.Load() }

// Written is the highest sequence that reached the terminal.
func (w *Writer) Written() uint64 { return w.written.Load() }

// Err is the first write failure, or nil.
//
// A terminal that has failed a write does not recover, and a UI that cannot reach
// its terminal has nothing left to do, so this is a reason to exit rather than
// something to retry.
func (w *Writer) Err() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.failure
}

// Drain waits until every frame queued so far has been written or failed, or
// until the timeout passes, and reports whether everything was accounted for.
//
// It is what to call before handing the terminal to another program, so that
// program does not find half a frame in front of it. A failed frame counts as
// drained: a broken terminal must not be able to wedge a shutdown.
func (w *Writer) Drain(timeout time.Duration) bool {
	target := w.queued.Load()
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	for w.processed.Load() < target {
		select {
		case <-w.progress:
		case <-w.loopDone:
			return w.processed.Load() >= target
		case <-deadline.C:
			return false
		}
	}
	return true
}

// Close drains what it can, stops the goroutine and reports whether anything had
// to be abandoned. It is idempotent.
//
// A write already inside the terminal cannot be interrupted. When the grace period
// ends with one outstanding, Close returns without waiting for the goroutine: it
// finishes on its own, discarding what is left rather than writing it.
func (w *Writer) Close() error {
	w.closeOnce.Do(func() {
		drained := w.Drain(DrainGrace)
		w.discarding.Store(true)
		w.closed = true
		close(w.frames)
		if drained {
			<-w.loopDone
		} else {
			w.closeErr = fmt.Errorf("term: %d frame(s) never reached the terminal: %w",
				w.queued.Load()-w.written.Load(), ErrClosed)
		}
	})
	return w.closeErr
}

// run is the write goroutine.
func (w *Writer) run() {
	defer close(w.loopDone)
	for f := range w.frames {
		err := ErrClosed
		if !w.discarding.Load() {
			err = w.writeAll(f.data)
		}
		if err == nil {
			w.written.Store(f.seq)
		}
		w.finish(f.seq, err)
	}
}

// writeAll writes every byte, looping over short writes.
func (w *Writer) writeAll(data []byte) error {
	for len(data) > 0 {
		n, err := w.dst.Write(data)
		if err != nil {
			return err
		}
		if n <= 0 {
			return io.ErrShortWrite
		}
		data = data[n:]
	}
	return nil
}

// finish records a frame's outcome and wakes whoever is watching the watermark.
func (w *Writer) finish(seq uint64, err error) {
	if err != nil && !errors.Is(err, ErrClosed) {
		w.mu.Lock()
		if w.failure == nil {
			w.failure = err
		}
		w.mu.Unlock()
		// A failed terminal stays failed. Continuing to write would produce a
		// stream of the same error and, worse, a partly-written frame after it.
		w.discarding.Store(true)
	}
	for {
		if seen := w.processed.Load(); seen >= seq || w.processed.CompareAndSwap(seen, seq) {
			break
		}
	}
	select {
	case w.progress <- struct{}{}:
	default:
	}
}

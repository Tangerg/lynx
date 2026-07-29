package runs

import (
	"fmt"
	"iter"
	"runtime"
	"testing"
)

// What the replay window actually costs (Batch D1).
//
// The window's budget is a published number: discovery tells a client it may
// replay up to Retention.MaxEvents / MaxBytes, and the client chooses replay over
// a cold read on the strength of it. Until now the number was argued in a comment
// and asserted by a unit test that only checked eviction arithmetic — nothing
// measured what the budget buys or what it charges the process.
//
// Two costs are separate and both matter:
//
//   - HEAP. The budget counts SERIALIZED payload length, but the window retains
//     live Go values. The two are not the same number, so "16 MiB of replay" is a
//     claim about bytes on the wire, not about the resident set. This file pins
//     the ratio between them so a payload shape whose in-memory footprint runs
//     away from its JSON length cannot do so unnoticed.
//   - CPU. Charging an event means serializing it, and that happens under the
//     journal's own lock — the same lock Append holds to assign positions and fan
//     out. A multi-megabyte tool result therefore pays a full marshal inside the
//     critical section of the run's only stream.
//
// These are benchmarks and one measurement test, not thresholds: a number that
// fails CI on a slower machine teaches everyone to ignore it. The test asserts
// only the invariant (the ratio stays within an order of magnitude); the
// benchmarks report, and `go test -bench` is where a tuning decision reads them.

// heapInUse settles the allocator and reports the bytes it is holding. Two GC
// cycles because the first can leave finalizable garbage the second reclaims.
func heapInUse() uint64 {
	runtime.GC()
	runtime.GC()
	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)
	return stats.HeapAlloc
}

// TestRetentionByteBudgetTracksHeap measures what a FULL window at the advertised
// byte budget actually holds, and fails if serialized length has stopped
// predicting it.
//
// The budget is the only thing bounding the window's memory, so it has to remain
// a usable proxy for the memory. A shape that serialized to 1 byte and retained a
// megabyte would satisfy every existing test while making the advertised number
// meaningless.
func TestRetentionByteBudgetTracksHeap(t *testing.T) {
	// A small window with the real event shape: the ratio is a property of the
	// payload, not of the budget's size, and a 16 MiB fill would make the test's own
	// allocations the thing being measured.
	const (
		payload   = 64 << 10
		maxBytes  = 4 << 20
		maxEvents = 4096
	)
	retention := Retention{MaxEvents: maxEvents, MaxBytes: maxBytes}

	before := heapInUse()
	j := newJournal(streamScope{Epoch: testEpoch, RunID: testRunID, SegmentID: testSegmentID}, retention)
	// Overfill deliberately: the window must be at its budget, not below it, and
	// eviction is what puts it there.
	for range (maxBytes / payload) * 2 {
		j.Append(sized(payload))
	}
	after := heapInUse()

	j.mu.Lock()
	charged := j.retainedBytes
	events := len(j.retained)
	j.mu.Unlock()

	if charged > maxBytes {
		t.Fatalf("window charged %d bytes, over its own budget of %d", charged, maxBytes)
	}
	// Held can read below charged when the allocator has not returned pages, and
	// that is not a failure — the budget over-charging is safe. Only the other
	// direction breaks the promise.
	held := int64(after) - int64(before)
	t.Logf("window: %d events, charged %d bytes, heap held %d bytes (%.2fx charged)",
		events, charged, held, float64(held)/float64(charged))
	if held > int64(charged)*10 {
		t.Fatalf("heap held %d bytes for a %d-byte window: serialized length no longer "+
			"predicts the window's memory, so the advertised budget bounds nothing",
			held, charged)
	}
}

// BenchmarkJournalAppendReplayable is what one replayable event costs to publish,
// by payload size. The existing BenchmarkJournalAppendDrain publishes a
// non-replayable event, which is never charged and so never serialized — it
// measures the fan-out and none of the retention.
func BenchmarkJournalAppendReplayable(b *testing.B) {
	sizes := []struct {
		name  string
		bytes int
	}{
		{"item_512B", 512},
		{"tool_result_64KiB", 64 << 10},
		{"tool_result_1MiB", 1 << 20},
	}
	for _, size := range sizes {
		b.Run(size.name, func(b *testing.B) {
			j := testJournal()
			attached := j.Tail()
			defer attached.Cancel()
			next, stop := iter.Pull(attached.Events)
			defer stop()
			e := sized(size.bytes)
			b.SetBytes(int64(size.bytes))
			b.ReportAllocs()
			for b.Loop() {
				j.Append(e)
				next()
			}
		})
	}
}

// BenchmarkJournalReplayAttach is what one reconnect costs: the backlog is copied
// and then charged again, event by event, on the attaching goroutine.
//
// This is the cost a reconnect storm multiplies — every client that drops and
// comes back pays it — and it is the one place the window's size turns directly
// into work rather than into residency.
func BenchmarkJournalReplayAttach(b *testing.B) {
	const payload = 64 << 10
	for _, depth := range []int{16, 64} {
		b.Run(fmt.Sprintf("backlog_%d", depth), func(b *testing.B) {
			j := testJournal()
			for range depth {
				j.Append(sized(payload))
			}
			// Resume from the oldest event the window still holds, not from a fixed
			// sequence: at these payload sizes the byte budget decides how deep the
			// backlog really is, and a hardcoded origin would fall out of the window
			// the moment either number moves.
			j.mu.Lock()
			oldest := j.retained[0].event.Sequence
			retained := len(j.retained)
			j.mu.Unlock()
			from := cursorAt(oldest)

			// No SetBytes: an attach is not a throughput. It once re-serialized the
			// whole backlog, and a MB/s column made that look like work being done
			// rather than work being repeated. B/op is the number that mattered.
			b.Logf("backlog: %d events, %d bytes", retained-1, (retained-1)*payload)
			b.ReportAllocs()
			for b.Loop() {
				attached, err := j.Replay(from)
				if err != nil {
					b.Fatalf("Replay: %v", err)
				}
				attached.Cancel()
			}
		})
	}
}

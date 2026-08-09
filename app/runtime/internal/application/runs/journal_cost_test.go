package runs

import (
	"fmt"
	"iter"
	"runtime"
	"testing"
)

// These measurements keep the replay window's retained-memory estimate honest.
// Benchmarks report accounting cost without turning machine speed into a CI
// threshold; the test only rejects a representation whose heap grows far beyond
// its charge.

// heapInUse settles the allocator and reports the bytes it is holding. Two GC
// cycles because the first can leave finalizable garbage the second reclaims.
func heapInUse() uint64 {
	runtime.GC()
	runtime.GC()
	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)
	return stats.HeapAlloc
}

// TestRetentionByteBudgetTracksHeap measures what a full window at the
// advertised byte budget actually holds.
//
// The budget is the only thing bounding the window's memory, so it has to remain
// a usable proxy for the memory. A shape that serialized to 1 byte and retained a
// megabyte would satisfy every existing test while making the advertised number
// meaningless.
func TestRetentionByteBudgetTracksHeap(t *testing.T) {
	// A small window with the real event shape avoids making the test's own
	// allocations the dominant measurement.
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
		j.append(sized(payload))
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
		t.Fatalf("heap held %d bytes for a %d-byte window: retention accounting "+
			"no longer predicts the window's memory",
			held, charged)
	}
}

// BenchmarkJournalAppendReplayable is what one replayable event costs to publish,
// by payload size. The non-replayable append benchmark measures fan-out without
// retention accounting.
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
			attached := j.tail()
			defer attached.Cancel()
			next, stop := iter.Pull(attached.Events)
			defer stop()
			e := sized(size.bytes)
			b.SetBytes(int64(size.bytes))
			b.ReportAllocs()
			for b.Loop() {
				j.append(e)
				next()
			}
		})
	}
}

// BenchmarkJournalReplayAttach is what one reconnect costs when the already
// charged backlog is copied into a subscriber queue.
func BenchmarkJournalReplayAttach(b *testing.B) {
	const payload = 64 << 10
	for _, depth := range []int{16, 64} {
		b.Run(fmt.Sprintf("backlog_%d", depth), func(b *testing.B) {
			j := testJournal()
			for range depth {
				j.append(sized(payload))
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

			// No SetBytes: an attach is not throughput. B/op is the useful signal.
			b.Logf("backlog: %d events, %d bytes", retained-1, (retained-1)*payload)
			b.ReportAllocs()
			for b.Loop() {
				attached, err := j.replay(from)
				if err != nil {
					b.Fatalf("Replay: %v", err)
				}
				attached.Cancel()
			}
		})
	}
}

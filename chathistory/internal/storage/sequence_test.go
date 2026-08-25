package storage

import (
	"sync"
	"testing"
)

func TestSequenceReservesContiguousRanges(t *testing.T) {
	var sequence Sequence

	if first := sequence.reserveAt(100, 3); first != 100 {
		t.Fatalf("first reservation = %d, want 100", first)
	}
	if first := sequence.reserveAt(99, 2); first != 103 {
		t.Fatalf("reservation after clock rollback = %d, want 103", first)
	}
}

func TestSequenceReserveIsConcurrentSafe(t *testing.T) {
	var sequence Sequence
	const workers = 32
	values := make(chan int64, workers)
	var group sync.WaitGroup
	for range workers {
		group.Go(func() {
			values <- sequence.Reserve(1)
		})
	}
	group.Wait()
	close(values)

	seen := make(map[int64]struct{}, workers)
	for value := range values {
		if _, exists := seen[value]; exists {
			t.Fatalf("duplicate reservation %d", value)
		}
		seen[value] = struct{}{}
	}
}

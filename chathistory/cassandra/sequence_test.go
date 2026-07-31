package cassandra

import "testing"

func TestSequenceUUIDPreservesBatchOrder(t *testing.T) {
	const base = int64(1_700_000_000_000_000_000)
	previous := sequenceUUID(base, 0).Time()
	for index := 1; index < 10; index++ {
		current := sequenceUUID(base, index).Time()
		if !current.After(previous) {
			t.Fatalf("sequence %d time = %v, previous = %v", index, current, previous)
		}
		previous = current
	}
}

package terminal

import (
	"testing"
	"time"
)

func TestActiveDurationClockStartsFromDurableExecutionTime(t *testing.T) {
	startedAt := time.Date(2026, 8, 11, 10, 0, 0, 0, time.UTC)
	clock := activeDurationClock{}
	if got := clock.elapsed(startedAt); got != 0 {
		t.Fatalf("zero clock elapsed = %v, want zero", got)
	}

	clock.start(1400*time.Millisecond, startedAt)
	if got := clock.elapsed(startedAt.Add(600 * time.Millisecond)); got != 2*time.Second {
		t.Fatalf("resumed elapsed = %v, want 2s", got)
	}
	if got := clock.elapsed(startedAt.Add(-time.Second)); got != 1400*time.Millisecond {
		t.Fatalf("clock before local segment start = %v, want carried duration", got)
	}
}

func TestActiveDurationClockExcludesHumanWaitBetweenSegments(t *testing.T) {
	firstSegment := time.Date(2026, 8, 11, 10, 0, 0, 0, time.UTC)
	resume := firstSegment.Add(24 * time.Hour)
	clock := activeDurationClock{}
	clock.start(0, firstSegment)
	clock.start(3*time.Second, resume)

	if got := clock.elapsed(resume.Add(time.Second)); got != 4*time.Second {
		t.Fatalf("elapsed after overnight wait = %v, want 4s active time", got)
	}
}

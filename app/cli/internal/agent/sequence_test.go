package agent

import (
	"errors"
	"testing"
)

func TestEventSequenceDeduplicatesAndCommitsTransactionally(t *testing.T) {
	sequence := NewEventSequence(9)
	envelope := Envelope{ID: "event_10", Cursor: 10}
	applied := 0
	result, err := sequence.Accept(envelope, func() error { applied++; return nil })
	if err != nil || !result.Applied || applied != 1 || sequence.Cursor() != 10 {
		t.Fatalf("first accept = %+v, %v; applied=%d cursor=%d", result, err, applied, sequence.Cursor())
	}
	result, err = sequence.Accept(envelope, func() error { applied++; return nil })
	if err != nil || result.Applied || applied != 1 {
		t.Fatalf("duplicate = %+v, %v; applied=%d", result, err, applied)
	}

	failed := Envelope{ID: "event_11", Cursor: 11}
	want := errors.New("fold failed")
	if _, err := sequence.Accept(failed, func() error { return want }); !errors.Is(err, want) {
		t.Fatalf("apply error = %v", err)
	}
	if sequence.Cursor() != 10 {
		t.Fatalf("failed callback advanced cursor to %d", sequence.Cursor())
	}
}

func TestEventSequenceRejectsGapConflictAndUnknownStaleReplay(t *testing.T) {
	sequence := NewEventSequence(4)
	if _, err := sequence.Accept(Envelope{ID: "six", Cursor: 6}, nil); !errors.Is(err, ErrEventGap) {
		t.Fatalf("gap error = %v", err)
	}
	five := Envelope{ID: "five", Cursor: 5}
	if _, err := sequence.Accept(five, nil); err != nil {
		t.Fatal(err)
	}
	conflict := five
	conflict.ID = "other"
	if _, err := sequence.Accept(conflict, nil); !errors.Is(err, ErrEventConflict) {
		t.Fatalf("conflict error = %v", err)
	}
	if _, err := sequence.Accept(Envelope{ID: "old", Cursor: 3}, nil); !errors.Is(err, ErrEventConflict) {
		t.Fatalf("stale replay error = %v", err)
	}
}

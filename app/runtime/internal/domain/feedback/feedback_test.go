package feedback

import (
	"errors"
	"testing"
	"time"
)

func TestNewEntryValidatesSignalAndTimestamp(t *testing.T) {
	now := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)
	entry, err := NewEntry("ses_1", "run_1", "item_1", RatingPositive, "", now)
	if err != nil {
		t.Fatalf("NewEntry: %v", err)
	}
	if entry.CreatedAt != now || entry.Rating != RatingPositive {
		t.Fatalf("entry = %+v", entry)
	}

	for _, invalid := range []Entry{
		{Rating: Rating("maybe"), CreatedAt: now},
		{CreatedAt: now},
		{Text: "text"},
	} {
		if err := invalid.Validate(); !errors.Is(err, ErrInvalid) {
			t.Fatalf("Validate(%+v) = %v, want ErrInvalid", invalid, err)
		}
	}
}

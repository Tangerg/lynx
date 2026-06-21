package schedule

import (
	"testing"
	"time"
)

func TestValidateCron(t *testing.T) {
	if err := ValidateCron("0 9 * * 1-5"); err != nil {
		t.Errorf("valid 5-field cron rejected: %v", err)
	}
	if err := ValidateCron("@daily"); err != nil {
		t.Errorf("@daily descriptor rejected: %v", err)
	}
	if err := ValidateCron("not a cron"); err == nil {
		t.Error("garbage cron accepted; want error")
	}
	if err := ValidateCron(""); err == nil {
		t.Error("empty cron accepted; want error")
	}
}

// TestNextRun: the next firing is strictly after `after` and lands on the
// scheduled minute (weekday 09:00 here).
func TestNextRun(t *testing.T) {
	// A Wednesday 10:00 — next "weekday 9am" is Thursday 09:00.
	after := time.Date(2026, 6, 17, 10, 0, 0, 0, time.UTC)
	next, err := NextRun("0 9 * * 1-5", after)
	if err != nil {
		t.Fatalf("NextRun: %v", err)
	}
	want := time.Date(2026, 6, 18, 9, 0, 0, 0, time.UTC)
	if !next.Equal(want) {
		t.Errorf("next = %v, want %v", next, want)
	}
	if !next.After(after) {
		t.Errorf("next %v is not strictly after %v", next, after)
	}
}

func TestNextRunInvalid(t *testing.T) {
	if _, err := NextRun("nonsense", time.Now()); err == nil {
		t.Error("NextRun on invalid cron returned no error")
	}
}

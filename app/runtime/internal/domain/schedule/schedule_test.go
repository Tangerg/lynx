package schedule

import (
	"errors"
	"testing"
	"time"
)

func TestScheduleValidate(t *testing.T) {
	base := Schedule{Prompt: "do it", Cron: "0 9 * * 1-5"}
	tests := []struct {
		name string
		mut  func(Schedule) Schedule
		want error // nil = accept
	}{
		{"valid, default model", func(s Schedule) Schedule { return s }, nil},
		{"valid, paired model", func(s Schedule) Schedule { s.Provider, s.Model = "anthropic", "claude"; return s }, nil},
		{"missing prompt", func(s Schedule) Schedule { s.Prompt = ""; return s }, ErrPromptRequired},
		{"missing cron", func(s Schedule) Schedule { s.Cron = ""; return s }, ErrCronRequired},
		{"provider without model", func(s Schedule) Schedule { s.Provider = "anthropic"; return s }, ErrIncompleteModelSelection},
		{"model without provider", func(s Schedule) Schedule { s.Model = "claude"; return s }, ErrIncompleteModelSelection},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.mut(base).Validate(); !errors.Is(err, tt.want) {
				t.Fatalf("Validate() = %v, want %v", err, tt.want)
			}
		})
	}

	bad := base
	bad.Cron = "not a cron"
	if err := bad.Validate(); !errors.Is(err, ErrInvalidCron) {
		t.Fatalf("garbage cron error = %v, want ErrInvalidCron", err)
	}
}

func TestScheduleApplyPatch(t *testing.T) {
	title := "weekday report"
	emptyCwd := ""
	enabled := false
	sc := Schedule{
		Title:   "old",
		Prompt:  "summarize",
		Cwd:     "/work",
		Cron:    "@daily",
		Enabled: true,
	}

	got := sc.Apply(Patch{
		Title:   &title,
		Cwd:     &emptyCwd,
		Enabled: &enabled,
	})
	if got.Title != title || got.Cwd != "" || got.Prompt != sc.Prompt || got.Cron != sc.Cron || got.Enabled {
		t.Fatalf("patched schedule = %+v", got)
	}
}

func TestScheduleScheduledAfter(t *testing.T) {
	after := time.Date(2026, 6, 17, 10, 0, 0, 0, time.UTC)
	sc := Schedule{Prompt: "do it", Cron: "0 9 * * 1-5", Enabled: true}
	got, err := sc.ScheduledAfter(after)
	if err != nil {
		t.Fatalf("ScheduledAfter: %v", err)
	}
	want := time.Date(2026, 6, 18, 9, 0, 0, 0, time.UTC)
	if !got.NextRunAt.Equal(want) {
		t.Fatalf("NextRunAt = %v, want %v", got.NextRunAt, want)
	}

	got, err = Schedule{
		Prompt:    "do it",
		Cron:      "@daily",
		Enabled:   false,
		NextRunAt: want,
	}.ScheduledAfter(after)
	if err != nil {
		t.Fatalf("ScheduledAfter disabled: %v", err)
	}
	if !got.NextRunAt.IsZero() {
		t.Fatalf("disabled NextRunAt = %v, want zero", got.NextRunAt)
	}
}

func TestValidateCron(t *testing.T) {
	if err := ValidateCron("0 9 * * 1-5"); err != nil {
		t.Errorf("valid 5-field cron rejected: %v", err)
	}
	if err := ValidateCron("@daily"); err != nil {
		t.Errorf("@daily descriptor rejected: %v", err)
	}
	if err := ValidateCron("not a cron"); !errors.Is(err, ErrInvalidCron) {
		t.Errorf("garbage cron error = %v, want ErrInvalidCron", err)
	}
	if err := ValidateCron(""); !errors.Is(err, ErrInvalidCron) {
		t.Errorf("empty cron error = %v, want ErrInvalidCron", err)
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
	if _, err := NextRun("nonsense", time.Now()); !errors.Is(err, ErrInvalidCron) {
		t.Errorf("NextRun error = %v, want ErrInvalidCron", err)
	}
}

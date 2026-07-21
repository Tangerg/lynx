package maintenance

import (
	"context"
	"testing"
	"time"
)

type fakeSweeper struct {
	calls int
	seen  []time.Time
}

func (f *fakeSweeper) SweepIdle(_ context.Context, now time.Time, _ time.Duration) ([]string, error) {
	f.calls++
	f.seen = append(f.seen, now)
	return nil, nil
}

func TestSkillCuratorRateLimitsSweeps(t *testing.T) {
	sweeper := &fakeSweeper{}
	curator := NewSkillCurator(sweeper, LifecycleConfig{SweepEvery: time.Hour})
	base := time.Unix(1_700_000_000, 0)
	curator.now = func() time.Time { return base }

	// First call fires (lastSweep is zero — stands in for the boot sweep).
	if err := curator.MaybeSweep(t.Context()); err != nil {
		t.Fatal(err)
	}
	// Within SweepEvery: skipped.
	if err := curator.MaybeSweep(t.Context()); err != nil {
		t.Fatal(err)
	}
	if sweeper.calls != 1 {
		t.Fatalf("calls within the window = %d, want 1", sweeper.calls)
	}
	// Past SweepEvery: fires again.
	curator.now = func() time.Time { return base.Add(2 * time.Hour) }
	if err := curator.MaybeSweep(t.Context()); err != nil {
		t.Fatal(err)
	}
	if sweeper.calls != 2 {
		t.Fatalf("calls after the window = %d, want 2", sweeper.calls)
	}
}

func TestSkillCuratorNilIsNoOp(t *testing.T) {
	var curator *SkillCurator
	if err := curator.MaybeSweep(context.Background()); err != nil {
		t.Fatalf("nil curator MaybeSweep = %v", err)
	}
}

package runmaintenance

import (
	"context"
	"testing"
	"time"
)

type fakeIdleSkillStore struct {
	calls int
	seen  []time.Time
}

func (f *fakeIdleSkillStore) SweepIdle(_ context.Context, now time.Time, _ time.Duration) ([]string, error) {
	f.calls++
	f.seen = append(f.seen, now)
	return nil, nil
}

func TestIdleSkillArchiverRateLimitsChecks(t *testing.T) {
	store := &fakeIdleSkillStore{}
	skillArchiver := NewIdleSkillArchiver(store, SkillArchiveConfig{CheckInterval: time.Hour})
	base := time.Unix(1_700_000_000, 0)
	skillArchiver.now = func() time.Time { return base }

	// First call fires (lastCheck is zero — stands in for the boot sweep).
	if err := skillArchiver.ArchiveIfDue(t.Context()); err != nil {
		t.Fatal(err)
	}
	// Within CheckInterval: skipped.
	if err := skillArchiver.ArchiveIfDue(t.Context()); err != nil {
		t.Fatal(err)
	}
	if store.calls != 1 {
		t.Fatalf("calls within the window = %d, want 1", store.calls)
	}
	// Past CheckInterval: fires again.
	skillArchiver.now = func() time.Time { return base.Add(2 * time.Hour) }
	if err := skillArchiver.ArchiveIfDue(t.Context()); err != nil {
		t.Fatal(err)
	}
	if store.calls != 2 {
		t.Fatalf("calls after the window = %d, want 2", store.calls)
	}
}

func TestIdleSkillArchiverNilIsNoOp(t *testing.T) {
	var skillArchiver *IdleSkillArchiver
	if err := skillArchiver.ArchiveIfDue(context.Background()); err != nil {
		t.Fatalf("nil skillArchiver ArchiveIfDue = %v", err)
	}
}

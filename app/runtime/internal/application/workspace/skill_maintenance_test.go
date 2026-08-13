package workspace

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/application/invalidation"
)

type fakeIdleSkillSweeper struct {
	archived []string
	err      error
	now      time.Time
}

func (f *fakeIdleSkillSweeper) SweepIdle(_ context.Context, now time.Time, _ time.Duration) ([]string, error) {
	f.now = now
	return f.archived, f.err
}

func TestIdleSkillArchiveNotifiesEveryCommittedPartialSweep(t *testing.T) {
	wantErr := errors.New("usage metadata unavailable")
	sweeper := &fakeIdleSkillSweeper{archived: []string{"old-skill"}, err: wantErr}
	var notices []invalidation.Notice
	maintenance := NewSkillMaintenance(sweeper, func(notice invalidation.Notice) {
		notices = append(notices, notice)
	})
	now := time.Date(2026, 8, 13, 9, 0, 0, 0, time.UTC)

	archived, err := maintenance.ArchiveIdle(t.Context(), now, 30*24*time.Hour)
	if !errors.Is(err, wantErr) {
		t.Fatalf("ArchiveIdle error = %v, want %v", err, wantErr)
	}
	if len(archived) != 1 || archived[0] != "old-skill" {
		t.Fatalf("ArchiveIdle archived = %v", archived)
	}
	if !sweeper.now.Equal(now) {
		t.Fatalf("sweep time = %s, want %s", sweeper.now, now)
	}
	if len(notices) != 1 || notices[0].Resource != invalidation.Skills {
		t.Fatalf("notices = %+v, want one Skills invalidation", notices)
	}

	sweeper.archived = nil
	sweeper.err = nil
	if _, err := maintenance.ArchiveIdle(t.Context(), now.Add(time.Hour), 30*24*time.Hour); err != nil {
		t.Fatal(err)
	}
	if len(notices) != 1 {
		t.Fatalf("empty sweep published notices = %+v", notices)
	}
}

func TestIdleSkillArchiveUnavailableFailsExplicitly(t *testing.T) {
	maintenance := NewSkillMaintenance(nil, nil)
	if _, err := maintenance.ArchiveIdle(t.Context(), time.Now(), time.Hour); !errors.Is(err, ErrSkillLibraryUnavailable) {
		t.Fatalf("ArchiveIdle error = %v, want ErrSkillLibraryUnavailable", err)
	}
}

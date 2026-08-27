package schedules

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/Tangerg/scope/app/runtime/internal/application/invalidation"
	"github.com/Tangerg/scope/app/runtime/internal/domain/schedule"
)

var errScheduleMutation = errors.New("schedule mutation failed")

type invalidationScheduleStore struct {
	*runNowStore
	fail        string
	deleteFound bool
}

func (i *invalidationScheduleStore) Create(_ context.Context, scheduled schedule.Schedule) (schedule.Schedule, error) {
	if i.fail == "create" {
		return schedule.Schedule{}, errScheduleMutation
	}
	scheduled.ID = "sch_created"
	return scheduled, nil
}

func (i *invalidationScheduleStore) Update(_ context.Context, scheduled schedule.Schedule, _ uint64) (schedule.Schedule, error) {
	if i.fail == "update" {
		return schedule.Schedule{}, errScheduleMutation
	}
	return scheduled, nil
}

func (i *invalidationScheduleStore) Delete(context.Context, string) (bool, error) {
	if i.fail == "delete" {
		return false, errScheduleMutation
	}
	return i.deleteFound, nil
}

func TestCommittedScheduleMutationsPublishExactInvalidations(t *testing.T) {
	store := &invalidationScheduleStore{
		runNowStore: &runNowStore{schedule: schedule.Schedule{
			ID: "sch_updated", Revision: 1, Instructions: "before", Cron: "@daily", Enabled: true,
		}},
		deleteFound: true,
	}
	var notices []invalidation.Notice
	coordinator := New(Dependencies{
		Store: store,
		Invalidations: func(notice invalidation.Notice) {
			notices = append(notices, notice)
		},
	})

	if _, err := coordinator.Create(t.Context(), CreateCommand{
		Instructions: "create", Cron: "@daily", Enabled: true,
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	title := "after"
	if _, err := coordinator.Update(t.Context(), UpdateCommand{
		ID: "sch_updated", ExpectedRevision: 1, Patch: schedule.Patch{Title: &title},
	}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if err := coordinator.Delete(t.Context(), "sch_deleted"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	want := []string{"sch_created", "sch_updated", "sch_deleted"}
	if len(notices) != len(want) {
		t.Fatalf("notices = %+v, want %d", notices, len(want))
	}
	for i, notice := range notices {
		if notice.Resource != invalidation.Schedules || !slices.Equal(notice.ScheduleIDs, []string{want[i]}) {
			t.Fatalf("notice %d = %+v, want schedule %q", i, notice, want[i])
		}
	}
}

func TestScheduleMutationsPublishOnlyAfterActualCommit(t *testing.T) {
	for _, operation := range []string{"create", "update", "delete"} {
		t.Run(operation, func(t *testing.T) {
			store := &invalidationScheduleStore{
				runNowStore: &runNowStore{schedule: schedule.Schedule{
					ID: "sch_1", Revision: 1, Instructions: "before", Cron: "@daily", Enabled: true,
				}},
				fail: operation, deleteFound: true,
			}
			var notices []invalidation.Notice
			coordinator := New(Dependencies{Store: store, Invalidations: func(notice invalidation.Notice) {
				notices = append(notices, notice)
			}})

			var err error
			switch operation {
			case "create":
				_, err = coordinator.Create(t.Context(), CreateCommand{Instructions: "create", Cron: "@daily", Enabled: true})
			case "update":
				title := "after"
				_, err = coordinator.Update(t.Context(), UpdateCommand{
					ID: "sch_1", ExpectedRevision: 1, Patch: schedule.Patch{Title: &title},
				})
			case "delete":
				err = coordinator.Delete(t.Context(), "sch_1")
			}
			if !errors.Is(err, errScheduleMutation) {
				t.Fatalf("error = %v, want %v", err, errScheduleMutation)
			}
			if len(notices) != 0 {
				t.Fatalf("published after failed %s: %+v", operation, notices)
			}
		})
	}

	store := &invalidationScheduleStore{runNowStore: &runNowStore{}, deleteFound: false}
	var notices []invalidation.Notice
	coordinator := New(Dependencies{Store: store, Invalidations: func(notice invalidation.Notice) {
		notices = append(notices, notice)
	}})
	if err := coordinator.Delete(t.Context(), "sch_missing"); err != nil {
		t.Fatalf("idempotent Delete: %v", err)
	}
	if len(notices) != 0 {
		t.Fatalf("idempotent delete published a false mutation: %+v", notices)
	}
}

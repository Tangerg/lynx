package terminal

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Tangerg/oolong/core/input"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
	"github.com/Tangerg/lynx/app/cli/internal/agent/mock"
	"github.com/Tangerg/lynx/app/cli/internal/changefeed"
	"github.com/Tangerg/lynx/app/cli/internal/schedule"
)

type scheduleServiceStub struct {
	mu        sync.Mutex
	schedules []schedule.Schedule
	created   chan schedule.Candidate
	updated   chan schedule.Patch
	deleted   chan string
	run       chan string
	reads     atomic.Int32
	now       time.Time
}

func newScheduleServiceStub() *scheduleServiceStub {
	now := time.Date(2026, time.August, 12, 10, 0, 0, 0, time.UTC)
	next := now.Add(time.Hour)
	return &scheduleServiceStub{
		schedules: []schedule.Schedule{{
			ID: "sch_review", Title: "Repository review", Instructions: "review the repository",
			Workspace: "/workspace", Cron: "0 * * * *", Enabled: true,
			NextRunAt: &next, CreatedAt: now, Revision: 1,
		}},
		created: make(chan schedule.Candidate, 1), updated: make(chan schedule.Patch, 4),
		deleted: make(chan string, 1), run: make(chan string, 1), now: now,
	}
}

func (service *scheduleServiceStub) Schedules(context.Context) ([]schedule.Schedule, error) {
	service.reads.Add(1)
	service.mu.Lock()
	defer service.mu.Unlock()
	result := make([]schedule.Schedule, len(service.schedules))
	for index, scheduled := range service.schedules {
		result[index] = cloneSchedule(scheduled)
	}
	return result, nil
}

func (service *scheduleServiceStub) Create(_ context.Context, candidate schedule.Candidate) (schedule.Schedule, error) {
	if err := candidate.Validate(); err != nil {
		return schedule.Schedule{}, err
	}
	service.created <- candidate
	next := service.now.Add(2 * time.Hour)
	created := schedule.Schedule{
		ID: "sch_created", Title: candidate.Title, Instructions: candidate.Instructions,
		Workspace: candidate.Workspace, Provider: candidate.Provider, Model: candidate.Model,
		Cron: candidate.Cron, Enabled: true, NextRunAt: &next, CreatedAt: service.now, Revision: 1,
	}
	service.mu.Lock()
	service.schedules = append(service.schedules, created)
	service.mu.Unlock()
	return cloneSchedule(created), nil
}

func (service *scheduleServiceStub) Update(_ context.Context, patch schedule.Patch) (schedule.Schedule, error) {
	if err := patch.Validate(); err != nil {
		return schedule.Schedule{}, err
	}
	service.updated <- patch
	service.mu.Lock()
	defer service.mu.Unlock()
	for index := range service.schedules {
		scheduled := &service.schedules[index]
		if scheduled.ID != patch.ID {
			continue
		}
		if scheduled.Revision != patch.ExpectedRevision {
			return schedule.Schedule{}, errors.New("revision conflict")
		}
		applySchedulePatch(scheduled, patch)
		scheduled.Revision++
		return cloneSchedule(*scheduled), nil
	}
	return schedule.Schedule{}, errors.New("schedule not found")
}

func (service *scheduleServiceStub) Delete(_ context.Context, id string) error {
	service.deleted <- id
	service.mu.Lock()
	defer service.mu.Unlock()
	for index := range service.schedules {
		if service.schedules[index].ID == id {
			service.schedules = append(service.schedules[:index], service.schedules[index+1:]...)
			return nil
		}
	}
	return errors.New("schedule not found")
}

func (service *scheduleServiceStub) RunNow(_ context.Context, id string) (schedule.RunHandle, error) {
	service.run <- id
	return schedule.RunHandle{SessionID: "ses_scheduled", RunID: "run_scheduled"}, nil
}

func cloneSchedule(scheduled schedule.Schedule) schedule.Schedule {
	if scheduled.LastRunAt != nil {
		scheduled.LastRunAt = new(*scheduled.LastRunAt)
	}
	if scheduled.NextRunAt != nil {
		scheduled.NextRunAt = new(*scheduled.NextRunAt)
	}
	return scheduled
}

func applySchedulePatch(scheduled *schedule.Schedule, patch schedule.Patch) {
	if patch.Title != nil {
		scheduled.Title = *patch.Title
	}
	if patch.Instructions != nil {
		scheduled.Instructions = *patch.Instructions
	}
	if workspace, bound := patch.Workspace.Binding(); bound {
		scheduled.Workspace = workspace
	} else if patch.Workspace.UsesDefault() {
		scheduled.Workspace = ""
	}
	if patch.Provider != nil {
		scheduled.Provider, scheduled.Model = *patch.Provider, *patch.Model
	}
	if patch.Cron != nil {
		scheduled.Cron = *patch.Cron
	}
	if patch.Enabled != nil {
		scheduled.Enabled = *patch.Enabled
		if scheduled.Enabled {
			next := scheduled.CreatedAt.Add(time.Hour)
			scheduled.NextRunAt = &next
		} else {
			scheduled.NextRunAt = nil
		}
	}
}

func TestScheduleCatalogReader(t *testing.T) {
	service := newScheduleServiceStub()
	host, stop := runUIWithRuntimeServices(t, Config{Runtime: mock.New(), Schedules: service})
	host.Shows(t, "Ask lyra")
	host.Type("/schedules")
	host.Press(input.Enter)
	host.Shows(t, "Repository review")
	host.Shows(t, "sch_review")
	stop()
}

func TestScheduleCreateFormSurvivesExtremeResize(t *testing.T) {
	service := newScheduleServiceStub()
	host, stop := runUIWithRuntimeServices(t, Config{Runtime: mock.New(), Schedules: service})
	host.Shows(t, "Ask lyra")
	host.Type("/schedule-create")
	host.Press(input.Enter)
	host.Shows(t, "Create scheduled run")
	host.Type("Daily audit")
	host.Press(input.Tab)
	host.Type("audit the repository")
	host.Press(input.Tab)
	host.Press(input.Tab)
	host.Press(input.Tab)
	host.Type("deepseek")
	host.Press(input.Enter)
	host.Shows(t, "provider and model must both be set or both be empty")
	select {
	case candidate := <-service.created:
		t.Fatalf("incomplete model selection reached the service: %+v", candidate)
	default:
	}
	if !host.Resize(1, 1) || !host.Repaint() || !host.Resize(96, 28) {
		t.Fatal("schedule form did not survive a minimal viewport")
	}
	host.Shows(t, "Create scheduled run")
	host.Press(input.Tab)
	host.Type("deepseek-v4-flash")
	host.Press(input.Enter)
	host.Shows(t, "Daily audit")
	created := awaitValue(t, service.created, "schedule creation")
	if created.Instructions != "audit the repository" || created.Cron != "0 9 * * 1-5" || created.Workspace == "" ||
		created.Provider != "deepseek" || created.Model != "deepseek-v4-flash" {
		t.Fatalf("created schedule candidate = %+v", created)
	}
	stop()
}

func TestWorkspaceReplacementRetiresAPresentedScheduleForm(t *testing.T) {
	backend := mock.New()
	service := newScheduleServiceStub()
	source := &runtimeChangeSourceStub{
		events: make(chan changefeed.Event, 1), subscription: make(chan changefeed.Subscription, 1),
		applied: make(chan changefeed.Event, 1),
	}
	host, stop := runUIWithRuntimeServices(t, Config{
		Runtime: backend, Schedules: service, Changes: source, SessionID: "ses_demo_1",
	})
	host.Shows(t, "Ask lyra")
	awaitValue(t, source.subscription, "runtime invalidation subscription")
	host.Type("/schedule-create")
	host.Press(input.Enter)
	host.Shows(t, "Create scheduled run")

	snapshot, err := backend.GetSession(t.Context(), "ses_demo_1")
	if err != nil {
		t.Fatal(err)
	}
	replacementWorkspace := filepath.Join(t.TempDir(), "replacement")
	if _, err := backend.UpdateSession(t.Context(), agent.UpdateSession{
		SessionID: snapshot.Session.ID, Workspace: &replacementWorkspace,
		ExpectedRevision: snapshot.Session.Revision,
	}); err != nil {
		t.Fatal(err)
	}
	source.events <- changefeed.Event{
		Type: changefeed.EventType(changefeed.SessionsChanged), Sequence: 1,
		SessionIDs: []string{"ses_demo_1"},
	}
	awaitSignal(t, source.applied, "workspace replacement invalidation")
	host.Hides(t, "Create scheduled run")
	host.Press(input.Enter)
	select {
	case candidate := <-service.created:
		t.Fatalf("retired schedule form created %+v", candidate)
	default:
	}
	stop()
}

func TestScheduleEditEnableRunAndDeleteCommands(t *testing.T) {
	t.Run("edit", func(t *testing.T) {
		service := newScheduleServiceStub()
		host, stop := runUIWithRuntimeServices(t, Config{Runtime: mock.New(), Schedules: service})
		host.Shows(t, "Ask lyra")
		host.Type("/schedule-edit sch_review")
		host.Press(input.Enter)
		host.Shows(t, "Edit scheduled run · sch_review")
		host.Type(" updated")
		host.Press(input.Enter)
		host.Shows(t, "Repository review updated")
		patch := awaitValue(t, service.updated, "schedule edit")
		if patch.Title == nil || *patch.Title != "Repository review updated" {
			t.Fatalf("edit patch = %+v", patch)
		}
		stop()
	})

	t.Run("disable", func(t *testing.T) {
		service := newScheduleServiceStub()
		host, stop := runUIWithRuntimeServices(t, Config{Runtime: mock.New(), Schedules: service})
		host.Shows(t, "Ask lyra")
		host.Type("/schedule-disable sch_review")
		host.Press(input.Enter)
		host.Shows(t, "status   disabled")
		patch := awaitValue(t, service.updated, "schedule disable")
		if patch.Enabled == nil || *patch.Enabled {
			t.Fatalf("disable patch = %+v", patch)
		}
		stop()
	})

	t.Run("enable", func(t *testing.T) {
		service := newScheduleServiceStub()
		service.schedules[0].Enabled = false
		service.schedules[0].NextRunAt = nil
		host, stop := runUIWithRuntimeServices(t, Config{Runtime: mock.New(), Schedules: service})
		host.Shows(t, "Ask lyra")
		host.Type("/schedule-enable sch_review")
		host.Press(input.Enter)
		host.Shows(t, "status   enabled")
		patch := awaitValue(t, service.updated, "schedule enable")
		if patch.Enabled == nil || !*patch.Enabled {
			t.Fatalf("enable patch = %+v", patch)
		}
		stop()
	})

	t.Run("run now", func(t *testing.T) {
		service := newScheduleServiceStub()
		host, stop := runUIWithRuntimeServices(t, Config{Runtime: mock.New(), Schedules: service})
		host.Shows(t, "Ask lyra")
		host.Type("/schedule-run sch_review")
		host.Press(input.Enter)
		host.Shows(t, "session ses_scheduled · run run_scheduled")
		if id := awaitValue(t, service.run, "schedule run"); id != "sch_review" {
			t.Fatalf("run schedule id = %q", id)
		}
		stop()
	})

	t.Run("delete", func(t *testing.T) {
		service := newScheduleServiceStub()
		host, stop := runUIWithRuntimeServices(t, Config{Runtime: mock.New(), Schedules: service})
		host.Shows(t, "Ask lyra")
		host.Type("/schedule-delete sch_review")
		host.Press(input.Enter)
		host.Shows(t, "Delete scheduled run")
		host.Press(input.Down)
		host.Press(input.Enter)
		host.Shows(t, "none configured")
		if id := awaitValue(t, service.deleted, "schedule deletion"); id != "sch_review" {
			t.Fatalf("deleted schedule id = %q", id)
		}
		stop()
	})
}

func TestSchedulesChangedRefetchesOnlyTheOpenScheduleReader(t *testing.T) {
	service := newScheduleServiceStub()
	source := &runtimeChangeSourceStub{
		events: make(chan changefeed.Event, 1), subscription: make(chan changefeed.Subscription, 1),
		applied: make(chan changefeed.Event, 1), supported: []changefeed.Topic{changefeed.SchedulesChanged},
	}
	host, stop := runUIWithRuntimeServices(t, Config{Runtime: mock.New(), Schedules: service, Changes: source})
	host.Shows(t, "Ask lyra")
	subscription := awaitValue(t, source.subscription, "schedule invalidation subscription")
	if len(subscription.Topics) != 1 || subscription.Topics[0] != changefeed.SchedulesChanged {
		t.Fatalf("schedule subscription = %+v", subscription)
	}
	host.Type("/schedules")
	host.Press(input.Enter)
	host.Shows(t, "Repository review")
	baseline := service.reads.Load()
	service.mu.Lock()
	service.schedules[0].Title = "Updated repository review"
	service.mu.Unlock()
	source.events <- changefeed.Event{
		Type: changefeed.EventType(changefeed.SchedulesChanged), Sequence: 1,
		ScheduleIDs: []string{"sch_review"},
	}
	awaitSignal(t, source.applied, "schedules.changed delivery")
	host.Shows(t, "Updated repository review")
	if service.reads.Load() <= baseline {
		t.Fatal("schedules.changed did not refetch the open schedule projection")
	}
	stop()
}

func TestScheduleDraftClearsWorkspaceBindingAndRejectsAmbiguousIdentity(t *testing.T) {
	t.Parallel()
	original := newScheduleServiceStub().schedules[0]
	draft := newScheduleFormDraft(scheduleFormUpdate, original, "")
	draft.workspace = ""
	patch, changed, err := draft.patch(original)
	if err != nil || !changed || !patch.Workspace.UsesDefault() {
		t.Fatalf("workspace clearing patch = (%+v, %v, %v)", patch, changed, err)
	}
	duplicate := cloneSchedule(original)
	duplicate.ID = "sch_review_other"
	if _, err := resolveSchedule([]schedule.Schedule{original, duplicate}, "sch_rev"); err == nil {
		t.Fatal("ambiguous schedule prefix was accepted")
	}
}

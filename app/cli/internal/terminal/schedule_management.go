package terminal

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Tangerg/lynx/app/cli/internal/schedule"
)

func (a *app) ShowSchedules() {
	if a.schedules == nil {
		a.message("this runtime composition has no schedule service")
		return
	}
	a.runRuntimeReaderQuery("loading schedules", scheduleOperation, runtimeReaderSchedules,
		func(ctx context.Context) (readerDocument, error) {
			schedules, err := a.schedules.Schedules(ctx)
			if err != nil {
				return readerDocument{}, err
			}
			return schedulesDocument(schedules), nil
		})
}

func schedulesDocument(schedules []schedule.Schedule) readerDocument {
	if len(schedules) == 0 {
		return paragraphDocument("Scheduled runs", "none configured", []string{"No scheduled runs are configured."})
	}
	sections := make([]ToolSection, 0, len(schedules)*2)
	for _, scheduled := range schedules {
		title := scheduled.Title
		if title == "" {
			title = "Untitled schedule"
		}
		status := "disabled"
		if scheduled.Enabled {
			status = "enabled"
		}
		metadata := []string{
			"id       " + scheduled.ID,
			"cron     " + scheduled.Cron,
			"status   " + status,
			"revision " + fmt.Sprint(scheduled.Revision),
			"created  " + scheduled.CreatedAt.Format(time.RFC3339),
		}
		if scheduled.Workspace != "" {
			metadata = append(metadata, "workspace "+scheduled.Workspace)
		}
		if scheduled.Provider != "" {
			metadata = append(metadata, "model    "+scheduled.Provider+"/"+scheduled.Model)
		}
		if scheduled.NextRunAt != nil {
			metadata = append(metadata, "next     "+scheduled.NextRunAt.Format(time.RFC3339))
		}
		if scheduled.LastRunAt != nil {
			metadata = append(metadata, "last     "+scheduled.LastRunAt.Format(time.RFC3339))
		}
		sections = append(sections,
			ToolSection{Title: title, Style: toolSectionParagraph, Text: scheduled.Instructions, Links: true},
			ToolSection{Title: "Configuration", Style: toolSectionCode, Language: "text", Text: strings.Join(metadata, "\n")},
		)
	}
	return readerDocument{Title: "Scheduled runs", Detail: fmt.Sprintf("%d configured", len(schedules)), Sections: sections}
}

func (a *app) OpenScheduleCreateForm() error {
	if a.schedules == nil {
		return errors.New("this runtime composition has no schedule service")
	}
	a.openScheduleForm(scheduleFormCreate, schedule.Schedule{})
	return nil
}

func (a *app) EditSchedule(identity string) error {
	return a.loadSchedule(identity, "loading schedule to edit", func(scheduled schedule.Schedule) {
		a.openScheduleForm(scheduleFormUpdate, scheduled)
	})
}

func (a *app) SetScheduleEnabled(identity string, enabled bool) error {
	verb := "enabling"
	if !enabled {
		verb = "disabling"
	}
	return a.loadSchedule(identity, verb+" schedule", func(scheduled schedule.Schedule) {
		if scheduled.Enabled == enabled {
			state := "disabled"
			if enabled {
				state = "enabled"
			}
			a.message("schedule is already " + state + " · " + scheduled.ID)
			return
		}
		patch := schedule.Patch{ID: scheduled.ID, ExpectedRevision: scheduled.Revision, Enabled: &enabled}
		a.updateSchedule(patch, verb+" schedule "+scheduled.ID)
	})
}

func (a *app) PrepareDeleteSchedule(identity string) error {
	return a.loadSchedule(identity, "loading schedule to delete", func(scheduled schedule.Schedule) {
		title := scheduled.Title
		if title == "" {
			title = scheduled.ID
		}
		a.confirmAction("Delete scheduled run", "Delete "+title+" ("+scheduled.ID+")?", "Delete permanently", func() {
			a.deleteSchedule(scheduled.ID)
		})
	})
}

func (a *app) RunScheduleNow(identity string) error {
	return a.loadSchedule(identity, "loading schedule to run", func(scheduled schedule.Schedule) {
		a.status.note("running schedule " + scheduled.ID)
		started := runOperation(a, scheduleOperation, false,
			func(ctx context.Context) (schedule.RunHandle, error) { return a.schedules.RunNow(ctx, scheduled.ID) },
			func(handle schedule.RunHandle, err error) {
				if err != nil {
					a.message("run schedule now failed: " + err.Error())
					return
				}
				a.message("schedule started · session " + handle.SessionID + " · run " + handle.RunID)
			},
		)
		if !started {
			a.message("another schedule operation is running")
		}
	})
}

func (a *app) loadSchedule(identity, label string, apply func(schedule.Schedule)) error {
	if a.schedules == nil {
		return errors.New("this runtime composition has no schedule service")
	}
	identity = strings.TrimSpace(identity)
	if identity == "" {
		return errors.New("a schedule id, unique prefix, or unique title is required")
	}
	a.status.note(label)
	started := runOperation(a, scheduleOperation, false,
		func(ctx context.Context) (schedule.Schedule, error) {
			schedules, err := a.schedules.Schedules(ctx)
			if err != nil {
				return schedule.Schedule{}, err
			}
			return resolveSchedule(schedules, identity)
		},
		func(scheduled schedule.Schedule, err error) {
			if err != nil {
				a.message(label + " failed: " + err.Error())
				return
			}
			apply(scheduled)
		},
	)
	if !started {
		return errors.New("another schedule operation is running")
	}
	return nil
}

func resolveSchedule(schedules []schedule.Schedule, identity string) (schedule.Schedule, error) {
	for _, scheduled := range schedules {
		if scheduled.ID == identity {
			return scheduled, nil
		}
	}
	matches := make([]schedule.Schedule, 0, 1)
	for _, scheduled := range schedules {
		if strings.HasPrefix(scheduled.ID, identity) || (scheduled.Title != "" && scheduled.Title == identity) {
			matches = append(matches, scheduled)
		}
	}
	switch len(matches) {
	case 0:
		return schedule.Schedule{}, errors.New("schedule not found: " + identity)
	case 1:
		return matches[0], nil
	default:
		return schedule.Schedule{}, errors.New("schedule identity is ambiguous; use the full id")
	}
}

func (a *app) createSchedule(candidate schedule.Candidate) {
	a.status.note("creating schedule")
	started := runOperation(a, scheduleOperation, false,
		func(ctx context.Context) (schedule.Schedule, error) { return a.schedules.Create(ctx, candidate) },
		func(created schedule.Schedule, err error) {
			if err != nil {
				a.message("create schedule failed: " + err.Error())
				return
			}
			a.message("schedule created · " + created.ID)
			a.ShowSchedules()
		},
	)
	if !started {
		a.message("another schedule operation is running")
	}
}

func (a *app) updateSchedule(patch schedule.Patch, label string) {
	a.status.note(label)
	started := runOperation(a, scheduleOperation, false,
		func(ctx context.Context) (schedule.Schedule, error) { return a.schedules.Update(ctx, patch) },
		func(updated schedule.Schedule, err error) {
			if err != nil {
				a.message(label + " failed: " + err.Error())
				return
			}
			a.message("schedule updated · " + updated.ID)
			a.ShowSchedules()
		},
	)
	if !started {
		a.message("another schedule operation is running")
	}
}

func (a *app) deleteSchedule(id string) {
	a.status.note("deleting schedule " + id)
	started := runOperation(a, scheduleOperation, false,
		func(ctx context.Context) (string, error) { return id, a.schedules.Delete(ctx, id) },
		func(deleted string, err error) {
			if err != nil {
				a.message("delete schedule failed: " + err.Error())
				return
			}
			a.message("schedule deleted · " + deleted)
			a.ShowSchedules()
		},
	)
	if !started {
		a.message("another schedule operation is running")
	}
}

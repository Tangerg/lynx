package schedule

import (
	"strings"
	"testing"
	"time"
)

func TestScheduleValidatesLifecycleAndModelSelection(t *testing.T) {
	t.Parallel()
	created := time.Date(2026, time.August, 12, 10, 0, 0, 0, time.UTC)
	next := created.Add(time.Hour)
	valid := Schedule{
		ID: "sch_1", Instructions: "review the repository", Cron: "0 * * * *", Enabled: true,
		Provider: "deepseek", Model: "deepseek-v4-flash", CreatedAt: created, NextRunAt: &next, Revision: 1,
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid schedule: %v", err)
	}

	for _, test := range []struct {
		name   string
		mutate func(*Schedule)
		want   string
	}{
		{name: "missing id", mutate: func(value *Schedule) { value.ID = "" }, want: "id is empty"},
		{name: "missing model", mutate: func(value *Schedule) { value.Model = "" }, want: "both be set"},
		{name: "enabled without next", mutate: func(value *Schedule) { value.NextRunAt = nil }, want: "inconsistent"},
		{name: "disabled with next", mutate: func(value *Schedule) { value.Enabled = false }, want: "inconsistent"},
		{name: "missing revision", mutate: func(value *Schedule) { value.Revision = 0 }, want: "persistence metadata"},
	} {
		t.Run(test.name, func(t *testing.T) {
			value := valid
			test.mutate(&value)
			if err := value.Validate(); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Validate() = %v, want %q", err, test.want)
			}
		})
	}
}

func TestPatchRequiresRevisionAndCoherentChanges(t *testing.T) {
	t.Parallel()
	title := "daily review"
	provider, model := "deepseek", "deepseek-v4-flash"
	valid := Patch{ID: "sch_1", ExpectedRevision: 3, Title: &title}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid patch: %v", err)
	}
	for _, test := range []struct {
		name  string
		patch Patch
		want  string
	}{
		{name: "no revision", patch: Patch{ID: "sch_1", Title: &title}, want: "revision"},
		{name: "no changes", patch: Patch{ID: "sch_1", ExpectedRevision: 1}, want: "no changes"},
		{name: "partial model", patch: Patch{ID: "sch_1", ExpectedRevision: 1, Provider: &provider}, want: "together"},
		{name: "complete model", patch: Patch{ID: "sch_1", ExpectedRevision: 1, Provider: &provider, Model: &model}},
		{name: "bind empty workspace", patch: Patch{ID: "sch_1", ExpectedRevision: 1, Workspace: BindWorkspace(" ")}, want: "binding is empty"},
		{name: "use default workspace", patch: Patch{ID: "sch_1", ExpectedRevision: 1, Workspace: UseDefaultWorkspace()}},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := test.patch.Validate()
			if test.want == "" && err != nil {
				t.Fatalf("Validate() = %v", err)
			}
			if test.want != "" && (err == nil || !strings.Contains(err.Error(), test.want)) {
				t.Fatalf("Validate() = %v, want %q", err, test.want)
			}
		})
	}
}

func TestWorkspaceChangeOwnsItsThreeStateSemantics(t *testing.T) {
	t.Parallel()
	var unchanged WorkspaceChange
	if unchanged.Changed() || unchanged.UsesDefault() {
		t.Fatalf("zero workspace change = %+v", unchanged)
	}
	bound := BindWorkspace(" /workspace ")
	path, ok := bound.Binding()
	if !bound.Changed() || !ok || path != "/workspace" || bound.UsesDefault() {
		t.Fatalf("bound workspace change = %+v", bound)
	}
	cleared := UseDefaultWorkspace()
	if !cleared.Changed() || !cleared.UsesDefault() {
		t.Fatalf("default workspace change = %+v", cleared)
	}
}

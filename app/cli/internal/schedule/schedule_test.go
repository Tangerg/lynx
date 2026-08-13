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

func TestScheduleMutationResultsMustFulfillTheCommand(t *testing.T) {
	t.Parallel()
	createdAt := time.Date(2026, time.August, 12, 10, 0, 0, 0, time.UTC)
	nextRunAt := createdAt.Add(time.Hour)
	candidate := Candidate{
		Title: "Review", Instructions: "review the repository", Workspace: "/workspace",
		Provider: "deepseek", Model: "deepseek-v4", Cron: "0 * * * *",
	}
	valid := Schedule{
		ID: "sch_1", Title: candidate.Title, Instructions: candidate.Instructions,
		Workspace: candidate.Workspace, Provider: candidate.Provider, Model: candidate.Model,
		Cron: candidate.Cron, Enabled: true, NextRunAt: &nextRunAt, CreatedAt: createdAt, Revision: 1,
	}
	if err := candidate.ValidateResult(valid); err != nil {
		t.Fatalf("valid create result: %v", err)
	}
	lastRunAt := createdAt.Add(30 * time.Minute)
	createTests := []struct {
		name   string
		mutate func(*Schedule)
		want   string
	}{
		{name: "title", mutate: func(result *Schedule) { result.Title = "ignored" }, want: "title"},
		{name: "instructions", mutate: func(result *Schedule) { result.Instructions = "ignored" }, want: "instructions"},
		{name: "workspace", mutate: func(result *Schedule) { result.Workspace = "/other" }, want: "workspace"},
		{name: "model", mutate: func(result *Schedule) { result.Model = "shallow" }, want: "model"},
		{name: "cron", mutate: func(result *Schedule) { result.Cron = "30 * * * *" }, want: "cron"},
		{name: "disabled", mutate: func(result *Schedule) {
			result.Enabled = false
			result.NextRunAt = nil
		}, want: "disabled"},
		{name: "prior run", mutate: func(result *Schedule) { result.LastRunAt = &lastRunAt }, want: "prior run"},
	}
	for _, test := range createTests {
		t.Run("create "+test.name, func(t *testing.T) {
			result := valid
			test.mutate(&result)
			err := candidate.ValidateResult(result)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("ValidateResult error = %v, want %q", err, test.want)
			}
		})
	}

	title, instructions, provider, model, cron, enabled := "Updated", "inspect", "anthropic", "deep", "30 * * * *", false
	patch := Patch{
		ID: valid.ID, ExpectedRevision: valid.Revision, Title: &title, Instructions: &instructions,
		Workspace: UseDefaultWorkspace(), Provider: &provider, Model: &model, Cron: &cron, Enabled: &enabled,
	}
	updated := valid
	updated.Title, updated.Instructions, updated.Workspace = title, instructions, ""
	updated.Provider, updated.Model, updated.Cron, updated.Enabled = provider, model, cron, enabled
	updated.NextRunAt, updated.Revision = nil, 2
	if err := patch.ValidateResult(updated); err != nil {
		t.Fatalf("valid update result: %v", err)
	}
	updateTests := []struct {
		name   string
		mutate func(*Schedule)
		want   string
	}{
		{name: "identity", mutate: func(result *Schedule) { result.ID = "sch_other" }, want: "schedule"},
		{name: "revision", mutate: func(result *Schedule) { result.Revision = patch.ExpectedRevision }, want: "revision"},
		{name: "title", mutate: func(result *Schedule) { result.Title = "ignored" }, want: "title"},
		{name: "instructions", mutate: func(result *Schedule) { result.Instructions = "ignored" }, want: "instructions"},
		{name: "workspace", mutate: func(result *Schedule) { result.Workspace = "/workspace" }, want: "workspace"},
		{name: "model", mutate: func(result *Schedule) { result.Model = "shallow" }, want: "model"},
		{name: "cron", mutate: func(result *Schedule) { result.Cron = "0 9 * * *" }, want: "cron"},
		{name: "enabled", mutate: func(result *Schedule) {
			result.Enabled = true
			result.NextRunAt = &nextRunAt
		}, want: "enabled"},
	}
	for _, test := range updateTests {
		t.Run("update "+test.name, func(t *testing.T) {
			result := updated
			test.mutate(&result)
			err := patch.ValidateResult(result)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("ValidateResult error = %v, want %q", err, test.want)
			}
		})
	}

	for _, invalid := range []interface{ Validate() error }{
		Candidate{Instructions: "review", Workspace: "relative", Cron: "0 * * * *"},
		Candidate{Instructions: "review", Provider: " deepseek", Model: "deep", Cron: "0 * * * *"},
		Patch{ID: "sch_1", ExpectedRevision: 1, Workspace: BindWorkspace("relative")},
	} {
		if err := invalid.Validate(); err == nil {
			t.Fatalf("invalid schedule mutation %#v was accepted", invalid)
		}
	}
}

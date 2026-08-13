package agentmemory

import (
	"strings"
	"testing"
	"time"
)

func TestTargetOwnsScopeWorkspaceInvariant(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name      string
		scope     Scope
		workspace string
		wantErr   bool
	}{
		{name: "project", scope: Project, workspace: "/repo"},
		{name: "project with relative workspace", scope: Project, workspace: "repo", wantErr: true},
		{name: "project without workspace", scope: Project, wantErr: true},
		{name: "user", scope: User},
		{name: "user with workspace", scope: User, workspace: "/repo", wantErr: true},
		{name: "unknown", scope: "session", wantErr: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, err := NewTarget(test.scope, test.workspace)
			if (err != nil) != test.wantErr {
				t.Fatalf("NewTarget() error = %v, wantErr %t", err, test.wantErr)
			}
		})
	}
}

func TestItemRejectsBrokenReviewProjection(t *testing.T) {
	t.Parallel()
	now := time.Now()
	valid := Item{ID: "mem_1", Scope: Project, Content: "fact", Origin: Automatic, Status: Pending, CreatedAt: now, UpdatedAt: now}
	if err := valid.Validate(); err != nil {
		t.Fatal(err)
	}
	invalid := valid
	invalid.Origin = Authored
	if err := invalid.Validate(); err == nil || !strings.Contains(err.Error(), "must be active") {
		t.Fatalf("Validate() = %v", err)
	}
}

func TestPatchRequiresAnIntentionalChange(t *testing.T) {
	t.Parallel()
	if err := (Patch{ID: "mem_1"}).Validate(); err == nil {
		t.Fatal("empty patch was accepted")
	}
	empty := "  "
	if err := (Patch{ID: "mem_1", Content: &empty}).Validate(); err == nil {
		t.Fatal("blank content was accepted")
	}
	pinned := true
	if err := (Patch{ID: "mem_1", Pinned: &pinned}).Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestAgentMemoryMutationResultsMustFulfillTheCommand(t *testing.T) {
	t.Parallel()
	now := time.Now()
	valid := Item{
		ID: "mem_1", Scope: User, Content: "authored", Origin: Authored, Status: Active,
		CreatedAt: now, UpdatedAt: now,
	}
	target, err := NewTarget(User, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := target.ValidateAddResult(" authored ", valid); err != nil {
		t.Fatalf("valid add result: %v", err)
	}
	wrongAdd := valid
	wrongAdd.Content = "ignored"
	if err := target.ValidateAddResult("authored", wrongAdd); err == nil || !strings.Contains(err.Error(), "content") {
		t.Fatalf("add result error = %v", err)
	}

	content, pinned := "edited", true
	patch := Patch{ID: valid.ID, Content: &content, Pinned: &pinned}
	updated := valid
	updated.Content, updated.Pinned = content, pinned
	if err := patch.ValidateResult(updated); err != nil {
		t.Fatalf("valid update result: %v", err)
	}
	for _, test := range []struct {
		name   string
		mutate func(*Item)
		want   string
	}{
		{name: "identity", mutate: func(result *Item) { result.ID = "mem_other" }, want: "item"},
		{name: "content", mutate: func(result *Item) { result.Content = "ignored" }, want: "content"},
		{name: "pinned", mutate: func(result *Item) { result.Pinned = false }, want: "pinned"},
	} {
		t.Run(test.name, func(t *testing.T) {
			result := updated
			test.mutate(&result)
			err := patch.ValidateResult(result)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("ValidateResult error = %v, want %q", err, test.want)
			}
		})
	}
}

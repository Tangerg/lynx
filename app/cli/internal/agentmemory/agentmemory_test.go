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

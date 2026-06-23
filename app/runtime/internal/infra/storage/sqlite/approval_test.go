package sqlite_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/approval"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/storage/sqlite"
)

func newApprovalStore(t *testing.T) *sqlite.ApprovalRuleStore {
	t.Helper()
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "lyra.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return sqlite.NewApprovalRuleStore(db)
}

// TestApprovalRuleStore_VisibleScopes verifies the WHERE-clause scope predicate:
// a session rule, a project rule (keyed by dir), and a global rule are each
// visible only from the right session/dir, and global is visible everywhere.
func TestApprovalRuleStore_VisibleScopes(t *testing.T) {
	ctx := context.Background()
	store := newApprovalStore(t)

	put := func(r approval.Rule) {
		if err := store.Put(ctx, r); err != nil {
			t.Fatalf("put: %v", err)
		}
	}
	put(approval.Rule{ID: "s", Scope: approval.ScopeSession, ScopeKey: "sess1", Tool: "shell", Decision: approval.Allow})
	put(approval.Rule{ID: "p", Scope: approval.ScopeProject, ScopeKey: "/proj/a", Tool: "write", Decision: approval.Deny})
	put(approval.Rule{ID: "g", Scope: approval.ScopeGlobal, Tool: "read", Decision: approval.Allow})

	ids := func(sessionID, dir string) map[string]bool {
		rules, err := store.Visible(ctx, sessionID, dir)
		if err != nil {
			t.Fatalf("visible: %v", err)
		}
		m := map[string]bool{}
		for _, r := range rules {
			m[r.ID] = true
		}
		return m
	}

	// From sess1 in /proj/a: all three visible.
	if got := ids("sess1", "/proj/a"); !got["s"] || !got["p"] || !got["g"] || len(got) != 3 {
		t.Fatalf("sess1@/proj/a sees %v, want s+p+g", got)
	}
	// From another session in another dir: only global.
	if got := ids("sess2", "/proj/b"); got["s"] || got["p"] || !got["g"] || len(got) != 1 {
		t.Fatalf("sess2@/proj/b sees %v, want only g", got)
	}
	// With no cwd: project rule must not match (skipped when dir is empty).
	if got := ids("sess1", ""); got["p"] {
		t.Fatalf("project rule leaked with empty dir: %v", got)
	}
}

// TestApprovalRuleStore_UpsertAndDelete verifies Put upserts by id (decision
// flips, no duplicate row) and Delete removes by id.
func TestApprovalRuleStore_UpsertAndDelete(t *testing.T) {
	ctx := context.Background()
	store := newApprovalStore(t)
	r := approval.Rule{ID: "x", Scope: approval.ScopeGlobal, Tool: "shell", Subject: "npm run *", Decision: approval.Allow}
	if err := store.Put(ctx, r); err != nil {
		t.Fatalf("put: %v", err)
	}
	r.Decision = approval.Deny
	if err := store.Put(ctx, r); err != nil {
		t.Fatalf("re-put: %v", err)
	}
	rules, _ := store.Visible(ctx, "s", "/p")
	if len(rules) != 1 || rules[0].Decision != approval.Deny || rules[0].Subject != "npm run *" {
		t.Fatalf("after upsert = %+v, want one Deny rule (no duplicate)", rules)
	}
	if err := store.Delete(ctx, "x"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if rules, _ := store.Visible(ctx, "s", "/p"); len(rules) != 0 {
		t.Fatalf("after delete = %+v, want none", rules)
	}
}

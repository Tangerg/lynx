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
	sessionRule := newApprovalRule(t, approval.ScopeSession, "sess1", "shell", "", approval.Allow)
	projectRule := newApprovalRule(t, approval.ScopeProject, "/proj/a", "write", "", approval.Deny)
	globalRule := newApprovalRule(t, approval.ScopeGlobal, "", "read", "", approval.Allow)
	put(sessionRule)
	put(projectRule)
	put(globalRule)

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
	if got := ids("sess1", "/proj/a"); !got[sessionRule.ID] || !got[projectRule.ID] || !got[globalRule.ID] || len(got) != 3 {
		t.Fatalf("sess1@/proj/a sees %v, want s+p+g", got)
	}
	// From another session in another dir: only global.
	if got := ids("sess2", "/proj/b"); got[sessionRule.ID] || got[projectRule.ID] || !got[globalRule.ID] || len(got) != 1 {
		t.Fatalf("sess2@/proj/b sees %v, want only g", got)
	}
	// With no cwd: project rule must not match (skipped when dir is empty).
	if got := ids("sess1", ""); got[projectRule.ID] {
		t.Fatalf("project rule leaked with empty dir: %v", got)
	}
}

// TestApprovalRuleStore_UpsertAndDelete verifies Put upserts by id (decision
// flips, no duplicate row) and Delete removes by id.
func TestApprovalRuleStore_UpsertAndDelete(t *testing.T) {
	ctx := context.Background()
	store := newApprovalStore(t)
	r := newApprovalRule(t, approval.ScopeGlobal, "", "shell", "npm run *", approval.Allow)
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
	if err := store.Delete(ctx, r.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if rules, _ := store.Visible(ctx, "s", "/p"); len(rules) != 0 {
		t.Fatalf("after delete = %+v, want none", rules)
	}
}

func TestApprovalRuleStore_DeleteSessionPreservesBroaderScopes(t *testing.T) {
	ctx := context.Background()
	store := newApprovalStore(t)
	sessionOne := newApprovalRule(t, approval.ScopeSession, "sess1", "shell", "", approval.Allow)
	sessionTwo := newApprovalRule(t, approval.ScopeSession, "sess2", "shell", "", approval.Allow)
	project := newApprovalRule(t, approval.ScopeProject, "/proj", "write", "", approval.Allow)
	global := newApprovalRule(t, approval.ScopeGlobal, "", "read", "", approval.Allow)
	for _, rule := range []approval.Rule{sessionOne, sessionTwo, project, global} {
		if err := store.Put(ctx, rule); err != nil {
			t.Fatalf("put %s: %v", rule.ID, err)
		}
	}
	if err := store.DeleteSession(ctx, "sess1"); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}
	rules, err := store.Visible(ctx, "sess1", "/proj")
	if err != nil {
		t.Fatalf("Visible: %v", err)
	}
	ids := map[string]bool{}
	for _, rule := range rules {
		ids[rule.ID] = true
	}
	if ids[sessionOne.ID] || !ids[project.ID] || !ids[global.ID] || len(ids) != 2 {
		t.Fatalf("visible after DeleteSession = %v, want p+g", ids)
	}
	if rules, err := store.Visible(ctx, "sess2", ""); err != nil || len(rules) != 2 {
		t.Fatalf("other session after DeleteSession = %+v, %v, want s2+g", rules, err)
	}
}

func newApprovalRule(t *testing.T, scope approval.Scope, scopeKey, toolName, subject string, decision approval.Decision) approval.Rule {
	t.Helper()
	rule, err := approval.NewRule(scope, scopeKey, toolName, subject, decision)
	if err != nil {
		t.Fatalf("new approval rule: %v", err)
	}
	return rule
}

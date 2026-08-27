package workspace

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Tangerg/scope/app/runtime/internal/domain/session"
	"github.com/Tangerg/scope/app/runtime/internal/testsupport/sessionfixture"
)

type staticAgentDocFinder struct{ files []AgentDocFile }

func (s staticAgentDocFinder) Find(context.Context, string, string) ([]AgentDocFile, error) {
	return s.files, nil
}

func TestWorkspacesFromSessions(t *testing.T) {
	t0 := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	workspaces := workspacesFromSessions([]session.Session{
		sessionfixture.MustRestore(session.Snapshot{ID: "s1", Workspace: sessionfixture.MustWorkspace("/a/proj"), UpdatedAt: t0}),
		sessionfixture.MustRestore(session.Snapshot{ID: "s2", Workspace: sessionfixture.MustWorkspace("/a/proj"), UpdatedAt: t0.Add(2 * time.Hour)}),
		sessionfixture.MustRestore(session.Snapshot{ID: "s3", Workspace: sessionfixture.MustWorkspace("/b/other"), UpdatedAt: t0.Add(time.Hour)}),
	})
	if len(workspaces) != 2 {
		t.Fatalf("workspaces = %d, want 2", len(workspaces))
	}
	if workspaces[0].Path != "/a/proj" || workspaces[0].Name != "proj" || workspaces[0].SessionCount != 2 {
		t.Fatalf("first workspace = %+v", workspaces[0])
	}
	if !workspaces[0].LastActiveAt.Equal(t0.Add(2 * time.Hour)) {
		t.Fatalf("last active = %v", workspaces[0].LastActiveAt)
	}
	if workspaces[1].Path != "/b/other" || workspaces[1].SessionCount != 1 {
		t.Fatalf("second workspace = %+v", workspaces[1])
	}
}

func TestAgentDocsPreservesDiscoveryProvenance(t *testing.T) {
	finder := staticAgentDocFinder{files: []AgentDocFile{
		{Path: "/home/.lyra/AGENTS.md", Content: "home", Scope: AgentDocScopeHome},
		{Path: "/repo/AGENTS.md", Content: "root", Scope: AgentDocScopeProjectRoot},
		{Path: "/repo/pkg/AGENTS.md", Content: "leaf", Scope: AgentDocScopeCWD},
	}}
	discovery := NewDiscovery(NewScope("", "/home", testPaths{}), nil, finder, nil)

	docs, err := discovery.AgentDocs(t.Context(), "/repo/pkg")
	if err != nil {
		t.Fatalf("AgentDocs: %v", err)
	}
	if len(docs) != 3 || docs[0].Scope != AgentDocScopeHome || docs[1].Scope != AgentDocScopeProjectRoot || docs[2].Scope != AgentDocScopeCWD {
		t.Fatalf("AgentDocs = %+v, want finder scopes unchanged", docs)
	}
}

func TestAgentDocsRejectsUnknownDiscoveryProvenance(t *testing.T) {
	finder := staticAgentDocFinder{files: []AgentDocFile{{Path: "/repo/AGENTS.md", Content: "rule", Scope: "other"}}}
	discovery := NewDiscovery(NewScope("", "/home", testPaths{}), nil, finder, nil)

	if _, err := discovery.AgentDocs(t.Context(), "/repo"); err == nil {
		t.Fatal("AgentDocs accepted an unknown scope")
	}
}

// TestFilePagesUseATotalOrderAndBindTheCompleteQuery covers the workspace.files
// query properties: directories precede files and paths make the order total; a
// next page seeks strictly past the previous sort key even if that row was
// deleted; and every normalized filter belongs to the cursor identity, so a
// cursor cannot silently continue a different workspace listing.
func TestFilePagesUseATotalOrderAndBindTheCompleteQuery(t *testing.T) {
	filters := []string{"/repo", "", "", "true", "false"}
	entries := []FileEntry{
		{Path: "c.txt", Kind: FileEntryFile},
		{Path: "docs", Kind: FileEntryDir},
		{Path: "a.txt", Kind: FileEntryFile},
		{Path: "b.txt", Kind: FileEntryFile},
	}

	first, cursor, err := pageFileEntries(entries, filters, "", 2)
	if err != nil {
		t.Fatalf("first page: %v", err)
	}
	if len(first) != 2 || first[0].Path != "docs" || first[1].Path != "a.txt" || cursor == "" {
		t.Fatalf("first page = %+v, cursor %q; want docs, a.txt and a cursor", first, cursor)
	}

	// a.txt was the anchor and disappears between reads. Continuation uses its
	// sort-key value, not row existence, so b.txt and c.txt remain reachable.
	second, next, err := pageFileEntries([]FileEntry{
		{Path: "c.txt", Kind: FileEntryFile},
		{Path: "docs", Kind: FileEntryDir},
		{Path: "b.txt", Kind: FileEntryFile},
	}, filters, cursor, 2)
	if err != nil {
		t.Fatalf("second page: %v", err)
	}
	if len(second) != 2 || second[0].Path != "b.txt" || second[1].Path != "c.txt" || next != "" {
		t.Fatalf("second page = %+v, cursor %q; want b.txt, c.txt and no cursor", second, next)
	}

	otherQuery := []string{"/repo", "docs", "", "true", "false"}
	if _, _, err := pageFileEntries(entries, otherQuery, cursor, 2); !errors.Is(err, ErrPageCursor) {
		t.Fatalf("cross-query cursor err = %v, want ErrPageCursor", err)
	}
}

package arch

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"maps"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/dispatch"
)

// TestPageCursorsBindToTheirOwnMethod is the structural half of contract §11.4
// gate 11: each seek-paged read binds its cursors to its own query.
//
// The paging component already refuses a cursor minted by another query — that is
// what the namespace in the token is for. What it cannot see is a READER that names
// the wrong namespace, and there are two ways to do that. Naming a query nothing
// publishes makes "does this reader bind its own method" unanswerable without
// knowing the exceptions; naming the SAME namespace as another read is worse, because
// then two queries accept each other's anchors and the seek lands in the wrong
// ordering.
//
// The constants are read out of source because a Go const block cannot be enumerated
// at runtime — the same reason, and the same mechanism, as the wire enums and the
// capability keys.
func TestPageCursorsBindToTheirOwnMethod(t *testing.T) {
	root := moduleRoot(t)
	namespaces := pageCursorNamespaces(t, root)

	// Five reads page by seeking today. The floor is here so the gate cannot pass by
	// finding nothing — a renamed convention would otherwise read as compliance.
	if len(namespaces) < 5 {
		t.Fatalf("found %d page cursor namespaces, want every seek-paged read's: %v", len(namespaces), namespaces)
	}

	published := dispatch.Contract().Names()
	seen := make(map[string]string, len(namespaces))
	for name, method := range namespaces {
		if !slices.Contains(published, method) {
			t.Errorf("%s binds its cursors to %q, which no registered method serves", name, method)
		}
		if owner, taken := seen[method]; taken {
			t.Errorf("%s and %s both bind their cursors to %q — each would continue the other's page", name, owner, method)
			continue
		}
		seen[method] = name
	}
}

// pageCursorNamespaces reads every `*PageMethod` string constant in the module,
// keyed by constant name.
func pageCursorNamespaces(t *testing.T, root string) map[string]string {
	t.Helper()

	out := make(map[string]string)
	err := filepath.WalkDir(filepath.Join(root, "internal"), func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		file, parseErr := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if parseErr != nil {
			t.Fatalf("parse %s: %v", path, parseErr)
		}
		for _, decl := range file.Decls {
			group, ok := decl.(*ast.GenDecl)
			if !ok || group.Tok != token.CONST {
				continue
			}
			for _, spec := range group.Specs {
				value, ok := spec.(*ast.ValueSpec)
				if !ok || len(value.Names) != 1 || len(value.Values) != 1 {
					continue
				}
				name := value.Names[0].Name
				if !strings.HasSuffix(name, "PageMethod") {
					continue
				}
				literal, ok := value.Values[0].(*ast.BasicLit)
				if !ok || literal.Kind != token.STRING {
					t.Errorf("%s is a page cursor namespace and not a string literal", name)
					continue
				}
				text, unquoteErr := strconv.Unquote(literal.Value)
				if unquoteErr != nil {
					t.Fatalf("unquote %s: %v", literal.Value, unquoteErr)
				}
				out[name] = text
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk internal: %v", err)
	}
	return out
}

// TestEverySeekPagedReadHasQueryFixtures is the behavioral half of gate 11: every
// seek-paged read has a fixture for each of the three properties the contract names.
//
// The component they share already proves all three generically, which is exactly
// why that is not enough — the question is whether each READER wired them. A reader
// that seeks with the wrong anchor still returns rows, in an order nobody notices is
// wrong until a page repeats or skips one.
//
// The index is read backwards too: the fixture's doc comment has to name the query it
// pages, so a test that stops covering a read cannot keep standing in for it.
func TestEverySeekPagedReadHasQueryFixtures(t *testing.T) {
	root := moduleRoot(t)

	for _, method := range pageCursorNamespaces(t, root) {
		covered, ok := pageFixtures[method]
		if !ok {
			t.Errorf("%s pages by seeking and has no query fixtures", method)
			continue
		}
		for _, property := range [...]queryProperty{fixedOrder, cursorBinding, pageDirection} {
			fixture, ok := covered[property]
			if !ok {
				t.Errorf("%s has no fixture for %s", method, property)
				continue
			}
			assertFixtureProves(t, root, fixture, method)
		}
	}
	for method := range pageFixtures {
		if !slices.Contains(slices.Collect(maps.Values(pageCursorNamespaces(t, root))), method) {
			t.Errorf("fixtures claim to page %s, which no read binds its cursors to", method)
		}
	}
}

// queryProperty is one of the three things contract §11.4 gate 11 asks a list query
// fixture to establish.
type queryProperty string

const (
	// fixedOrder: the rows come back in one defined order, total enough that no two
	// can trade places between pages.
	fixedOrder queryProperty = "fixed order"
	// cursorBinding: an anchor only continues the query and scope that minted it.
	cursorBinding queryProperty = "cursor binding"
	// pageDirection: the next page starts strictly past the last row of this one, and
	// says there is more only when the read found more.
	pageDirection queryProperty = "next-page direction"
)

var pageFixtures = map[string]map[queryProperty]fixtureRef{
	"items.list": {
		fixedOrder:    {"internal/application/queries", "TestListItemPageBoundsTheQueryAndSeeksPastTheAnchor"},
		pageDirection: {"internal/application/queries", "TestListItemPageBoundsTheQueryAndSeeksPastTheAnchor"},
		cursorBinding: {"internal/application/queries", "TestListItemPageRefusesAForeignCursor"},
	},
	"runs.list": {
		fixedOrder:    {"internal/application/queries", "TestListRunPageWalksBackwardThroughHistory"},
		pageDirection: {"internal/application/queries", "TestListRunPageWalksBackwardThroughHistory"},
		cursorBinding: {"internal/application/queries", "TestListRunPageRefusesACursorFromAnotherQuery"},
	},
	"interrupts.list": {
		fixedOrder:    {"internal/application/queries", "TestListPendingInterruptPagePagesOldestFirst"},
		pageDirection: {"internal/application/queries", "TestListPendingInterruptPagePagesOldestFirst"},
		cursorBinding: {"internal/application/queries", "TestListPendingInterruptPagePagesOldestFirst"},
	},
	"sessions.list": {
		fixedOrder:    {"internal/application/sessions", "TestListViewPagePagesInAFixedOrderAndRefusesAForeignCursor"},
		pageDirection: {"internal/application/sessions", "TestListViewPagePagesInAFixedOrderAndRefusesAForeignCursor"},
		cursorBinding: {"internal/application/sessions", "TestListViewPagePagesInAFixedOrderAndRefusesAForeignCursor"},
	},
	"schedules.list": {
		fixedOrder:    {"internal/application/schedules", "TestListPagePagesNewestFirstAndRefusesAForeignCursor"},
		pageDirection: {"internal/application/schedules", "TestListPagePagesNewestFirstAndRefusesAForeignCursor"},
		cursorBinding: {"internal/application/schedules", "TestListPagePagesNewestFirstAndRefusesAForeignCursor"},
	},
}

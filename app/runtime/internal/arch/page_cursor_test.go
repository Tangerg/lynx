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
)

// TestPageCursorsHaveUniqueSemanticNamespaces is the structural half of contract
// §11.4 gate 11: each seek-paged read binds its cursors to its own application
// namespace.
//
// The application pagination capability already refuses a cursor minted by another query — that is
// what the namespace in the token is for. What it cannot see is two readers sharing
// a namespace, which would let each continue the other's cursor in a different
// ordering or filter scope.
//
// The constants are read out of source because a Go const block cannot be enumerated
// at runtime.
func TestPageCursorsHaveUniqueSemanticNamespaces(t *testing.T) {
	root := moduleRoot(t)
	namespaces := pageCursorNamespaces(t, root)
	paginationPath := filepath.Join(root, "internal", "application", "pagination", "pagination.go")
	paginationFile, err := parser.ParseFile(token.NewFileSet(), paginationPath, nil, 0)
	if err != nil {
		t.Fatalf("parse pagination cursor: %v", err)
	}
	if fields := structFields(paginationFile, "token"); !slices.Equal(fields, []string{"Version", "Namespace", "Filters", "Key"}) {
		t.Fatalf("pagination token fields = %v, want semantic namespace ownership", fields)
	}

	// Six reads page by seeking today. The floor is here so the gate cannot pass by
	// finding nothing — a renamed convention would otherwise read as compliance.
	if len(namespaces) < 6 {
		t.Fatalf("found %d page cursor namespaces, want every seek-paged read's: %v", len(namespaces), namespaces)
	}

	seen := make(map[string]string, len(namespaces))
	for name, namespace := range namespaces {
		if namespace == "" {
			t.Errorf("%s has an empty page cursor namespace", name)
			continue
		}
		if strings.HasSuffix(namespace, ".list") {
			t.Errorf("%s uses transport method-shaped namespace %q", name, namespace)
		}
		if owner, taken := seen[namespace]; taken {
			t.Errorf("%s and %s both bind their cursors to %q — each would continue the other's page", name, owner, namespace)
			continue
		}
		seen[namespace] = name
	}
}

// pageCursorNamespaces reads every `*PageNamespace` string constant in the
// module, keyed by constant name.
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
		collectPageCursorNamespaces(t, file, out)
		return nil
	})
	if err != nil {
		t.Fatalf("walk internal: %v", err)
	}
	return out
}

func collectPageCursorNamespaces(t *testing.T, file *ast.File, namespaces map[string]string) {
	t.Helper()
	for _, declaration := range file.Decls {
		constants, isConstantDeclaration := declaration.(*ast.GenDecl)
		if !isConstantDeclaration || constants.Tok != token.CONST {
			continue
		}
		for _, spec := range constants.Specs {
			collectPageCursorNamespace(t, spec, namespaces)
		}
	}
}

func collectPageCursorNamespace(t *testing.T, spec ast.Spec, namespaces map[string]string) {
	t.Helper()
	value, isSingleValue := spec.(*ast.ValueSpec)
	if !isSingleValue || len(value.Names) != 1 || len(value.Values) != 1 {
		return
	}
	name := value.Names[0].Name
	if !strings.HasSuffix(name, "PageNamespace") {
		return
	}
	literal, isStringLiteral := value.Values[0].(*ast.BasicLit)
	if !isStringLiteral || literal.Kind != token.STRING {
		t.Errorf("%s is a page cursor namespace and not a string literal", name)
		return
	}
	namespace, err := strconv.Unquote(literal.Value)
	if err != nil {
		t.Fatalf("unquote %s: %v", literal.Value, err)
	}
	namespaces[name] = namespace
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

	for _, namespace := range pageCursorNamespaces(t, root) {
		covered, ok := pageFixtures[namespace]
		if !ok {
			t.Errorf("%s pages by seeking and has no query fixtures", namespace)
			continue
		}
		for _, property := range [...]queryProperty{fixedOrder, cursorBinding, pageDirection} {
			fixture, ok := covered[property]
			if !ok {
				t.Errorf("%s has no fixture for %s", namespace, property)
				continue
			}
			assertFixtureProves(t, root, fixture, namespace)
		}
	}
	for namespace := range pageFixtures {
		if !slices.Contains(slices.Collect(maps.Values(pageCursorNamespaces(t, root))), namespace) {
			t.Errorf("fixtures claim to page %s, which no read binds its cursors to", namespace)
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
	"items": {
		fixedOrder:    {"internal/application/queries", "TestListItemPageBoundsTheQueryAndSeeksPastTheAnchor"},
		pageDirection: {"internal/application/queries", "TestListItemPageBoundsTheQueryAndSeeksPastTheAnchor"},
		cursorBinding: {"internal/application/queries", "TestListItemPageRefusesAForeignCursor"},
	},
	"runs": {
		fixedOrder:    {"internal/application/queries", "TestListRunPageWalksBackwardThroughHistory"},
		pageDirection: {"internal/application/queries", "TestListRunPageWalksBackwardThroughHistory"},
		cursorBinding: {"internal/application/queries", "TestListRunPageRefusesACursorFromAnotherQuery"},
	},
	"interrupts": {
		fixedOrder:    {"internal/application/queries", "TestListPendingInterruptPagePagesOldestFirst"},
		pageDirection: {"internal/application/queries", "TestListPendingInterruptPagePagesOldestFirst"},
		cursorBinding: {"internal/application/queries", "TestListPendingInterruptPagePagesOldestFirst"},
	},
	"sessions": {
		fixedOrder:    {"internal/application/sessions", "TestListViewPagePagesInAFixedOrderAndRefusesAForeignCursor"},
		pageDirection: {"internal/application/sessions", "TestListViewPagePagesInAFixedOrderAndRefusesAForeignCursor"},
		cursorBinding: {"internal/application/sessions", "TestListViewPagePagesInAFixedOrderAndRefusesAForeignCursor"},
	},
	"schedules": {
		fixedOrder:    {"internal/application/schedules", "TestListPagePagesNewestFirstAndRefusesAForeignCursor"},
		pageDirection: {"internal/application/schedules", "TestListPagePagesNewestFirstAndRefusesAForeignCursor"},
		cursorBinding: {"internal/application/schedules", "TestListPagePagesNewestFirstAndRefusesAForeignCursor"},
	},
	"workspace.files": {
		fixedOrder:    {"internal/application/workspace", "TestFilePagesUseATotalOrderAndBindTheCompleteQuery"},
		pageDirection: {"internal/application/workspace", "TestFilePagesUseATotalOrderAndBindTheCompleteQuery"},
		cursorBinding: {"internal/application/workspace", "TestFilePagesUseATotalOrderAndBindTheCompleteQuery"},
	},
}

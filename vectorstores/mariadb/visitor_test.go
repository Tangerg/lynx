package mariadb_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/core/vectorstore/filter"
	"github.com/Tangerg/lynx/internal/vectorstorekit/storetest"
	"github.com/Tangerg/lynx/vectorstores/mariadb"
)

func TestVisitor_Conformance(t *testing.T) {
	storetest.VisitorConformance(t, func(src string) error {
		expr, err := filter.Parse(src)
		if err != nil {
			return err
		}
		v := mariadb.NewVisitor("metadata")
		return v.Visit(expr)
	})
}

// build is the test driver — parse src, visit, return (sql, args, err).
func build(t *testing.T, src string) (string, []any, error) {
	t.Helper()
	expr, err := filter.Parse(src)
	if err != nil {
		return "", nil, err
	}
	v := mariadb.NewVisitor("metadata")
	if err := v.Visit(expr); err != nil {
		return "", nil, err
	}
	sql, args := v.Result()
	return sql, args, nil
}

func TestVisitor_IsNull(t *testing.T) {
	sql, args, err := build(t, `author is null`)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if !strings.Contains(sql, "JSON_VALUE(metadata, '$.author')") || !strings.Contains(sql, "IS NULL") {
		t.Fatalf("sql=%q must contain JSON_VALUE(metadata, '$.author') IS NULL", sql)
	}
	if len(args) != 0 {
		t.Fatalf("IS NULL takes no bound args, got %v", args)
	}
}

func TestVisitor_IsNotNull(t *testing.T) {
	sql, _, err := build(t, `author is not null`)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	// NOT(field IS NULL) — semantically IS NOT NULL.
	if !strings.Contains(sql, "NOT") || !strings.Contains(sql, "IS NULL") {
		t.Fatalf("sql=%q must wrap IS NULL in NOT", sql)
	}
}

func TestVisitor_CollectionMembershipPreservesJSONType(t *testing.T) {
	tests := []struct {
		name      string
		predicate filter.Predicate
		wantSQL   string
		wantArgs  []any
	}{
		{name: "string", predicate: filter.Has("visible_to", "user-42"), wantSQL: "JSON_CONTAINS(metadata, JSON_ARRAY(?), '$.visible_to')", wantArgs: []any{"user-42"}},
		{name: "boolean", predicate: filter.Has("visible_to", true), wantSQL: "JSON_CONTAINS(metadata, JSON_ARRAY(true), '$.visible_to')"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			visitor := mariadb.NewVisitor("metadata")
			if err := visitor.Visit(test.predicate); err != nil {
				t.Fatal(err)
			}
			sql, args := visitor.Result()
			if sql != test.wantSQL {
				t.Fatalf("sql = %q, want %q", sql, test.wantSQL)
			}
			if !reflect.DeepEqual(args, test.wantArgs) {
				t.Fatalf("args = %#v, want %#v", args, test.wantArgs)
			}
		})
	}
}

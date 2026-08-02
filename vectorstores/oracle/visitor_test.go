package oracle_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/core/vectorstore/filter"
	"github.com/Tangerg/lynx/internal/vectorstorekit/storetest"
	"github.com/Tangerg/lynx/vectorstores/oracle"
)

func TestVisitor_Conformance(t *testing.T) {
	storetest.VisitorConformance(t, func(src string) error {
		expr, err := filter.Parse(src)
		if err != nil {
			return err
		}
		v := oracle.NewVisitor("metadata")
		return v.Visit(expr)
	})
}

func TestVisitor_CollectionMembershipUsesJSONExists(t *testing.T) {
	sql, args, err := build(t, `profile['tags'] has 'rag'`)
	if err != nil {
		t.Fatal(err)
	}
	want := `json_exists(metadata, '$.profile.tags[*]?(@ == $member)' PASSING :1 AS "member")`
	if sql != want {
		t.Fatalf("sql = %q, want %q", sql, want)
	}
	if !reflect.DeepEqual(args, []any{"rag"}) {
		t.Fatalf("args = %#v", args)
	}
}

func TestVisitor_CollectionMembershipRejectsBoolean(t *testing.T) {
	visitor := oracle.NewVisitor("metadata")
	if err := visitor.Visit(filter.Has("flags", true)); err == nil {
		t.Fatal("Visit() error = nil, want Oracle PASSING boolean limitation")
	}
}

// build is the test driver — parse src, visit, return (sql, args, err).
func build(t *testing.T, src string) (string, []any, error) {
	t.Helper()
	expr, err := filter.Parse(src)
	if err != nil {
		return "", nil, err
	}
	v := oracle.NewVisitor("metadata")
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
	if !strings.Contains(sql, "json_value(metadata, '$.author')") || !strings.Contains(sql, "IS NULL") {
		t.Fatalf("sql=%q must contain json_value(metadata, '$.author') IS NULL", sql)
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

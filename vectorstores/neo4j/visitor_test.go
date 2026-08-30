package neo4j

import (
	"strings"
	"testing"

	"github.com/Tangerg/scope/core/vectorstore/filter"
)

// build is the test driver — parse src, visit, return (cypher, params, err).
func build(t *testing.T, src string) (string, map[string]any, error) {
	t.Helper()
	expr, err := filter.Parse(src)
	if err != nil {
		return "", nil, err
	}
	v := newVisitor("node", "metadata")
	if err := expr.Accept(v); err != nil {
		return "", nil, err
	}
	cypher, params := v.snapshot()
	return cypher, params, nil
}

func TestVisitor_IsNull(t *testing.T) {
	cypher, params, err := build(t, `author is null`)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if !strings.Contains(cypher, "node.`metadata.author`") || !strings.Contains(cypher, "IS NULL") {
		t.Fatalf("cypher=%q must contain node.`metadata.author` IS NULL", cypher)
	}
	if len(params) != 0 {
		t.Fatalf("IS NULL takes no bound params, got %v", params)
	}
}

func TestVisitor_IsNotNull(t *testing.T) {
	cypher, _, err := build(t, `author is not null`)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	// NOT (field IS NULL) — Cypher treats this as equivalent to IS NOT NULL.
	if !strings.Contains(cypher, "NOT") || !strings.Contains(cypher, "IS NULL") {
		t.Fatalf("cypher=%q must wrap IS NULL in NOT", cypher)
	}
}

func TestVisitor_CollectionMembershipReversesInOperands(t *testing.T) {
	cypher, params, err := build(t, `visible_to has 'user-42'`)
	if err != nil {
		t.Fatal(err)
	}
	if want := "$p1 IN node.`metadata.visible_to`"; cypher != want {
		t.Fatalf("cypher = %q, want %q", cypher, want)
	}
	if params["p1"] != "user-42" {
		t.Fatalf("params = %#v", params)
	}
}

package neo4j_test

import (
	"strings"
	"testing"

	"github.com/Tangerg/lynx/core/vectorstore/filter"
	"github.com/Tangerg/lynx/vectorstores/neo4j"
)

// build is the test driver — parse src, visit, return (cypher, params, err).
func build(t *testing.T, src string) (string, map[string]any, error) {
	t.Helper()
	expr, err := filter.ParseAndAnalyze(src)
	if err != nil {
		return "", nil, err
	}
	v := neo4j.NewVisitor("node", "metadata")
	if err := v.Visit(expr); err != nil {
		return "", nil, err
	}
	cypher, params := v.Result()
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

package codebase

import (
	"math"
	"testing"
	"time"
)

func TestStatusOwnsLifecycleInvariants(t *testing.T) {
	now := time.Now()
	for name, status := range map[string]Status{
		"known":     {State: Ready, FileCount: 2, ChunkCount: 4, IndexedAt: &now},
		"indexing":  {State: Indexing, OperationID: "op_1"},
		"bad state": {State: "mystery"},
		"negative":  {State: Ready, FileCount: -1},
		// A newly admitted background operation can be visible before the indexer
		// publishes its first indexing state.
		"admitted": {State: Ready, OperationID: "op_1"},
	} {
		err := status.Validate()
		if (name == "known" || name == "indexing" || name == "admitted") != (err == nil) {
			t.Fatalf("%s validation = %v", name, err)
		}
	}
}

func TestHitRejectsInvalidSpanAndScore(t *testing.T) {
	if err := (Hit{Path: "main.go", StartLine: 2, EndLine: 4, Score: .8}).Validate(); err != nil {
		t.Fatalf("valid hit: %v", err)
	}
	if err := (Hit{Path: "main.go", StartLine: 4, EndLine: 2, Score: .8}).Validate(); err == nil {
		t.Fatal("accepted reversed span")
	}
	if err := (Hit{Path: "main.go", StartLine: 1, EndLine: 1, Score: 1.1}).Validate(); err == nil {
		t.Fatal("accepted invalid score")
	}
	if err := (Hit{Path: "main.go", StartLine: 1, EndLine: 1, Score: math.NaN()}).Validate(); err == nil {
		t.Fatal("accepted NaN score")
	}
}

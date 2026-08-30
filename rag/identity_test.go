package rag_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/scope/core/document"
	"github.com/Tangerg/scope/rag"
)

func testQuery(t *testing.T) rag.Query {
	t.Helper()
	query, err := rag.NewQuery("what is GOAP?")
	if err != nil {
		t.Fatal(err)
	}
	return query
}

// TestIdentityStagesValidateTheirInput is the reason optional stages use an
// explicit identity rather than a skipped nil: an identity is still a stage, so
// it must reject an invalid query exactly like a real one. A stage that passed
// anything through would let a malformed query reach the retriever.
func TestIdentityStagesValidateTheirInput(t *testing.T) {
	var invalid rag.Query
	valid := testQuery(t)

	t.Run("transformer", func(t *testing.T) {
		if _, err := rag.IdentityTransformer().Transform(t.Context(), invalid); err == nil {
			t.Fatal("an invalid query passed through")
		}
		transformed, err := rag.IdentityTransformer().Transform(t.Context(), valid)
		if err != nil || transformed.Text() != valid.Text() {
			t.Fatalf("Transform = %#v, %v", transformed, err)
		}
	})

	t.Run("expander", func(t *testing.T) {
		if _, err := rag.IdentityExpander().Expand(t.Context(), invalid); err == nil {
			t.Fatal("an invalid query passed through")
		}
		expanded, err := rag.IdentityExpander().Expand(t.Context(), valid)
		if err != nil {
			t.Fatal(err)
		}
		if len(expanded) != 1 || expanded[0].Text() != valid.Text() {
			t.Fatalf("Expand = %#v", expanded)
		}
	})

	t.Run("retriever", func(t *testing.T) {
		if _, err := rag.NopRetriever().Retrieve(t.Context(), invalid); err == nil {
			t.Fatal("an invalid query passed through")
		}
		candidates, err := rag.NopRetriever().Retrieve(t.Context(), valid)
		if err != nil || len(candidates) != 0 {
			t.Fatalf("Retrieve = %#v, %v", candidates, err)
		}
	})

	t.Run("refiner", func(t *testing.T) {
		if _, err := rag.IdentityRefiner().Refine(t.Context(), invalid, nil); err == nil {
			t.Fatal("an invalid query passed through")
		}
		refined, err := rag.IdentityRefiner().Refine(t.Context(), valid, nil)
		if err != nil || len(refined) != 0 {
			t.Fatalf("Refine = %#v, %v", refined, err)
		}
	})

	t.Run("augmenter", func(t *testing.T) {
		if _, err := rag.IdentityAugmenter().Augment(t.Context(), invalid, nil); err == nil {
			t.Fatal("an invalid query passed through")
		}
		augmentation, err := rag.IdentityAugmenter().Augment(t.Context(), valid, nil)
		if err != nil {
			t.Fatal(err)
		}
		if augmentation.Text() != valid.Text() {
			t.Fatalf("Augment produced %q, want the query text", augmentation.Text())
		}
	})
}

// TestIdentityStagesObserveCancellation keeps a canceled retrieval from doing
// work the caller no longer wants, even through a stage that does nothing.
func TestIdentityStagesObserveCancellation(t *testing.T) {
	valid := testQuery(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if _, err := rag.IdentityTransformer().Transform(ctx, valid); !errors.Is(err, context.Canceled) {
		t.Errorf("Transform error = %v", err)
	}
	if _, err := rag.IdentityExpander().Expand(ctx, valid); !errors.Is(err, context.Canceled) {
		t.Errorf("Expand error = %v", err)
	}
	if _, err := rag.NopRetriever().Retrieve(ctx, valid); !errors.Is(err, context.Canceled) {
		t.Errorf("Retrieve error = %v", err)
	}
	if _, err := rag.IdentityRefiner().Refine(ctx, valid, nil); !errors.Is(err, context.Canceled) {
		t.Errorf("Refine error = %v", err)
	}
	if _, err := rag.IdentityAugmenter().Augment(ctx, valid, nil); !errors.Is(err, context.Canceled) {
		t.Errorf("Augment error = %v", err)
	}
}

// TestIdentityRefinerDoesNotAliasItsInput keeps a no-op stage from handing back
// the caller's slice, which a later stage could mutate under them.
func TestIdentityRefinerDoesNotAliasItsInput(t *testing.T) {
	valid := testQuery(t)
	document, err := document.NewDocument("body", nil)
	if err != nil {
		t.Fatal(err)
	}
	candidates := rag.Candidates{{Document: document, Score: 0.5}}

	refined, err := rag.IdentityRefiner().Refine(t.Context(), valid, candidates)
	if err != nil {
		t.Fatal(err)
	}
	if len(refined) != len(candidates) {
		t.Fatalf("Refine returned %d candidates, want %d", len(refined), len(candidates))
	}
	candidates[0].Score = 99
	if refined[0].Score != 0.5 {
		t.Fatal("IdentityRefiner aliases the caller's candidates")
	}
}

// TestValueKeyRejectsUnusableNames keeps the typed query envelope from
// accepting a key that cannot be told apart in a diagnostic.
func TestValueKeyRejectsUnusableNames(t *testing.T) {
	for name, value := range map[string]string{
		"empty":               "",
		"leading whitespace":  " tenant",
		"trailing whitespace": "tenant ",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := rag.NewValueKey[string](value); !errors.Is(err, rag.ErrInvalidQueryValueKey) {
				t.Fatalf("NewValueKey(%q) error = %v", value, err)
			}
		})
	}
	if _, err := rag.NewValueKey[any]("anything"); !errors.Is(err, rag.ErrInvalidQueryValueKey) {
		t.Fatal("NewValueKey accepted an any value type")
	}
}

// TestZeroValueKeyIsUnusable proves a key must come from the constructor: an
// uninitialized key would otherwise silently share identity with every other.
func TestZeroValueKeyIsUnusable(t *testing.T) {
	var key rag.ValueKey[string]
	if key.Name() != "" {
		t.Fatalf("the zero key reports the name %q", key.Name())
	}

	query := testQuery(t)
	if _, _, err := query.Value(key); err == nil {
		t.Fatal("the zero key read a value")
	}
	if _, err := query.WithValue(key, "value"); err == nil {
		t.Fatal("the zero key stored a value")
	}
}

// TestWithTextKeepsValuesAndRejectsBlank is what makes a transformer safe: a
// rewritten query must keep the per-call context it was carrying.
func TestWithTextKeepsValuesAndRejectsBlank(t *testing.T) {
	key, err := rag.NewValueKey[string]("tenant")
	if err != nil {
		t.Fatal(err)
	}
	query, err := testQuery(t).WithValue(key, "acme")
	if err != nil {
		t.Fatal(err)
	}

	rewritten, err := query.WithText("what is goal-oriented action planning?")
	if err != nil {
		t.Fatal(err)
	}
	value, found, err := rewritten.Value(key)
	if err != nil || !found || value != "acme" {
		t.Fatalf("rewritten query lost its value: %q, %t, %v", value, found, err)
	}

	for _, blank := range []string{"", "   ", "\t\n"} {
		if _, err := query.WithText(blank); !errors.Is(err, rag.ErrInvalidQuery) {
			t.Fatalf("WithText(%q) error = %v", blank, err)
		}
	}
}

// TestWithValueRejectsNil keeps an absent value distinguishable from a stored
// nil, which a consumer would otherwise read as present.
func TestWithValueRejectsNil(t *testing.T) {
	key, err := rag.NewValueKey[[]string]("domains")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := testQuery(t).WithValue(key, nil); !errors.Is(err, rag.ErrNilQueryValue) {
		t.Fatalf("WithValue(nil) error = %v", err)
	}
}

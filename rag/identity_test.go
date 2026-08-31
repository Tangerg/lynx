package rag_test

import (
	"context"
	"errors"
	"testing"

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

func TestIdentityAugmenterValidatesItsInput(t *testing.T) {
	var invalid rag.Query
	valid := testQuery(t)
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
}

func TestIdentityAugmenterObservesCancellation(t *testing.T) {
	valid := testQuery(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if _, err := rag.IdentityAugmenter().Augment(ctx, valid, nil); !errors.Is(err, context.Canceled) {
		t.Errorf("Augment error = %v", err)
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

package fs

import (
	"os"
	"strings"
	"testing"
)

// editFuzz runs LocalExecutor.Edit against a temp file seeded with content and
// returns the resulting file text + error.
func editFuzz(t *testing.T, content, old, new string, replaceAll bool) (string, error) {
	t.Helper()
	dir := t.TempDir()
	path := writeTemp(t, dir, "f.go", content)
	_, err := NewLocalExecutor(dir).Edit(t.Context(), EditInput{
		Path: path, OldString: old, NewString: new, ReplaceAll: replaceAll,
	})
	got, _ := os.ReadFile(path)
	return string(got), err
}

func TestEdit_FuzzyIndentationDrift(t *testing.T) {
	// File is indented with a tab; the model copied the body with 4 spaces.
	content := "func f() {\n\treturn 1\n}\n"
	got, err := editFuzz(t, content, "    return 1", "    return 2", false)
	if err != nil {
		t.Fatalf("Edit: %v", err)
	}
	// The original (tab-indented) region is replaced with the model's new text.
	if got != "func f() {\n    return 2\n}\n" {
		t.Fatalf("file = %q", got)
	}
}

func TestEdit_FuzzyTrailingWhitespace(t *testing.T) {
	content := "alpha   \nbeta\n" // alpha has trailing spaces
	got, err := editFuzz(t, content, "alpha\nbeta", "x\ny", false)
	if err != nil {
		t.Fatalf("Edit: %v", err)
	}
	if got != "x\ny\n" {
		t.Fatalf("file = %q", got)
	}
}

func TestEdit_FuzzyInternalWhitespace(t *testing.T) {
	content := "total = a  +  b\n" // collapsed-whitespace variant
	got, err := editFuzz(t, content, "total = a + b", "total = a - b", false)
	if err != nil {
		t.Fatalf("Edit: %v", err)
	}
	if got != "total = a - b\n" {
		t.Fatalf("file = %q", got)
	}
}

func TestEdit_FuzzyAmbiguousRefused(t *testing.T) {
	// Two 3-line blocks match `old` apart from the bar line's indentation, and
	// neither matches exactly (old's bar has no indent). Must refuse, not guess.
	content := "foo\n\tbar\nbaz\nfoo\n    bar\nbaz\n"
	old := "foo\nbar\nbaz"
	_, err := editFuzz(t, content, old, "X\nY\nZ", false)
	if err == nil || !strings.Contains(err.Error(), "regions match") {
		t.Fatalf("want ambiguous-refusal error, got %v", err)
	}
}

func TestEdit_NotFoundStillFails(t *testing.T) {
	_, err := editFuzz(t, "alpha\n", "zeta", "x", false)
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("want not-found error, got %v", err)
	}
}

func TestEdit_ExactStillPreferred(t *testing.T) {
	// An exact match must take the exact path (unaffected by the fuzzy fallback).
	got, err := editFuzz(t, "x = 1\nx = 1\n", "x = 1", "x = 2", true)
	if err != nil {
		t.Fatalf("Edit: %v", err)
	}
	if got != "x = 2\nx = 2\n" {
		t.Fatalf("file = %q", got)
	}
}

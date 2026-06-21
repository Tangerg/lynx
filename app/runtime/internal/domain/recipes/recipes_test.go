package recipes

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// write creates dir/<name> with content, failing the test on error.
func write(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func find(recipes []Recipe, name string) (Recipe, bool) {
	for _, r := range recipes {
		if r.Name == name {
			return r, true
		}
	}
	return Recipe{}, false
}

// TestList covers the merge/precedence/sort contract and frontmatter parsing:
// project layers over global (winning a name collision), entries sort by name,
// and frontmatter is optional.
func TestList(t *testing.T) {
	project := filepath.Join(t.TempDir(), ".lyra", "recipes")
	global := t.TempDir()

	write(t, global, "review.md", "---\ndescription: global review\n---\nGlobal review body $ARGUMENTS")
	write(t, global, "explain.md", "---\ndescription: explain a thing\nargumentHint: \"[symbol]\"\n---\nExplain $1")
	// Project's review.md shadows the global one (same name → project wins).
	write(t, project, "review.md", "---\ndescription: project review\n---\nProject review body")
	// A bare prompt with no frontmatter is a valid recipe.
	write(t, project, "commit.md", "Write a conventional commit for the staged changes.")
	// Non-recipe entries are ignored.
	write(t, project, "notes.txt", "not a recipe")
	write(t, project, ".hidden.md", "dotfile, skipped")

	got, err := List(context.Background(), project, global)
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	// Sorted by name: commit, explain, review (notes.txt / .hidden.md excluded).
	wantNames := []string{"commit", "explain", "review"}
	if len(got) != len(wantNames) {
		t.Fatalf("got %d recipes %v, want %d %v", len(got), names(got), len(wantNames), wantNames)
	}
	for i, n := range wantNames {
		if got[i].Name != n {
			t.Errorf("recipe[%d].Name = %q, want %q", i, got[i].Name, n)
		}
	}

	// Project wins the collision: review carries the project body + description.
	review, _ := find(got, "review")
	if review.Scope != ScopeProject {
		t.Errorf("review.Scope = %q, want %q", review.Scope, ScopeProject)
	}
	if review.Description != "project review" {
		t.Errorf("review.Description = %q, want project copy", review.Description)
	}
	if review.Body != "Project review body" {
		t.Errorf("review.Body = %q, want project copy", review.Body)
	}

	// Frontmatter parses; argumentHint is optional.
	explain, _ := find(got, "explain")
	if explain.Scope != ScopeGlobal {
		t.Errorf("explain.Scope = %q, want %q", explain.Scope, ScopeGlobal)
	}
	if explain.Description != "explain a thing" || explain.ArgumentHint != "[symbol]" {
		t.Errorf("explain frontmatter = %q / %q, want \"explain a thing\" / \"[symbol]\"", explain.Description, explain.ArgumentHint)
	}
	if explain.Body != "Explain $1" {
		t.Errorf("explain.Body = %q, want \"Explain $1\"", explain.Body)
	}

	// Bare prompt: no frontmatter → empty metadata, whole content is the body.
	commit, _ := find(got, "commit")
	if commit.Description != "" || commit.ArgumentHint != "" {
		t.Errorf("commit metadata = %q / %q, want empty", commit.Description, commit.ArgumentHint)
	}
	if commit.Body != "Write a conventional commit for the staged changes." {
		t.Errorf("commit.Body = %q, want the whole file", commit.Body)
	}
}

// TestListMissingDirs: absent directories contribute nothing rather than
// erroring (a fresh install with no recipes lists empty).
func TestListMissingDirs(t *testing.T) {
	got, err := List(context.Background(), filepath.Join(t.TempDir(), "nope"), filepath.Join(t.TempDir(), "nope"))
	if err != nil {
		t.Fatalf("List on missing dirs: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("got %d recipes, want 0", len(got))
	}
}

// TestParseDegradesToBody: a malformed or unterminated frontmatter fence keeps
// the recipe (whole document becomes the body) rather than dropping it.
func TestParseDegradesToBody(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "broken.md", "---\nthis is: not: valid: yaml\nunterminated")

	got, err := List(context.Background(), dir, "")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	r, ok := find(got, "broken")
	if !ok {
		t.Fatal("broken recipe was dropped; want it kept with the whole file as body")
	}
	if r.Description != "" {
		t.Errorf("broken.Description = %q, want empty (no parsed frontmatter)", r.Description)
	}
}

func names(rs []Recipe) []string {
	out := make([]string, len(rs))
	for i, r := range rs {
		out[i] = r.Name
	}
	return out
}

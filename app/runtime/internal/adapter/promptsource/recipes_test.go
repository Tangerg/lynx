package promptsource

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	workspaceapp "github.com/Tangerg/lynx/app/runtime/internal/application/workspace"
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

func findRecipe(rs []workspaceapp.Recipe, name string) (workspaceapp.Recipe, bool) {
	for _, r := range rs {
		if r.Name == name {
			return r, true
		}
	}
	return workspaceapp.Recipe{}, false
}

func recipeNames(rs []workspaceapp.Recipe) []string {
	out := make([]string, len(rs))
	for i, r := range rs {
		out[i] = r.Name
	}
	return out
}

// TestListRecipes covers the merge/precedence/sort contract and frontmatter
// parsing: project layers over global (winning a name collision), entries sort
// by name, and frontmatter is optional.
func TestListRecipes(t *testing.T) {
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

	got, err := listRecipes(context.Background(), project, global)
	if err != nil {
		t.Fatalf("ListRecipes: %v", err)
	}

	// Sorted by name: commit, explain, review (notes.txt / .hidden.md excluded).
	wantNames := []string{"commit", "explain", "review"}
	if len(got) != len(wantNames) {
		t.Fatalf("got %d recipes %v, want %d %v", len(got), recipeNames(got), len(wantNames), wantNames)
	}
	for i, n := range wantNames {
		if got[i].Name != n {
			t.Errorf("recipe[%d].Name = %q, want %q", i, got[i].Name, n)
		}
	}

	// Project wins the collision: review carries the project body + description.
	review, _ := findRecipe(got, "review")
	if review.Scope != workspaceapp.RecipeScopeProject {
		t.Errorf("review.Scope = %q, want %q", review.Scope, workspaceapp.RecipeScopeProject)
	}
	if review.Description != "project review" {
		t.Errorf("review.Description = %q, want project copy", review.Description)
	}
	if review.Body != "Project review body" {
		t.Errorf("review.Body = %q, want project copy", review.Body)
	}

	// Frontmatter parses; argumentHint is optional.
	explain, _ := findRecipe(got, "explain")
	if explain.Scope != workspaceapp.RecipeScopeGlobal {
		t.Errorf("explain.Scope = %q, want %q", explain.Scope, workspaceapp.RecipeScopeGlobal)
	}
	if explain.Description != "explain a thing" || explain.ArgumentHint != "[symbol]" {
		t.Errorf("explain frontmatter = %q / %q, want \"explain a thing\" / \"[symbol]\"", explain.Description, explain.ArgumentHint)
	}
	if explain.Body != "Explain $1" {
		t.Errorf("explain.Body = %q, want \"Explain $1\"", explain.Body)
	}

	// Bare prompt: no frontmatter → empty metadata, whole content is the body.
	commit, _ := findRecipe(got, "commit")
	if commit.Description != "" || commit.ArgumentHint != "" {
		t.Errorf("commit metadata = %q / %q, want empty", commit.Description, commit.ArgumentHint)
	}
	if commit.Body != "Write a conventional commit for the staged changes." {
		t.Errorf("commit.Body = %q, want the whole file", commit.Body)
	}
}

// TestListRecipesMissingDirs: absent directories contribute nothing rather than
// erroring (a fresh install with no recipes lists empty).
func TestListRecipesMissingDirs(t *testing.T) {
	got, err := listRecipes(context.Background(), filepath.Join(t.TempDir(), "nope"), filepath.Join(t.TempDir(), "nope"))
	if err != nil {
		t.Fatalf("ListRecipes on missing dirs: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("got %d recipes, want 0", len(got))
	}
}

// TestListRecipesDegradesToBody: a malformed or unterminated frontmatter fence
// keeps the recipe (whole document becomes the body) rather than dropping it.
func TestListRecipesDegradesToBody(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "broken.md", "---\nthis is: not: valid: yaml\nunterminated")

	got, err := listRecipes(context.Background(), dir, "")
	if err != nil {
		t.Fatalf("ListRecipes: %v", err)
	}
	r, ok := findRecipe(got, "broken")
	if !ok {
		t.Fatal("broken recipe was dropped; want it kept with the whole file as body")
	}
	if r.Description != "" {
		t.Errorf("broken.Description = %q, want empty (no parsed frontmatter)", r.Description)
	}
}

func TestRecipeFileConventions(t *testing.T) {
	if got := recipeDir("/work"); got != "/work/.lyra/recipes" {
		t.Fatalf("recipeDir(/work) = %q", got)
	}
	if got := recipeDir(""); got != "" {
		t.Fatalf("recipeDir(\"\") = %q, want empty", got)
	}

	for _, test := range []struct {
		file string
		name string
		ok   bool
	}{
		{file: "review.md", name: "review", ok: true},
		{file: "notes.txt"},
		{file: ".hidden.md"},
		{file: "README"},
	} {
		name, ok := recipeName(test.file)
		if name != test.name || ok != test.ok {
			t.Errorf("recipeName(%q) = %q, %v; want %q, %v", test.file, name, ok, test.name, test.ok)
		}
	}
}

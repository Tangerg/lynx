package recipes_test

import (
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/recipes"
)

// TestParse covers the frontmatter/body split rule: valid frontmatter is lifted
// into the metadata, the remainder is the body, and a bare prompt (no fence)
// keeps the whole document as the body.
func TestParse(t *testing.T) {
	withFM := recipes.Parse("review", recipes.ScopeProject, "/p/review.md",
		[]byte("---\ndescription: do a review\nargumentHint: \"[path]\"\n---\nReview $1"))
	if withFM.Name != "review" || withFM.Scope != recipes.ScopeProject || withFM.Source != "/p/review.md" {
		t.Fatalf("tags = %+v, want name/scope/source stamped", withFM)
	}
	if withFM.Description != "do a review" || withFM.ArgumentHint != "[path]" {
		t.Fatalf("frontmatter = %q / %q", withFM.Description, withFM.ArgumentHint)
	}
	if withFM.Body != "Review $1" {
		t.Fatalf("body = %q, want %q", withFM.Body, "Review $1")
	}

	bare := recipes.Parse("commit", recipes.ScopeGlobal, "/g/commit.md",
		[]byte("Write a conventional commit."))
	if bare.Description != "" || bare.ArgumentHint != "" {
		t.Fatalf("bare metadata = %q / %q, want empty", bare.Description, bare.ArgumentHint)
	}
	if bare.Body != "Write a conventional commit." {
		t.Fatalf("bare body = %q", bare.Body)
	}
}

// TestParseDegradesToBody: a malformed or unterminated frontmatter fence keeps
// the whole document as the body rather than erroring.
func TestParseDegradesToBody(t *testing.T) {
	got := recipes.Parse("broken", recipes.ScopeProject, "/p/broken.md",
		[]byte("---\nthis is: not: valid: yaml\nunterminated"))
	if got.Description != "" {
		t.Fatalf("Description = %q, want empty (no parsed frontmatter)", got.Description)
	}
	if got.Body == "" {
		t.Fatal("body should keep the whole document, not be dropped")
	}
}

// TestRecipeName covers the .md filter: only regular *.md files name a recipe;
// dotfiles and other extensions are skipped.
func TestRecipeName(t *testing.T) {
	cases := []struct {
		file string
		want string
		ok   bool
	}{
		{"review.md", "review", true},
		{"notes.txt", "", false},
		{".hidden.md", "", false},
		{"README", "", false},
	}
	for _, c := range cases {
		got, ok := recipes.RecipeName(c.file)
		if got != c.want || ok != c.ok {
			t.Errorf("RecipeName(%q) = %q,%v; want %q,%v", c.file, got, ok, c.want, c.ok)
		}
	}
}

// TestProjectDir resolves <workdir>/.lyra/recipes, and empty for an empty
// workdir (no project recipes).
func TestProjectDir(t *testing.T) {
	if got := recipes.ProjectDir("/work"); got != "/work/.lyra/recipes" {
		t.Fatalf("ProjectDir(/work) = %q", got)
	}
	if got := recipes.ProjectDir(""); got != "" {
		t.Fatalf("ProjectDir(\"\") = %q, want empty", got)
	}
}

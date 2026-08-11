package authoringcontext

import "testing"

func TestRecipeExpansionMatchesClientProtocol(t *testing.T) {
	recipe := Recipe{Name: "review", Body: "all=[$ARGUMENTS] one=$1 two=$2 adjacent=$1$2 ten=$10 unicode=好$1 missing=$9", Scope: ProjectRecipe, Source: "/repo/review.md"}
	got, err := recipe.Expand("  alpha   beta  ")
	if err != nil {
		t.Fatalf("Expand: %v", err)
	}
	want := "all=[alpha   beta] one=alpha two=beta adjacent=alphabeta ten=$10 unicode=好alpha missing="
	if got != want {
		t.Fatalf("Expand = %q, want %q", got, want)
	}
}

func TestRecipeExpansionNeverReinterpretsUserArguments(t *testing.T) {
	recipe := Recipe{
		Name: "literal", Body: "all=[$ARGUMENTS] first=[$1]", Scope: ProjectRecipe, Source: "/repo/literal.md",
	}
	got, err := recipe.Expand("alpha $2")
	if err != nil {
		t.Fatalf("Expand: %v", err)
	}
	if want := "all=[alpha $2] first=[alpha]"; got != want {
		t.Fatalf("Expand = %q, want %q", got, want)
	}
}

func TestRecipeRejectsIncompleteDefinition(t *testing.T) {
	if _, err := (Recipe{Name: "empty", Scope: GlobalRecipe, Source: "/recipe.md"}).Expand(""); err == nil {
		t.Fatal("Expand accepted an empty body")
	}
}

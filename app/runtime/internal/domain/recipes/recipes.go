// Package recipes defines the user-invoked prompt-template values exposed by
// recipe discovery. Filesystem layout and Markdown frontmatter are adapter
// concerns; this bounded context keeps only the product vocabulary.
package recipes

// Scope tags where a discovered recipe came from.
const (
	ScopeProject = "project"
	ScopeGlobal  = "global"
)

// Recipe is one discovered prompt template, tagged with where it came from.
// Body carries the $ARGUMENTS / $1..$9 placeholders the client substitutes at
// invocation.
type Recipe struct {
	Name         string
	Description  string
	ArgumentHint string
	Body         string
	Scope        string
	Source       string
}

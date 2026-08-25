// Package markdown reads CommonMark
// markdown sources using github.com/yuin/goldmark.
//
// Two modes are offered:
//
//   - Whole-document mode (default): the entire markdown payload becomes
//     one [*document.Document]; downstream splitters / token-budget
//     batchers handle chunking.
//   - Heading-split mode (opt in via [Config.HeadingSplitLevel]): the reader
//     walks goldmark's AST and emits one [*document.Document] per top-
//     level section, identified by an H1/H2 heading. Each section
//     carries the heading text + its hierarchy as metadata so embeddings
//     can include the path.
//
// Example:
//
//	r, _ := markdown.New(strings.NewReader(src),
//	    markdown.Config{HeadingSplitLevel: 2}) // split on H1+H2
//	docs, _ := r.Read(ctx)
package markdown

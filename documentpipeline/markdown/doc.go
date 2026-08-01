// Package markdown provides structure-aware, token-bounded splitting for
// Markdown documents.
//
// Headings are repeated as retrieval context, tables split only between rows,
// lists split only between items, and fenced code blocks split only between
// lines while retaining their fences. The base documentpipeline module stays
// format-neutral and does not depend on a Markdown parser.
package markdown

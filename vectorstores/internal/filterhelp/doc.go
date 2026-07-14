// Package filterhelp factors out the four AST-traversal helpers every
// vector-store visitor uses to decode filter expressions.
//
// Every backend's visitor.go used to ship its own near-identical
// copies of:
//   - LiteralAsKey  — *filter.Literal used as an index key → string
//   - LiteralToValue — *filter.Literal → typed Go value
//   - ExtractValue   — assert filter.Expr is *filter.Literal, then convert
//   - CollectKeyPath — walk an *filter.IndexExpr chain into []string
package filterhelp

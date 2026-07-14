// Package filter defines the stable metadata-filter expression vocabulary used
// by vector stores.
//
// Callers can build expressions with typed constructors such as [EQ], [GE],
// [In], [And], and [Not], use [ExprBuilder] for dynamic AND-by-default
// composition, or parse and validate the textual DSL with [Parse].
// The returned [Expr] tree contains only semantic nodes ([Ident], [Literal],
// [ListLiteral], [UnaryExpr], [BinaryExpr], and [IndexExpr]); lexer tokens,
// parser state, and optimization machinery are internal implementation details.
//
// Example:
//
//	expr := filter.And(
//		filter.EQ("category", "tech"),
//		filter.GE("year", 2020),
//	)
//	if err := filter.Validate(expr); err != nil {
//		return err
//	}
//
// [Parse] validates and simplifies the tree before a provider translates it.
package filter

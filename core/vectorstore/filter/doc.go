// Package filter defines the stable metadata-filter expression vocabulary used
// by vector stores.
//
// Callers can build predicates with typed constructors such as [EQ], [GE],
// [In], [Has], [And], and [Not], or parse and validate the textual DSL with
// [Parse]. [In] tests a selected scalar against a supplied list; [Has] tests
// whether a selected collection contains one complete scalar element.
// The returned [Predicate] tree contains only semantic nodes ([Ident], [Literal],
// [ListLiteral], [UnaryExpr], [BinaryExpr], and [IndexExpr]); lexer tokens,
// scanner state, and parser state are unexported implementation details.
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
// [Parse] validates and normalizes the tree before a provider translates it.
// Provider compilers and interpreters can share the complete-tree [Visitor]
// contract; [Visit] validates once and dispatches a predicate to one or more
// visitors. Provider visitors can use the literal conversion, key-path, and
// operator dispatch functions in this package without depending on another
// implementation layer. [Formatter] renders the same tree back to the textual
// DSL.
package filter

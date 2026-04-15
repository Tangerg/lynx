package filter

import (
	"github.com/Tangerg/lynx/core/vectorstore/filter/ast"
	"github.com/Tangerg/lynx/core/vectorstore/filter/token"
)

// Re-exported types from internal/ast for use by external packages.

// Expr is the base interface for all AST expression nodes.
type Expr = ast.Expr

// AtomicExpr represents atomic (leaf) expressions in the AST.
type AtomicExpr = ast.AtomicExpr

// ComputedExpr represents computed (composite) expressions in the AST.
type ComputedExpr = ast.ComputedExpr

// Ident represents an identifier node in the AST.
type Ident = ast.Ident

// Literal represents a literal value node in the AST.
type Literal = ast.Literal

// ListLiteral represents a list literal node in the AST.
type ListLiteral = ast.ListLiteral

// UnaryExpr represents a unary expression node in the AST.
type UnaryExpr = ast.UnaryExpr

// BinaryExpr represents a binary expression node in the AST.
type BinaryExpr = ast.BinaryExpr

// IndexExpr represents an index expression node in the AST.
type IndexExpr = ast.IndexExpr

// Visitor defines the visitor pattern interface for AST node operations.
type Visitor = ast.Visitor

// Re-exported types from internal/token for use by external packages.

// Kind represents a token kind.
type Kind = token.Kind

// Token represents a lexical token.
type Token = token.Token

// Position represents a location in the source code.
type Position = token.Position

// NoPosition is a sentinel value for unknown position.
var NoPosition = token.NoPosition

// Token kind values re-exported for external packages.
// Prefixed with "Kind" to avoid conflicts with filter builder functions (EQ, NE, etc.).
var (
	KindAND    = token.AND
	KindOR     = token.OR
	KindNOT    = token.NOT
	KindIN     = token.IN
	KindLIKE   = token.LIKE
	KindEQ     = token.EQ
	KindNE     = token.NE
	KindLT     = token.LT
	KindLE     = token.LE
	KindGT     = token.GT
	KindGE     = token.GE
	KindTRUE   = token.TRUE
	KindFALSE  = token.FALSE
	KindSTRING = token.STRING
	KindNUMBER = token.NUMBER
	KindERROR  = token.ERROR
)

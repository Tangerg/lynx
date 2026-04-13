// Package filter provides a type-safe, extensible expression filtering library
// for building and evaluating filter expressions with compile-time type checking.
//
// # Overview
//
// The filter package implements a complete expression language for filtering data,
// commonly used in vector databases, query builders, and dynamic filtering scenarios.
// It provides three distinct APIs to suit different use cases:
//
//   - Factory Functions: Type-safe expression construction with compile-time checking
//   - Builder API: Fluent interface with deferred error handling for complex conditions
//   - String Parsing: Runtime parsing of user-provided filter expressions
//
// # Architecture
//
// The library is organized into several layers, each with a clear responsibility:
//
//	┌─────────────────────────────────────────────────────────────┐
//	│                      API Layer (filter/)                     │
//	│  Factory Functions, Builder API, Convenience Functions       │
//	└─────────────────────────────────────────────────────────────┘
//	                            ↓
//	┌──────────────────┬──────────────────┬──────────────────────┐
//	│   Lexical        │    Syntax        │    Semantic          │
//	│   Analysis       │    Analysis      │    Analysis          │
//	│   (lexer/)       │    (parser/)     │    (visitors/)       │
//	├──────────────────┼──────────────────┼──────────────────────┤
//	│ Token Scanning   │ AST Construction │ Type Checking        │
//	│ Position Tracking│ Operator         │ Operator Validation  │
//	│ Error Detection  │ Precedence       │ Visitor Pattern      │
//	└──────────────────┴──────────────────┴──────────────────────┘
//	                            ↓
//	┌─────────────────────────────────────────────────────────────┐
//	│                   Foundation Layer                           │
//	│  Token Definitions (token/), AST Nodes (ast/),              │
//	│  Position Information (position/)                            │
//	└─────────────────────────────────────────────────────────────┘
//
// # Grammar Specification
//
// The filter expression language follows this EBNF grammar:
//
//	Expr        = OrExpr ;
//	OrExpr      = AndExpr { ("OR" | "or") AndExpr } ;
//	AndExpr     = NotExpr { ("AND" | "and") NotExpr } ;
//	NotExpr     = ("NOT" | "not") NotExpr | CompareExpr ;
//	CompareExpr = Primary CompareOp Primary ;
//	Primary     = AccessExpr | Literal | ParenExpr | ListLit ;
//	AccessExpr  = Ident { "[" AccessKey "]" } ;
//	AccessKey   = StrLit | NumLit ;
//	CompareOp   = "==" | "!=" | "<" | "<=" | ">" | ">=" |
//	              ("LIKE" | "like") | ("IN" | "in") ;
//	ParenExpr   = "(" Expr ")" ;
//	ListLit     = "(" LitSeq ")" ;
//	LitSeq      = Literal { "," Literal } ;
//	Literal     = NumLit | StrLit | BoolLit ;
//	NumLit      = [ "-" ] (DecNum | IntNum) ;
//	DecNum      = IntNum "." Digit+ ;
//	IntNum      = Digit+ ;
//	StrLit      = "'" { EscapeSeq | NormalChar } "'" ;
//	EscapeSeq   = "\\" | "\'" | "\"" | "\n" | "\t" | "\r" ;
//	NormalChar  = ? any printable character except single quote ? ;
//	BoolLit     = ("TRUE" | "true") | ("FALSE" | "false") ;
//	Ident       = Letter { Letter | Digit | "_" } ;
//	Digit       = "0" | ... | "9" ;
//	Letter      = "a" | ... | "z" | "A" | ... | "Z" ;
//
// Note: The grammar enforces that list literals must contain at least one element.
// Empty lists "()" are not allowed to maintain semantic clarity.
//
// # Operator Precedence
//
// Operators are evaluated in the following order (highest to lowest precedence):
//
//  1. []            Index/subscript access
//  2. IN, LIKE      Membership and pattern matching
//  3. ==, !=        Equality comparison
//     <, <=, >, >=  Relational comparison
//  4. NOT           Logical negation
//  5. AND           Logical conjunction
//  6. OR            Logical disjunction
//
// Parentheses can be used to override default precedence.
//
// # Type System
//
// The library provides compile-time type safety through Go generics:
//
// Supported literal types:
//   - Numeric: int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64
//   - String: string
//   - Boolean: bool
//   - AST nodes: *ast.Literal, *ast.Ident, *ast.IndexExpr, etc.
//
// Type constraints for operators:
//   - EQ, NE: Accept any comparable types (numeric, string, boolean)
//   - LT, LE, GT, GE: Accept only numeric types
//   - IN: Right operand must be a list literal
//   - LIKE: Right operand must be a string
//   - AND, OR: Operands must be computed expressions (return boolean)
//   - NOT: Operand must be a computed expression
//
// # Usage Examples
//
// ## Factory Functions API
//
// The factory functions API provides the strongest type safety with compile-time
// type checking. Use this for static filter conditions:
//
//	// Simple comparison
//	expr := filter.EQ("age", 18)
//
//	// Logical combination
//	expr := filter.And(
//	    filter.GT("age", 18),
//	    filter.EQ("status", "active"),
//	)
//
//	// Complex nested expression
//	expr := filter.Or(
//	    filter.And(
//	        filter.GT("age", 18),
//	        filter.LT("age", 65),
//	    ),
//	    filter.EQ("retired", true),
//	)
//
//	// Membership test
//	expr := filter.In("role", []string{"admin", "owner"})
//
//	// Pattern matching
//	expr := filter.Like("email", "%@gmail.com")
//
//	// Array/object access
//	expr := filter.EQ(
//	    filter.Index("tags", 0),
//	    "important",
//	)
//
//	// Chained index access
//	expr := filter.GT(
//	    filter.Index(filter.Index("data", 0), "score"),
//	    90,
//	)
//
//	// Negation
//	expr := filter.Not(filter.LT("age", 18))
//
// ## Builder API
//
// The Builder API provides a fluent interface with deferred error handling,
// ideal for constructing complex conditions dynamically:
//
//	// Simple chain
//	expr, err := filter.NewBuilder().
//	    GT("age", 18).
//	    EQ("status", "active").
//	    Build()
//
//	// Nested conditions with OR
//	expr, err := filter.NewBuilder().
//	    GT("age", 18).
//	    EQ("status", "active").
//	    Or(func(sub *filter.ExprBuilder) {
//	        sub.In("role", []string{"admin", "owner"})
//	    }).
//	    Build()
//
//	// Complex nesting with AND and NOT
//	expr, err := filter.NewBuilder().
//	    GT("age", 18).
//	    And(func(sub *filter.ExprBuilder) {
//	        sub.EQ("verified", true).
//	        Not(func(inner *filter.ExprBuilder) {
//	            inner.In("status", []string{"banned", "suspended"})
//	        })
//	    }).
//	    Build()
//
//	// Dynamic condition building
//	func BuildUserFilter(params FilterParams) (ast.Expr, error) {
//	    builder := filter.NewBuilder()
//
//	    if params.MinAge > 0 {
//	        builder.GT("age", params.MinAge)
//	    }
//
//	    if params.Status != "" {
//	        builder.EQ("status", params.Status)
//	    }
//
//	    if len(params.Roles) > 0 {
//	        builder.In("role", params.Roles)
//	    }
//
//	    return builder.Build()
//	}
//
// ## String Parsing API
//
// The string parsing API accepts user-provided filter expressions at runtime.
// This is most suitable for user input, configuration files, or query strings:
//
//	// Basic parsing
//	expr, err := filter.Parse("age > 18")
//	if err != nil {
//	    // Handle parse error
//	}
//
//	// Parse with semantic analysis
//	expr, err := filter.ParseAndAnalyze("age > 18 AND status == 'active'")
//	if err != nil {
//	    // Handle parse or analysis error
//	}
//
//	// Complex expression
//	input := "(age > 18 AND age < 65) OR retired == true"
//	expr, err := filter.Parse(input)
//
//	// Membership and pattern matching
//	input := "role IN ('admin', 'owner') AND email LIKE '%@company.com'"
//	expr, err := filter.Parse(input)
//
//	// Array/object access
//	input := "tags[0] == 'urgent' AND data['user']['name'] == 'John'"
//	expr, err := filter.Parse(input)
//
// ## Semantic Analysis
//
// After constructing or parsing an expression, use the Analyze function to
// perform semantic validation:
//
//	expr, err := filter.Parse("age > 18")
//	if err != nil {
//	    return err
//	}
//
//	// Validate operator-operand type compatibility
//	if err := filter.Analyze(expr); err != nil {
//	    // Handle semantic error (e.g., type mismatch)
//	    return err
//	}
//
//	// Or use ParseAndAnalyze for convenience
//	expr, err := filter.ParseAndAnalyze("age > 18")
//	// Both parsing and analysis errors are captured
//
// # Supported Operators
//
// ## Comparison Operators
//
//	==  Equal to
//	!=  Not equal to
//	<   Less than (numeric only)
//	<=  Less than or equal to (numeric only)
//	>   Greater than (numeric only)
//	>=  Greater than or equal to (numeric only)
//
// ## Membership and Pattern Operators
//
//	IN    Tests if value exists in a list
//	LIKE  Pattern matching with wildcards (% for any characters)
//
// ## Logical Operators
//
//	AND  Logical conjunction (both conditions must be true)
//	OR   Logical disjunction (at least one condition must be true)
//	NOT  Logical negation (inverts the condition)
//
// ## Access Operators
//
//	[]   Array/object element access (supports chaining)
//
// # Type Compatibility Matrix
//
// The following table shows which operators are compatible with which types:
//
//	┌──────────┬─────────┬────────┬─────────┬──────────┐
//	│ Operator │ Numeric │ String │ Boolean │ List     │
//	├──────────┼─────────┼────────┼─────────┼──────────┤
//	│ ==, !=   │    ✓    │   ✓    │    ✓    │    ✗     │
//	│ <, <=    │    ✓    │   ✗    │    ✗    │    ✗     │
//	│ >, >=    │    ✓    │   ✗    │    ✗    │    ✗     │
//	│ IN       │    ✓*   │   ✓*   │    ✓*   │    ✗     │
//	│ LIKE     │    ✗    │   ✓    │    ✗    │    ✗     │
//	│ AND, OR  │    ✗    │   ✗    │    ✓    │    ✗     │
//	│ NOT      │    ✗    │   ✗    │    ✓    │    ✗     │
//	└──────────┴─────────┴────────┴─────────┴──────────┘
//
//	* For IN operator, the right operand must be a list of the same type
//
// # Error Handling
//
// The library provides detailed error information at multiple levels:
//
// Lexical Errors (during tokenization):
//
//	filter.Parse("age > @18")
//	// Error: illegal character '@' at position 1:7
//
// Syntax Errors (during parsing):
//
//	filter.Parse("age > ")
//	// Error: unexpected end of input, expected primary expression at 1:7
//
//	filter.Parse("age > 18 AND")
//	// Error: unexpected end of input, expected expression at 1:13
//
// Semantic Errors (during analysis):
//
//	expr := filter.GT("name", "John")  // Compile error: string not allowed for GT
//
//	expr, _ := filter.Parse("age > 'John'")
//	filter.Analyze(expr)
//	// Error: type mismatch: cannot compare numeric field with string literal
//
// Position Information:
//
// All errors include precise position information in the format "line:column":
//
//	filter.Parse("age > 18\nstatus === 'active'")
//	// Error: unexpected token '=' at position 2:9
//
// # Features and Characteristics
//
// Type Safety:
//   - Compile-time type checking with Go generics
//   - Runtime type validation during semantic analysis
//   - Prevents invalid operator-operand combinations
//
// Performance:
//   - Hand-written lexer for optimal tokenization (O(n) complexity)
//   - Pratt parser for efficient expression parsing (O(n) complexity)
//   - Minimal allocations with careful memory management
//   - Zero external dependencies (except optional spf13/cast for convenience)
//
// Extensibility:
//   - Visitor pattern for easy addition of new analysis passes
//   - Table-driven parser for straightforward operator additions
//   - Clean separation between lexing, parsing, and analysis
//   - AST design supports future code generation
//
// Developer Experience:
//   - Three API styles for different use cases
//   - Fluent Builder API with method chaining
//   - Detailed error messages with position information
//   - Comprehensive documentation and examples
//
// # Design Patterns
//
// The library employs several well-established design patterns:
//
// Pratt Parser (Top-Down Operator Precedence):
//   - Elegant handling of operator precedence
//   - Simple and maintainable parsing logic
//   - Easy addition of new operators
//
// Visitor Pattern:
//   - Separates algorithms from data structures
//   - Enables multiple analysis passes without modifying AST
//   - Supports future extensions (optimization, code generation)
//
// Builder Pattern:
//   - Fluent interface for complex expression construction
//   - Deferred error handling for better UX
//   - Support for nested sub-expressions
//
// Factory Pattern:
//   - Type-safe object construction with generics
//   - Encapsulates type conversion logic
//   - Consistent API across different node types
//
// # Limitations and Considerations
//
// Known Limitations:
//   - No arithmetic operations (age + 1 > 18 is not supported)
//   - No function calls (len(tags) > 0 is not supported)
//   - No variable bindings or assignments
//   - Empty lists are not allowed (status IN () causes parse error)
//   - No expression evaluation engine (AST construction only)
//
// Semantic Constraints:
//   - Left operand of comparison must be a field access (identifier or index expression)
//   - Right operand of IN must be a non-empty list literal
//   - Right operand of LIKE must be a string literal
//   - Logical operators (AND, OR) require boolean operands
//   - Relational operators (<, >, <=, >=) require numeric operands
//
// Thread Safety:
//   - Lexer, Parser, and Analyzer are NOT thread-safe
//   - Each goroutine should create its own instances
//   - AST nodes are immutable after construction and can be shared
//
// # Package Structure
//
// The library is organized into focused packages:
//
// position:
//   - Position information for error reporting
//   - NoPosition constant for programmatically constructed nodes
//
// token:
//   - Token type definitions (Kind enum)
//   - Token structure with position and literal value
//   - Operator precedence definitions
//   - Keyword and operator mappings
//
// ast:
//   - Abstract Syntax Tree node definitions
//   - Expression interfaces (Expr, ComputedExpr)
//   - Node types: BinaryExpr, UnaryExpr, Ident, Literal, IndexExpr, ListLiteral
//
// lexer:
//   - Lexical analyzer (tokenizer)
//   - Character-by-character scanning
//   - Position tracking and error detection
//   - String escape sequence handling
//
// parser:
//   - Syntax analyzer (parser)
//   - Pratt parsing algorithm implementation
//   - Operator precedence handling
//   - AST construction
//
// visitors:
//   - Visitor interface for AST traversal
//   - Analyzer implementation for semantic validation
//   - Type checking and operator validation
//
// filter (this package):
//   - High-level APIs (factory functions, builder, parsing)
//   - Convenience functions for common use cases
//   - Integration of lexer, parser, and analyzer
//
// # Advanced Usage
//
// ## Mixing API Styles
//
// Different API styles can be combined for flexibility:
//
//	// Parse base condition from config
//	baseExpr, _ := filter.Parse("status == 'active'")
//
//	// Add dynamic conditions using factory functions
//	finalExpr := filter.And(
//	    baseExpr.(ast.ComputedExpr),
//	    filter.Or(
//	        filter.GT("age", 18),
//	        filter.In("role", []string{"admin"}),
//	    ),
//	)
//
// ## Expression Caching
//
// For frequently used expressions, consider caching parsed results:
//
//	var exprCache = make(map[string]ast.Expr)
//	var cacheMutex sync.RWMutex
//
//	func GetCachedExpr(query string) (ast.Expr, error) {
//	    cacheMutex.RLock()
//	    if expr, ok := exprCache[query]; ok {
//	        cacheMutex.RUnlock()
//	        return expr, nil
//	    }
//	    cacheMutex.RUnlock()
//
//	    expr, err := filter.ParseAndAnalyze(query)
//	    if err != nil {
//	        return nil, err
//	    }
//
//	    cacheMutex.Lock()
//	    exprCache[query] = expr
//	    cacheMutex.Unlock()
//
//	    return expr, nil
//	}
//
// ## Pre-compiled Expressions
//
// For static conditions, pre-compile expressions at package initialization:
//
//	var (
//	    ActiveUsersExpr  = filter.EQ("status", "active")
//	    AdultUsersExpr   = filter.GT("age", 18)
//	    AdminRoleExpr    = filter.In("role", []string{"admin", "owner"})
//	)
//
//	func GetActiveAdultAdmins() ast.Expr {
//	    return filter.And(
//	        ActiveUsersExpr,
//	        filter.And(AdultUsersExpr, AdminRoleExpr),
//	    )
//	}
//
// # Future Extensions
//
// The architecture is designed to support future enhancements:
//
// Planned Features:
//   - Expression evaluation engine with context support
//   - Schema validation for field existence and types
//   - Code generation (SQL WHERE clauses, MongoDB queries, etc.)
//   - Custom function support (len, contains, substring, etc.)
//   - Arithmetic expression support (age + 1 > 18)
//   - Optimization passes (constant folding, dead code elimination)
//
// Extension Points:
//   - Implement Visitor interface for new analysis passes
//   - Add operator handlers to parser for new operators
//   - Extend AST with new node types for additional features
//   - Create code generators for different target languages
//
// # Performance Considerations
//
// Time Complexity:
//   - Lexing: O(n) where n is input length
//   - Parsing: O(n) where n is number of tokens
//   - Analysis: O(n) where n is number of AST nodes
//   - Overall: O(n) linear time complexity
//
// Space Complexity:
//   - Token stream: O(n) where n is number of tokens
//   - AST: O(n) where n is number of nodes
//   - Recursion depth: O(d) where d is expression nesting depth
//   - Overall: O(n) linear space complexity
//
// Optimization Opportunities:
//   - Object pooling for frequently allocated node types
//   - Pre-allocation of slices with estimated capacity
//   - String interning for repeated identifiers
//   - Lazy analysis with result caching
//
// # Best Practices
//
// Choose the Right API:
//   - Factory functions for static, type-checked conditions
//   - Builder for dynamic, complex condition construction
//   - String parsing for user input and configuration
//
// Error Handling:
//   - Always check errors from Parse and Analyze
//   - Use typed error assertions to provide specific feedback
//   - Include position information in user-facing error messages
//
// Performance:
//   - Cache frequently used expressions
//   - Pre-compile static conditions at startup
//   - Reuse Lexer instances with Reset() if applicable
//
// Testing:
//   - Test both valid and invalid expressions
//   - Verify error messages and positions
//   - Test edge cases (empty input, deep nesting, long expressions)
//   - Benchmark performance-critical paths
//
// # Version History
//
// Current Version: 1.0.0
//
// Changes from Grammar Specification:
//   - Added support for negative numeric literals (-123, -3.14)
//   - Empty lists are explicitly forbidden for semantic clarity
//   - Comparison expressions constrained to improve type safety
//   - Keywords are case-insensitive (AND/and, OR/or, etc.)
//
// # References
//
// Parsing Algorithms:
//   - Pratt, Vaughan. "Top Down Operator Precedence." (1973)
//   - Nystrom, Bob. "Crafting Interpreters." (2021)
//
// Design Patterns:
//   - Gamma et al. "Design Patterns: Elements of Reusable Object-Oriented Software." (1994)
//   - Visitor Pattern for AST traversal
//   - Builder Pattern for fluent APIs
//
// Related Projects:
//   - ANTLR: Parser generator (reference for grammar design)
//   - Expr (antonmedv/expr): Go expression evaluator
//   - govaluate: Go expression evaluation library
//
// # See Also
//
// Related packages:
//   - position: Position information for error reporting
//   - token: Token definitions and constants
//   - ast: Abstract Syntax Tree node definitions
//   - lexer: Lexical analysis implementation
//   - parser: Syntax analysis implementation
//   - visitors: Semantic analysis and AST traversal
//
// External documentation:
//   - Full grammar specification in EBNF format
//   - ANTLR4 grammar definition
//   - API reference and examples
//   - Architecture documentation
package filter

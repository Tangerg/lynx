package token

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

// Kind represents the type of a lexical token in the query language.
// It uses an integer enumeration to efficiently categorize different token types.
type Kind int

// Token type constants define all possible token kinds in the query language.
// The enumeration is bounded by kindBegin and kindEnd for validation purposes.
const (
	kindBegin Kind = iota // Boundary marker for validation - not a valid token
	ERROR                 // Represents lexical errors during tokenization
	EOF                   // End of file/input marker
	IDENT                 // Identifiers like field names: name, age, email, etc.
	NUMBER                // Numeric literals: 12345 or 123.45
	STRING                // String literals enclosed in quotes: 'abcde'
	TRUE                  // Boolean literal: true
	FALSE                 // Boolean literal: false
	EQ                    // Equal: ==
	NE                    // Not equal: !=
	LT                    // Less than: <
	LE                    // Less than or equal: <=
	GT                    // Greater than: >
	GE                    // Greater than or equal: >=
	AND                   // Logical AND: and
	OR                    // Logical OR: or
	NOT                   // Logical NOT: not
	IN                    // Membership test: in
	LIKE                  // Pattern matching: like
	LPAREN                // Left parenthesis: (
	RPAREN                // Right parenthesis: )
	LBRACK                // Left bracket: [
	RBRACK                // Right bracket: ]
	COMMA                 // Comma separator: ,
	kindEnd               // Boundary marker for validation - not a valid token
)

// kindsMetadata maps each Kind to its metadata for O(1) lookup.
// The array indices correspond to Kind enum values.
var kindsMetadata = [...]*struct {
	Name      string // Human-readable name for debugging and error reporting
	Literal   string // Literal string representation in source code (empty if not applicable)
	IsKeyword bool   // Whether this token kind is a reserved keyword
}{
	ERROR: {
		Name: "ERROR",
	},
	EOF: {
		Name: "EOF",
	},
	IDENT: {
		Name: "IDENT",
	},
	NUMBER: {
		Name: "NUMBER",
	},
	STRING: {
		Name: "STRING",
	},
	TRUE: {
		Name:      "BOOL",
		Literal:   "true",
		IsKeyword: true,
	},
	FALSE: {
		Name:      "BOOL",
		Literal:   "false",
		IsKeyword: true,
	},
	EQ: {
		Name:    "EQ",
		Literal: "==",
	},
	NE: {
		Name:    "NE",
		Literal: "!=",
	},
	LT: {
		Name:    "LT",
		Literal: "<",
	},
	LE: {
		Name:    "LE",
		Literal: "<=",
	},
	GT: {
		Name:    "GT",
		Literal: ">",
	},
	GE: {
		Name:    "GE",
		Literal: ">=",
	},
	AND: {
		Name:      "AND",
		Literal:   "and",
		IsKeyword: true,
	},
	OR: {
		Name:      "OR",
		Literal:   "or",
		IsKeyword: true,
	},
	NOT: {
		Name:      "NOT",
		Literal:   "not",
		IsKeyword: true,
	},
	IN: {
		Name:      "IN",
		Literal:   "in",
		IsKeyword: true,
	},
	LIKE: {
		Name:      "LIKE",
		Literal:   "like",
		IsKeyword: true,
	},
	LPAREN: {
		Name:    "LPAREN",
		Literal: "(",
	},
	RPAREN: {
		Name:    "RPAREN",
		Literal: ")",
	},
	LBRACK: {
		Name:    "LBRACK",
		Literal: "[",
	},
	RBRACK: {
		Name:    "RBRACK",
		Literal: "]",
	},
	COMMA: {
		Name:    "COMMA",
		Literal: ",",
	},
}

// keywordKinds provides O(1) lookup for keyword recognition during tokenization.
// It maps lowercase keyword strings to their corresponding Kind values.
var keywordKinds map[string]Kind

// init initializes the keyword lookup map for efficient keyword recognition.
// This runs once at package initialization time.
func init() {
	keywordKinds = make(map[string]Kind)
	for i := kindBegin + 1; i < kindEnd; i++ {
		k := kindsMetadata[i]
		if k == nil {
			panic(fmt.Sprintf("missing metadata for token kind %d", i))
		}
		if k.IsKeyword {
			keywordKinds[k.Literal] = i
		}
	}
}

// IsValid checks whether the Kind value is within the valid range.
// Valid kinds are those between kindBegin and kindEnd (exclusive).
func (k Kind) IsValid() bool {
	return k > kindBegin && k < kindEnd
}

// ensureValid panics if the Kind is not valid, providing fail-fast behavior.
// This is used internally to catch programming errors early.
func (k Kind) ensureValid() {
	if !k.IsValid() {
		panic("invalid token Kind: " + strconv.Itoa(int(k)))
	}
}

// Name returns the human-readable name of the token kind.
// This is primarily used for debugging, error messages, and logging.
func (k Kind) Name() string {
	k.ensureValid()
	return kindsMetadata[k].Name
}

// Literal returns the literal string representation of the token kind.
// Returns empty string for tokens that don't have fixed literals (like IDENT, NUMBER, STRING, etc...).
func (k Kind) Literal() string {
	k.ensureValid()
	return kindsMetadata[k].Literal
}

// String provides a formatted string representation of the Kind for debugging.
// It includes both the name and literal representation in a readable format.
func (k Kind) String() string {
	k.ensureValid()
	metadata := kindsMetadata[k]
	return fmt.Sprintf(
		`
Kind {
  name: %s, 
  literal: %s
}`,
		metadata.Name, metadata.Literal,
	)
}

// Is checks if this Kind equals the given other Kind.
// This provides a more readable way to compare kinds than using == directly.
func (k Kind) Is(other Kind) bool {
	return k == other
}

// IsLiteral returns true if this Kind represents a literal value.
// Literals are constant values like strings, numbers, and booleans.
func (k Kind) IsLiteral() bool {
	switch k {
	case STRING, NUMBER, TRUE, FALSE:
		return true
	default:
		return false
	}
}

// IsKeyword returns true if this Kind represents a reserved keyword.
// Keywords cannot be used as identifiers in the query language.
func (k Kind) IsKeyword() bool {
	k.ensureValid()
	return kindsMetadata[k].IsKeyword
}

// IsEqualityOperator returns true if this Kind represents an equality operator.
// Equality operators test for equality or inequality between two values and can
// be applied to any comparable types (numbers, strings, booleans, etc.).
func (k Kind) IsEqualityOperator() bool {
	switch k {
	case EQ, NE:
		return true
	default:
		return false
	}
}

// IsOrderingOperator returns true if this Kind represents an ordering operator.
// Ordering operators compare the relative order between two values and can
// only be applied to orderable types (numbers, comparable strings).
func (k Kind) IsOrderingOperator() bool {
	switch k {
	case LT, LE, GT, GE:
		return true
	default:
		return false
	}
}

// IsComparisonOperator returns true if this Kind represents a comparison operator.
// Comparison operators evaluate the relationship between two values and return
// a boolean result. These include equality, inequality, and relational operators.
func (k Kind) IsComparisonOperator() bool {
	return k.IsEqualityOperator() || k.IsOrderingOperator()
}

// IsLogicalOperator returns true if this Kind represents a logical operator.
// Logical operators perform boolean logic operations on boolean operands
// and return boolean results. These include conjunction (AND) and disjunction (OR).
func (k Kind) IsLogicalOperator() bool {
	switch k {
	case AND, OR:
		return true
	default:
		return false
	}
}

// IsMatchingOperator returns true if this Kind represents a matching operator.
// Matching operators test whether a value satisfies certain criteria or patterns
// and return boolean results. These include membership (IN) and pattern matching (LIKE).
func (k Kind) IsMatchingOperator() bool {
	switch k {
	case IN, LIKE:
		return true
	default:
		return false
	}
}

// IsBinaryOperator returns true if this Kind represents a binary operator.
// Binary operators require two operands (left and right).
func (k Kind) IsBinaryOperator() bool {
	return k.IsComparisonOperator() || k.IsLogicalOperator() || k.IsMatchingOperator()
}

// IsUnaryOperator returns true if this Kind represents a unary operator.
// Unary operators require only one operand.
func (k Kind) IsUnaryOperator() bool {
	return k == NOT
}

// IsOperator returns true if this Kind represents any type of operator.
// This includes both binary and unary operators.
func (k Kind) IsOperator() bool {
	return k.IsBinaryOperator() || k.IsUnaryOperator()
}

// IsDelimiter returns true if this Kind represents a delimiter.
// Delimiters are punctuation marks used to separate or group elements.
func (k Kind) IsDelimiter() bool {
	switch k {
	case LPAREN, RPAREN, LBRACK, RBRACK, COMMA:
		return true
	default:
		return false
	}
}

// Operator precedence constants based on PostgreSQL documentation.
// See: https://www.postgresql.org/docs/current/sql-syntax-lexical.html#SQL-PRECEDENCE
// Higher values indicate higher precedence (tighter binding).
const (
	PrecedenceLowest = iota
	PrecedenceOR     // 1: OR (lowest precedence)
	PrecedenceAND    // 2: AND
	PrecedenceNOT    // 3: NOT
	PrecedenceCMP    // 4: EQ, NE, LT, LE, GT, GE
	PrecedenceMatch  // 5: LIKE, IN
	PrecedenceIndex  // 6: [] (highest precedence)
)

// Precedence returns the operator precedence for this Kind.
// Higher numbers indicate higher precedence (tighter binding).
// Returns PrecedenceLowest for non-operators.
//
// Precedence levels:
//   - 1: OR (lowest precedence)
//   - 2: AND
//   - 3: NOT
//   - 4: EQ, NE, LT, LE, GT, GE
//   - 5: LIKE, IN
//   - 6: [] (highest precedence)
func (k Kind) Precedence() int {
	switch k {
	case OR:
		return PrecedenceOR
	case AND:
		return PrecedenceAND
	case NOT:
		return PrecedenceNOT
	case EQ, NE, LT, LE, GT, GE:
		return PrecedenceCMP
	case LIKE, IN:
		return PrecedenceMatch
	case LBRACK:
		return PrecedenceIndex
	default:
		return PrecedenceLowest // Non-operators have no precedence
	}
}

// KindOf determines the appropriate Kind for a given identifier string.
// If the identifier is a reserved keyword, returns the corresponding keyword Kind.
// Otherwise, returns IDENT. The comparison is case-insensitive.
func KindOf(ident string) Kind {
	keywordKind, exists := keywordKinds[strings.ToLower(ident)]
	if exists {
		return keywordKind
	}
	return IDENT
}

// IsKeyword checks if the given identifier string is a reserved keyword.
// The comparison is case-insensitive.
func IsKeyword(ident string) bool {
	return KindOf(ident).IsKeyword()
}

// IsIdentifier validates whether a string can be used as a valid identifier.
// Valid identifiers must:
//   - Be non-empty
//   - Not be reserved keywords
//   - Contain only letters, digits, and underscores
//   - Follow the language's identifier naming rules
func IsIdentifier(ident string) bool {
	if ident == "" || IsKeyword(ident) {
		return false
	}
	for _, char := range ident {
		if !IsLiteralChar(char) {
			return false
		}
	}
	return true
}

// IsLiteralChar checks if a rune is valid within an identifier or literal.
// Valid characters are:
//   - Unicode letters (any language)
//   - Unicode digits
//   - Underscore (_)
func IsLiteralChar(char rune) bool {
	return unicode.IsLetter(char) || unicode.IsDigit(char) || char == '_'
}

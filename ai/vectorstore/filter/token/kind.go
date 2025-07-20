package token

import (
	"fmt"
	"slices"
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

	kindEnd // Boundary marker for validation - not a valid token
)

// kindNames maps each Kind to its human-readable name for debugging and error reporting.
// The array indices correspond to Kind enum values for O(1) lookup.
var kindNames = [...]string{
	kindBegin: "",
	ERROR:     "ERROR",
	EOF:       "EOF",
	IDENT:     "IDENT",
	NUMBER:    "NUMBER",
	STRING:    "STRING",
	TRUE:      "TRUE",
	FALSE:     "FALSE",
	EQ:        "EQ",
	NE:        "NE",
	LT:        "LT",
	LE:        "LE",
	GT:        "GT",
	GE:        "GE",
	AND:       "AND",
	OR:        "OR",
	NOT:       "NOT",
	IN:        "IN",
	LIKE:      "LIKE",
	LPAREN:    "LPAREN",
	RPAREN:    "RPAREN",
	LBRACK:    "LBRACK",
	RBRACK:    "RBRACK",
	COMMA:     "COMMA",
	kindEnd:   "",
}

// kindLiterals maps each Kind to its literal string representation in source code.
// Only tokens with fixed literal representations are included (operators, keywords, punctuation).
var kindLiterals = [...]string{
	kindBegin: "",
	TRUE:      "true",
	FALSE:     "false",
	EQ:        "==",
	NE:        "!=",
	LT:        "<",
	LE:        "<=",
	GT:        ">",
	GE:        ">=",
	AND:       "and",
	OR:        "or",
	IN:        "in",
	LIKE:      "like",
	NOT:       "not",
	LPAREN:    "(",
	RPAREN:    ")",
	LBRACK:    "[",
	RBRACK:    "]",
	COMMA:     ",",
	kindEnd:   "",
}

// keywordKinds defines which token types are reserved keywords in the language.
// These identifiers cannot be used as variable names or field names.
var keywordKinds = []Kind{
	TRUE, FALSE, // Boolean literals
	AND, OR, // Logical operators
	IN, LIKE, // Special operators
	NOT, // Unary operator
}

// keywordKindsMap provides O(1) lookup for keyword recognition during tokenization.
// It maps lowercase keyword strings to their corresponding Kind values.
var keywordKindsMap map[string]Kind

// init initializes the keyword lookup map for efficient keyword recognition.
// This runs once at package initialization time.
func init() {
	keywordKindsMap = make(map[string]Kind, len(keywordKinds))
	for _, keywordKind := range keywordKinds {
		keywordKindsMap[keywordKind.Literal()] = keywordKind
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
	return kindNames[k]
}

// Literal returns the literal string representation of the token kind.
// Returns empty string for tokens that don't have fixed literals (like IDENT, NUMBER).
func (k Kind) Literal() string {
	k.ensureValid()
	return kindLiterals[k]
}

// String provides a formatted string representation of the Kind for debugging.
// It includes both the name and literal representation in a readable format.
func (k Kind) String() string {
	return fmt.Sprintf(
		`
Kind {
  name: %s, 
  literal: %s
}`,
		k.Name(), k.Literal(),
	)
}

// Is checks if this Kind equals the given other Kind.
// This provides a more readable way to compare kinds than using == directly.
func (k Kind) Is(other Kind) bool {
	return k == other
}

// IsKeyword returns true if this Kind represents a reserved keyword.
// Keywords cannot be used as identifiers in the query language.
func (k Kind) IsKeyword() bool {
	return slices.Contains(keywordKinds, k)
}

// IsBinaryOperator returns true if this Kind represents a binary operator.
// Binary operators require two operands (left and right).
func (k Kind) IsBinaryOperator() bool {
	switch k {
	case EQ, NE, LT, LE, GT, GE, AND, OR, IN, LIKE:
		return true
	default:
		return false
	}
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

// Precedence returns the operator precedence for this Kind.
// Higher numbers indicate higher precedence (tighter binding).
// Returns 0 for non-operators.
//
// Precedence levels:
//
//	1: OR (lowest precedence)
//	2: AND
//	3: NOT
//	4: EQ, NE
//	5: LT, LE, GT, GE
//	6: LIKE, IN (highest precedence)
func (k Kind) Precedence() int {
	switch k {
	case OR:
		return 1
	case AND:
		return 2
	case NOT:
		return 3
	case EQ, NE:
		return 4
	case LT, LE, GT, GE:
		return 5
	case LIKE, IN:
		return 6
	default:
		return 0 // Non-operators have no precedence
	}
}

// KindOf determines the appropriate Kind for a given identifier string.
// If the identifier is a reserved keyword, returns the corresponding keyword Kind.
// Otherwise, returns IDENT. The comparison is case-insensitive.
func KindOf(ident string) Kind {
	keywordKind, exists := keywordKindsMap[strings.ToLower(ident)]
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
	if !unicode.IsLetter(char) &&
		!unicode.IsDigit(char) &&
		char != '_' {
		return false
	}
	return true
}

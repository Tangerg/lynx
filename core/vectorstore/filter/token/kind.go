package token

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

// Kind enumerates every lexical token category the filter language
// recognizes. Use [KindOf] to classify identifier text and the
// IsXxx predicates ([Kind.IsOperator], [Kind.IsKeyword], ...) to
// branch on operator families.
type Kind int

// Token kinds. The kindBegin / kindEnd sentinels bracket the valid
// range and are NOT themselves kinds.
const (
	kindBegin Kind = iota
	ERROR          // Lexical error sentinel.
	EOF            // End-of-input marker.
	IDENT          // User-supplied identifier (field/variable name).
	NUMBER         // Numeric literal: 123 or 1.5.
	STRING         // String literal: 'abc'.
	TRUE           // Boolean literal: true.
	FALSE          // Boolean literal: false.
	EQ             // Equality: ==
	NE             // Inequality: !=
	LT             // Less than: <
	LE             // Less or equal: <=
	GT             // Greater than: >
	GE             // Greater or equal: >=
	AND            // Logical AND.
	OR             // Logical OR.
	NOT            // Logical NOT.
	IN             // Membership: x IN (...).
	LIKE           // Pattern match: x LIKE 'foo%'.
	LPAREN         // Left paren: (
	RPAREN         // Right paren: )
	LBRACK         // Left bracket: [
	RBRACK         // Right bracket: ]
	COMMA          // List separator: ,
	kindEnd
)

// kindMetadata is the per-kind data table. Indexed by Kind.
var kindMetadata = [...]*struct {
	Name      string
	Literal   string
	IsKeyword bool
}{
	ERROR:  {Name: "ERROR"},
	EOF:    {Name: "EOF"},
	IDENT:  {Name: "IDENT"},
	NUMBER: {Name: "NUMBER"},
	STRING: {Name: "STRING"},
	TRUE:   {Name: "BOOL", Literal: "true", IsKeyword: true},
	FALSE:  {Name: "BOOL", Literal: "false", IsKeyword: true},
	EQ:     {Name: "EQ", Literal: "=="},
	NE:     {Name: "NE", Literal: "!="},
	LT:     {Name: "LT", Literal: "<"},
	LE:     {Name: "LE", Literal: "<="},
	GT:     {Name: "GT", Literal: ">"},
	GE:     {Name: "GE", Literal: ">="},
	AND:    {Name: "AND", Literal: "and", IsKeyword: true},
	OR:     {Name: "OR", Literal: "or", IsKeyword: true},
	NOT:    {Name: "NOT", Literal: "not", IsKeyword: true},
	IN:     {Name: "IN", Literal: "in", IsKeyword: true},
	LIKE:   {Name: "LIKE", Literal: "like", IsKeyword: true},
	LPAREN: {Name: "LPAREN", Literal: "("},
	RPAREN: {Name: "RPAREN", Literal: ")"},
	LBRACK: {Name: "LBRACK", Literal: "["},
	RBRACK: {Name: "RBRACK", Literal: "]"},
	COMMA:  {Name: "COMMA", Literal: ","},
}

// keywordKinds maps lowercase keyword text to its Kind. Built once at
// package init from kindMetadata so [KindOf] can do an O(1) lookup.
var keywordKinds map[string]Kind

func init() {
	keywordKinds = make(map[string]Kind)
	for i := kindBegin + 1; i < kindEnd; i++ {
		meta := kindMetadata[i]
		if meta == nil {
			panic(fmt.Sprintf("token.init: missing metadata for kind %d", i))
		}
		if meta.IsKeyword {
			keywordKinds[meta.Literal] = i
		}
	}
}

// IsValid reports whether k is a real token kind (not the sentinel
// boundaries).
func (k Kind) IsValid() bool { return k > kindBegin && k < kindEnd }

// ensureValid panics on invalid kinds — fail-fast for programmer error.
func (k Kind) ensureValid() {
	if !k.IsValid() {
		panic("token.Kind: invalid value " + strconv.Itoa(int(k)))
	}
}

// Name returns the human-readable name (used in error messages and
// debug output).
func (k Kind) Name() string {
	k.ensureValid()
	return kindMetadata[k].Name
}

// Literal returns the canonical lexeme. Empty for kinds with variable
// content (IDENT, NUMBER, STRING).
func (k Kind) Literal() string {
	k.ensureValid()
	return kindMetadata[k].Literal
}

// String renders the kind as a multi-line debug record.
func (k Kind) String() string {
	k.ensureValid()
	meta := kindMetadata[k]
	return fmt.Sprintf(
		`
Kind {
  name: %s,
  literal: %s
}`,
		meta.Name, meta.Literal,
	)
}

// Is reports whether k equals other — sugar for `k == other` that
// reads better in switch-style chains.
func (k Kind) Is(other Kind) bool { return k == other }

// IsLiteral reports whether the kind is a literal value (string,
// number, true, false).
func (k Kind) IsLiteral() bool {
	switch k {
	case STRING, NUMBER, TRUE, FALSE:
		return true
	default:
		return false
	}
}

// IsKeyword reports whether the kind is a reserved keyword (cannot
// be used as an identifier).
func (k Kind) IsKeyword() bool {
	k.ensureValid()
	return kindMetadata[k].IsKeyword
}

// IsEqualityOperator reports whether the kind is == or !=.
func (k Kind) IsEqualityOperator() bool {
	switch k {
	case EQ, NE:
		return true
	default:
		return false
	}
}

// IsOrderingOperator reports whether the kind is <, <=, >, or >=.
func (k Kind) IsOrderingOperator() bool {
	switch k {
	case LT, LE, GT, GE:
		return true
	default:
		return false
	}
}

// IsComparisonOperator is the union of equality and ordering — every
// operator that compares two values and yields a boolean.
func (k Kind) IsComparisonOperator() bool {
	return k.IsEqualityOperator() || k.IsOrderingOperator()
}

// IsLogicalOperator reports whether the kind is AND or OR.
func (k Kind) IsLogicalOperator() bool {
	switch k {
	case AND, OR:
		return true
	default:
		return false
	}
}

// IsMatchingOperator reports whether the kind is IN or LIKE.
func (k Kind) IsMatchingOperator() bool {
	switch k {
	case IN, LIKE:
		return true
	default:
		return false
	}
}

// IsBinaryOperator reports whether the kind takes two operands —
// comparison, logical, or matching.
func (k Kind) IsBinaryOperator() bool {
	return k.IsComparisonOperator() || k.IsLogicalOperator() || k.IsMatchingOperator()
}

// IsUnaryOperator reports whether the kind takes one operand — only
// NOT today.
func (k Kind) IsUnaryOperator() bool { return k == NOT }

// IsOperator reports whether the kind is any operator (unary or
// binary).
func (k Kind) IsOperator() bool {
	return k.IsBinaryOperator() || k.IsUnaryOperator()
}

// IsDelimiter reports whether the kind is structural punctuation —
// parens, brackets, commas.
func (k Kind) IsDelimiter() bool {
	switch k {
	case LPAREN, RPAREN, LBRACK, RBRACK, COMMA:
		return true
	default:
		return false
	}
}

// Operator-precedence levels, modeled on PostgreSQL's lexical-syntax
// table — higher values bind tighter.
const (
	PrecedenceLowest = iota
	PrecedenceOR     // 1: OR (loosest binding).
	PrecedenceAND    // 2: AND.
	PrecedenceNOT    // 3: NOT.
	PrecedenceCMP    // 4: ==, !=, <, <=, >, >=.
	PrecedenceMatch  // 5: LIKE, IN.
	PrecedenceIndex  // 6: [] (tightest binding).
)

// Precedence returns the operator's precedence level. Non-operators
// return [PrecedenceLowest] so the parser can use Precedence as a
// uniform priority key.
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
		return PrecedenceLowest
	}
}

// KindOf maps an identifier string onto its Kind — keywords match
// case-insensitively and return the corresponding keyword kind; all
// other strings yield IDENT. The all-lowercase fast path skips the
// allocation in [strings.ToLower] for the common case where the
// caller already supplies lowercase text.
func KindOf(ident string) Kind {
	if !hasUpperASCII(ident) {
		if k, ok := keywordKinds[ident]; ok {
			return k
		}
		return IDENT
	}
	if k, ok := keywordKinds[strings.ToLower(ident)]; ok {
		return k
	}
	return IDENT
}

// hasUpperASCII reports whether s contains any A–Z byte. Cheap test
// for the all-lowercase fast path in [KindOf].
func hasUpperASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if c := s[i]; c >= 'A' && c <= 'Z' {
			return true
		}
	}
	return false
}

// IsKeyword reports whether ident is a reserved keyword.
func IsKeyword(ident string) bool { return KindOf(ident).IsKeyword() }

// IsIdentifier reports whether ident can serve as a user-supplied
// identifier — non-empty, not a keyword, made entirely of letters,
// digits, and underscores.
func IsIdentifier(ident string) bool {
	if ident == "" || IsKeyword(ident) {
		return false
	}

	for _, r := range ident {
		if !IsLiteralChar(r) {
			return false
		}
	}
	return true
}

// IsLiteralChar reports whether r may appear inside an identifier —
// any Unicode letter, any Unicode digit, or '_'.
func IsLiteralChar(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
}

package token

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
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
	ERROR
	EOF
	IDENT
	NUMBER
	STRING
	TRUE
	FALSE
	EQ
	NE
	LT
	LE
	GT
	GE
	AND
	OR
	NOT
	IN
	LIKE
	IS
	NULL
	LPAREN
	RPAREN
	LBRACK
	RBRACK
	COMMA
	kindEnd
)

// category is a bitmask of the structural categories a Kind belongs
// to. The classification is mostly disjoint, but the helpers
// [Kind.IsBinaryOperator], [Kind.IsComparisonOperator], and
// [Kind.IsOperator] are unions over multiple categories below.
type category uint16

const (
	catLiteral    category = 1 << iota // STRING / NUMBER / TRUE / FALSE.
	catEqualityOp                      // ==, !=.
	catOrderingOp                      // <, <=, >, >=.
	catLogicalOp                       // AND, OR.
	catMatchingOp                      // IN, LIKE.
	catUnaryOp                         // NOT.
	catNullOp                          // IS (null test).
	catDelimiter                       // (, ), [, ], comma.
)

// Precedence defaults to [PrecedenceLowest]; Categories defaults to 0.
var kindMetadata = [...]*struct {
	Name       string
	Literal    string
	IsKeyword  bool
	Precedence int
	Categories category
}{
	ERROR:  {Name: "ERROR"},
	EOF:    {Name: "EOF"},
	IDENT:  {Name: "IDENT"},
	NUMBER: {Name: "NUMBER", Categories: catLiteral},
	STRING: {Name: "STRING", Categories: catLiteral},
	TRUE:   {Name: "BOOL", Literal: "true", IsKeyword: true, Categories: catLiteral},
	FALSE:  {Name: "BOOL", Literal: "false", IsKeyword: true, Categories: catLiteral},
	EQ:     {Name: "EQ", Literal: "==", Precedence: PrecedenceCMP, Categories: catEqualityOp},
	NE:     {Name: "NE", Literal: "!=", Precedence: PrecedenceCMP, Categories: catEqualityOp},
	LT:     {Name: "LT", Literal: "<", Precedence: PrecedenceCMP, Categories: catOrderingOp},
	LE:     {Name: "LE", Literal: "<=", Precedence: PrecedenceCMP, Categories: catOrderingOp},
	GT:     {Name: "GT", Literal: ">", Precedence: PrecedenceCMP, Categories: catOrderingOp},
	GE:     {Name: "GE", Literal: ">=", Precedence: PrecedenceCMP, Categories: catOrderingOp},
	AND:    {Name: "AND", Literal: "and", IsKeyword: true, Precedence: PrecedenceAND, Categories: catLogicalOp},
	OR:     {Name: "OR", Literal: "or", IsKeyword: true, Precedence: PrecedenceOR, Categories: catLogicalOp},
	NOT:    {Name: "NOT", Literal: "not", IsKeyword: true, Precedence: PrecedenceNOT, Categories: catUnaryOp},
	IN:     {Name: "IN", Literal: "in", IsKeyword: true, Precedence: PrecedenceMatch, Categories: catMatchingOp},
	LIKE:   {Name: "LIKE", Literal: "like", IsKeyword: true, Precedence: PrecedenceMatch, Categories: catMatchingOp},
	IS:     {Name: "IS", Literal: "is", IsKeyword: true, Precedence: PrecedenceCMP, Categories: catNullOp},
	// NULL is a reserved keyword so the lexer doesn't treat "null" as a
	// field identifier; it carries no category (not a literal), so it is
	// only ever consumed as the right operand of IS.
	NULL:   {Name: "NULL", Literal: "null", IsKeyword: true},
	LPAREN: {Name: "LPAREN", Literal: "(", Categories: catDelimiter},
	RPAREN: {Name: "RPAREN", Literal: ")", Categories: catDelimiter},
	LBRACK: {Name: "LBRACK", Literal: "[", Precedence: PrecedenceIndex, Categories: catDelimiter},
	RBRACK: {Name: "RBRACK", Literal: "]", Categories: catDelimiter},
	COMMA:  {Name: "COMMA", Literal: ",", Categories: catDelimiter},
}

// hasCategory reports whether k belongs to any of the supplied
// categories. Invalid kinds belong to none.
func (k Kind) hasCategory(mask category) bool {
	if !k.IsValid() {
		return false
	}
	return kindMetadata[k].Categories&mask != 0
}

// keywordKinds maps lowercase keyword text to its Kind. Built once
// lazily via [sync.OnceValues] from kindMetadata so [KindOf] can do
// an O(1) lookup without an init() ordering dependency.
var keywordKinds = sync.OnceValues(func() (map[string]Kind, struct{}) {
	m := make(map[string]Kind)
	for i := kindBegin + 1; i < kindEnd; i++ {
		meta := kindMetadata[i]
		if meta == nil {
			panic(fmt.Sprintf("token.init: missing metadata for kind %d", i))
		}
		if meta.IsKeyword {
			m[meta.Literal] = i
		}
	}
	return m, struct{}{}
})

func (k Kind) IsValid() bool { return k > kindBegin && k < kindEnd }

func (k Kind) ensureValid() {
	if !k.IsValid() {
		panic("token.Kind: invalid value " + strconv.Itoa(int(k)))
	}
}

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

func (k Kind) String() string { return k.Name() }

func (k Kind) Is(other Kind) bool { return k == other }

func (k Kind) IsLiteral() bool { return k.hasCategory(catLiteral) }

// IsKeyword reports whether the kind is a reserved keyword (cannot
// be used as an identifier).
func (k Kind) IsKeyword() bool {
	k.ensureValid()
	return kindMetadata[k].IsKeyword
}

func (k Kind) IsEqualityOperator() bool { return k.hasCategory(catEqualityOp) }

func (k Kind) IsOrderingOperator() bool { return k.hasCategory(catOrderingOp) }

// IsComparisonOperator is the union of equality and ordering — every
// operator that compares two values and yields a boolean.
func (k Kind) IsComparisonOperator() bool { return k.hasCategory(catEqualityOp | catOrderingOp) }

func (k Kind) IsLogicalOperator() bool { return k.hasCategory(catLogicalOp) }

func (k Kind) IsMatchingOperator() bool { return k.hasCategory(catMatchingOp) }

func (k Kind) IsNullOperator() bool { return k.hasCategory(catNullOp) }

// IsBinaryOperator reports whether the kind takes two operands —
// comparison, logical, matching, or null-test (IS).
func (k Kind) IsBinaryOperator() bool {
	return k.hasCategory(catEqualityOp | catOrderingOp | catLogicalOp | catMatchingOp | catNullOp)
}

// IsUnaryOperator reports whether the kind takes one operand — only
// NOT today.
func (k Kind) IsUnaryOperator() bool { return k.hasCategory(catUnaryOp) }

// IsOperator reports whether the kind is any operator (unary or
// binary).
func (k Kind) IsOperator() bool {
	return k.hasCategory(catEqualityOp | catOrderingOp | catLogicalOp | catMatchingOp | catUnaryOp | catNullOp)
}

// IsDelimiter reports whether the kind is structural punctuation —
// parens, brackets, commas.
func (k Kind) IsDelimiter() bool { return k.hasCategory(catDelimiter) }

// Operator-precedence levels, modeled on PostgreSQL's lexical-syntax
// table — higher values bind tighter.
const (
	PrecedenceLowest = iota
	PrecedenceOR
	PrecedenceAND
	PrecedenceNOT
	PrecedenceCMP
	PrecedenceMatch
	PrecedenceIndex
)

func (k Kind) Precedence() int {
	if !k.IsValid() {
		return PrecedenceLowest
	}
	return kindMetadata[k].Precedence
}

// KindOf maps an identifier string onto its Kind — keywords match
// case-insensitively and return the corresponding keyword kind; all
// other strings yield IDENT. The all-lowercase fast path skips the
// allocation in [strings.ToLower] for the common case where the
// caller already supplies lowercase text.
func KindOf(ident string) Kind {
	kw, _ := keywordKinds()
	if !hasUpperASCII(ident) {
		if k, ok := kw[ident]; ok {
			return k
		}
		return IDENT
	}
	if k, ok := kw[strings.ToLower(ident)]; ok {
		return k
	}
	return IDENT
}

func hasUpperASCII(s string) bool {
	for i := range len(s) {
		if c := s[i]; c >= 'A' && c <= 'Z' {
			return true
		}
	}
	return false
}

func IsKeyword(ident string) bool { return KindOf(ident).IsKeyword() }

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

func IsLiteralChar(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
}

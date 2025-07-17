package parser

import (
	"errors"
	"fmt"
	"unicode/utf8"
)

// TokenKind represents the type of a token in the lexer.
type TokenKind int

// Token type constants define all possible token kinds.
const (
	ERROR TokenKind = iota
	EOF
	IDENTIFIER
	STRING
	NUMBER
	BOOLEAN
	EQ     // =
	NE     // !=
	GT     // >
	GE     // >=
	LT     // <
	LE     // <=
	LIKE   // LIKE
	IN     // IN
	AND    // AND
	OR     // OR
	NOT    // NOT
	COMMA  //,
	LPAREN // (
	RPAREN // )
	//LBRACKET // [
	//RBRACKET // ]
)

// tokenKindNames maps token kinds to their string representations.
var tokenKindNames = map[TokenKind]string{
	ERROR:      "ERROR",
	EOF:        "EOF",
	IDENTIFIER: "IDENTIFIER",
	STRING:     "STRING",
	NUMBER:     "NUMBER",
	BOOLEAN:    "BOOLEAN",
	EQ:         "EQ",
	NE:         "NE",
	GT:         "GT",
	GE:         "GE",
	LT:         "LT",
	LE:         "LE",
	LIKE:       "LIKE",
	IN:         "IN",
	AND:        "AND",
	OR:         "OR",
	NOT:        "NOT",
	COMMA:      "COMMA",
	LPAREN:     "LPAREN",
	RPAREN:     "RPAREN",
	//LBRACKET:   "LBRACKET",
	//RBRACKET:   "RBRACKET",
}

// String returns the string representation of a TokenKind.
func (t TokenKind) String() string {
	return tokenKindNames[t]
}

func (t TokenKind) Is(test TokenKind) bool {
	return t == test
}

// reservedWords maps reserved keywords to their corresponding token kinds.
var reservedWords = map[string]TokenKind{
	"TRUE":  BOOLEAN,
	"true":  BOOLEAN,
	"FALSE": BOOLEAN,
	"false": BOOLEAN,
	"AND":   AND,
	"and":   AND,
	"OR":    OR,
	"or":    OR,
	"NOT":   NOT,
	"not":   NOT,
	"LIKE":  LIKE,
	"like":  LIKE,
	"IN":    IN,
	"in":    IN,
}

// LookupTokenKind checks if a string is a reserved keyword and returns
// the corresponding TokenKind. If not found, returns IDENTIFIER.
func LookupTokenKind(input string) TokenKind {
	if kind, exists := reservedWords[input]; exists {
		return kind
	}
	return IDENTIFIER
}

// Position represents a position in the source code with line and column numbers.
type Position struct {
	lineNum   int
	columnNum int
}

// Line returns the line number of the position (1-based).
func (p *Position) Line() int {
	return p.lineNum
}

// Column returns the column number of the position (1-based).
func (p *Position) Column() int {
	return p.columnNum
}

// String returns a formatted string representation of the position.
func (p *Position) String() string {
	return fmt.Sprintf("line:column(%d:%d)", p.lineNum, p.columnNum)
}

// Token represents a lexical token with its kind, value, and position.
type Token struct {
	kind     TokenKind
	value    string
	position Position
}

// NewIllegalToken creates an error token for illegal characters.
func NewIllegalToken(char rune, pos Position) Token {
	pos.columnNum = max(pos.columnNum-1, 1)
	err := fmt.Errorf("illegal character '%c' at %s", char, pos.String())
	return NewErrorToken(err, pos)
}

// NewErrorToken creates an error token with the given error and position.
func NewErrorToken(err error, pos Position) Token {
	if err == nil {
		err = errors.New("unexpected error")
	}

	return Token{
		kind:     ERROR,
		value:    err.Error(),
		position: pos,
	}
}

// NewEOFToken creates an end-of-file token at the given position.
func NewEOFToken(pos Position) Token {
	return Token{
		kind:     EOF,
		value:    "",
		position: pos,
	}
}

// NewToken creates a new token with the specified kind, value, and position.
// It adjusts the column position based on the token's length.
func NewToken(kind TokenKind, val string, pos Position) Token {
	pos.columnNum = max(pos.columnNum-utf8.RuneCountInString(val), 1)

	return Token{
		kind:     kind,
		value:    val,
		position: pos,
	}
}

// Kind returns the token's kind.
func (t *Token) Kind() TokenKind {
	return t.kind
}

// Value returns the token's string value.
func (t *Token) Value() string {
	return t.value
}

// Position returns the token's position in the source.
func (t *Token) Position() Position {
	return t.position
}

// String returns a formatted string representation of the token.
func (t *Token) String() string {
	return fmt.Sprintf("Token{type: %s, value: %s, position: %s}",
		t.kind.String(), t.value, t.position.String())
}

package parser

import (
	"fmt"
	"unicode/utf8"
)

type TokenType int

const (
	ERROR TokenType = iota
	ILLEGAL
	EOF
	IDENTIFIER
	STRING
	INTEGER
	DECIMAL
	TRUE
	FALSE
	NULL
	EQ      // =
	NE      // !=
	GT      // >
	GE      // >=
	LT      // <
	LE      // <=
	LIKE    // LIKE
	IS      // IS
	IN      // IN
	AND     // AND
	OR      // OR
	NOT     // NOT
	LPAREN  // (
	RPAREN  // )
	LSQUARE // [
	RSQUARE // ]
)

var tokenTypes = map[TokenType]string{
	ERROR:      "ERROR",
	ILLEGAL:    "ILLEGAL",
	EOF:        "EOF",
	IDENTIFIER: "IDENTIFIER",
	STRING:     "STRING",
	INTEGER:    "INTEGER",
	DECIMAL:    "DECIMAL",
	TRUE:       "TRUE",
	FALSE:      "FALSE",
	NULL:       "NULL",
	EQ:         "EQ",
	NE:         "NE",
	GT:         "GT",
	GE:         "GE",
	LT:         "LT",
	LE:         "LE",
	LIKE:       "LIKE",
	IS:         "IS",
	IN:         "IN",
	AND:        "AND",
	OR:         "OR",
	NOT:        "NOT",
	LPAREN:     "LPAREN",
	RPAREN:     "RPAREN",
	LSQUARE:    "LSQUARE",
	RSQUARE:    "RSQUARE",
}

func (t TokenType) String() string {
	tokenName, exists := tokenTypes[t]
	if exists {
		return tokenName
	}
	return tokenTypes[ERROR]
}

var keywords = map[string]TokenType{
	"TRUE":  TRUE,
	"true":  TRUE,
	"FALSE": FALSE,
	"false": FALSE,
	"NULL":  NULL,
	"null":  NULL,
	"AND":   AND,
	"and":   AND,
	"OR":    OR,
	"or":    OR,
	"NOT":   NOT,
	"not":   NOT,
	"LIKE":  LIKE,
	"like":  LIKE,
	"IS":    IS,
	"is":    IS,
	"IN":    IN,
	"in":    IN,
}

func LookupTokenType(identifier string) TokenType {
	if tokenType, exists := keywords[identifier]; exists {
		return tokenType
	}
	return IDENTIFIER
}

type Position struct {
	line   int
	column int
}

func (p *Position) Line() int {
	return p.line
}

func (p *Position) Column() int {
	return p.column
}

func (p *Position) String() string {
	return fmt.Sprintf("line:column(%d:%d)", p.line, p.column)
}

type Token struct {
	tokenType TokenType
	value     string
	position  Position
}

func NewIllegalToken(character rune, position Position) Token {
	position.column = max(position.column-1, 1)
	return Token{
		tokenType: ILLEGAL,
		value:     string(character),
		position:  position,
	}
}

func NewEOFToken(position Position) Token {
	return Token{
		tokenType: EOF,
		value:     "",
		position:  position,
	}
}

func NewErrorToken(err error, position Position) Token {
	if err == nil {
		panic("error is nil")
	}
	return Token{
		tokenType: ERROR,
		value:     err.Error(),
		position:  position,
	}
}

func NewToken(tokenType TokenType, value string, position Position) Token {
	position.column = max(position.column-utf8.RuneCountInString(value), 1)
	return Token{
		tokenType: tokenType,
		value:     value,
		position:  position,
	}
}

func (t *Token) Type() TokenType {
	return t.tokenType
}

func (t *Token) Value() string {
	return t.value
}

func (t *Token) Position() Position {
	return t.position
}

func (t *Token) String() string {
	return fmt.Sprintf("Token{type: %s, value: %s, position: %s}",
		t.tokenType.String(), t.value, t.position.String())
}

package parser

import "fmt"

type TokenType int

const (
	ERROR TokenType = iota
	ILLEGAL
	EOF
	IDENTIFIER
	STRING
	INT
	FLOAT
	TRUE
	FALSE
	NULL
	EQ
	NEQ
	GT
	GTE
	LT
	LTE
	LIKE
	IS
	IN
	AND
	OR
	NOT
	COMMA
	QUOTE
	LPAREN
	RPAREN
	LBRACKET
	RBRACKET
	SEMICOLON
)

var tokenTypes = map[TokenType]string{
	ERROR:      "ERROR",
	ILLEGAL:    "ILLEGAL",
	EOF:        "EOF",
	IDENTIFIER: "IDENTIFIER",
	STRING:     "STRING",
	INT:        "INT",
	FLOAT:      "FLOAT",
	TRUE:       "TRUE",
	FALSE:      "FALSE",
	NULL:       "NULL",
	LIKE:       "LIKE",
	AND:        "AND",
	OR:         "OR",
	NOT:        "NOT",
	IN:         "IN",
	IS:         "IS",
	EQ:         "EQ",
	NEQ:        "NEQ",
	GT:         "GT",
	GTE:        "GTE",
	LT:         "LT",
	LTE:        "LTE",
	COMMA:      "COMMA",
	QUOTE:      "QUOTE",
	LPAREN:     "LPAREN",
	RPAREN:     "RPAREN",
	LBRACKET:   "LBRACKET",
	RBRACKET:   "RBRACKET",
	SEMICOLON:  "SEMICOLON",
}

func (t TokenType) String() string {
	s, ok := tokenTypes[t]
	if ok {
		return s
	}
	return tokenTypes[ERROR]
}

func (t TokenType) IsError() bool {
	return t == ERROR
}

func (t TokenType) IsIllegal() bool {
	return t == ILLEGAL
}

func (t TokenType) IsEOF() bool {
	return t == EOF
}

func (t TokenType) IsIdentifier() bool {
	return t == IDENTIFIER
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

func LookupTokenType(ident string) TokenType {
	if tok, ok := keywords[ident]; ok {
		return tok
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
	_type    TokenType
	value    string
	position Position
}

func NewToken(_type TokenType, value string, position Position) Token {
	position.column = max(position.column-len(value), 1)
	return Token{_type: _type, value: value, position: position}
}

func (t *Token) Type() TokenType {
	return t._type
}

func (t *Token) Value() string {
	return t.value
}

func (t *Token) Position() Position {
	return t.position
}

func (t *Token) String() string {
	return fmt.Sprintf("Token{type: %s, value: %s, position: %s}", t._type, t.value, t.position.String())
}

package parser

import (
	"testing"
)

func TestNewLexer(t *testing.T) {
	lexer := NewLexer("name = 'Tom' AND age >= 18 or age < 15;")
	tokens, err := lexer.Tokenize()
	if err != nil {
		t.Fatal(err)
	}
	for _, token := range tokens {
		t.Log(token.String())
	}
}

package lexer_test

import (
	"testing"

	"github.com/Tangerg/lynx/core/vectorstore/filter/lexer"
	"github.com/Tangerg/lynx/core/vectorstore/filter/token"
)

func tokenize(t *testing.T, input string) []token.Token {
	t.Helper()
	l, err := lexer.NewLexer(input)
	if err != nil {
		t.Fatal(err)
	}
	return l.Tokens()
}

func kinds(tokens []token.Token) []token.Kind {
	out := make([]token.Kind, 0, len(tokens))
	for _, tk := range tokens {
		out = append(out, tk.Kind)
	}
	return out
}

func TestNewLexer_RejectsEmpty(t *testing.T) {
	if _, err := lexer.NewLexer(""); err == nil {
		t.Fatal("empty input must error")
	}
}

func TestLexer_SimpleEquality(t *testing.T) {
	tokens := tokenize(t, `name == 'john'`)
	want := []token.Kind{token.IDENT, token.EQ, token.STRING, token.EOF}
	got := kinds(tokens)
	if len(got) != len(want) {
		t.Fatalf("got %d tokens, want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("token[%d]=%s, want %s", i, got[i].Name(), want[i].Name())
		}
	}
}

func TestLexer_AllOperators(t *testing.T) {
	tokens := tokenize(t, `== != < <= > >= AND OR NOT IN LIKE`)
	want := []token.Kind{
		token.EQ, token.NE, token.LT, token.LE, token.GT, token.GE,
		token.AND, token.OR, token.NOT, token.IN, token.LIKE, token.EOF,
	}
	got := kinds(tokens)
	for i, k := range want {
		if got[i] != k {
			t.Errorf("token[%d]=%s, want %s", i, got[i].Name(), k.Name())
		}
	}
}

func TestLexer_NumericLiterals(t *testing.T) {
	tokens := tokenize(t, `42 3.14 -7 -2.5`)
	wantLits := []string{"42", "3.14", "-7", "-2.5"}
	for i, want := range wantLits {
		if !tokens[i].Kind.Is(token.NUMBER) {
			t.Errorf("token[%d] kind=%s, want NUMBER", i, tokens[i].Kind.Name())
		}
		if tokens[i].Literal != want {
			t.Errorf("token[%d] literal=%q, want %q", i, tokens[i].Literal, want)
		}
	}
}

func TestLexer_StringEscapes(t *testing.T) {
	tokens := tokenize(t, `'a\nb' 'it\'s'`)
	if tokens[0].Literal != "a\nb" {
		t.Errorf("token[0]=%q, want %q", tokens[0].Literal, "a\nb")
	}
	if tokens[1].Literal != "it's" {
		t.Errorf("token[1]=%q, want %q", tokens[1].Literal, "it's")
	}
}

func TestLexer_Keywords_CaseInsensitive(t *testing.T) {
	tokens := tokenize(t, `and AND And or OR Or not NOT`)
	wantKinds := []token.Kind{
		token.AND, token.AND, token.AND,
		token.OR, token.OR, token.OR,
		token.NOT, token.NOT,
	}
	for i, k := range wantKinds {
		if tokens[i].Kind != k {
			t.Errorf("token[%d]=%s, want %s", i, tokens[i].Kind.Name(), k.Name())
		}
	}
}

func TestLexer_BooleanLiterals(t *testing.T) {
	tokens := tokenize(t, `true false TRUE FALSE`)
	want := []token.Kind{token.TRUE, token.FALSE, token.TRUE, token.FALSE}
	for i, k := range want {
		if tokens[i].Kind != k {
			t.Errorf("token[%d]=%s, want %s", i, tokens[i].Kind.Name(), k.Name())
		}
	}
}

func TestLexer_Identifiers(t *testing.T) {
	tokens := tokenize(t, `foo foo_bar foo123`)
	wantLits := []string{"foo", "foo_bar", "foo123"}
	for i, want := range wantLits {
		if !tokens[i].Kind.Is(token.IDENT) {
			t.Errorf("token[%d] kind=%s", i, tokens[i].Kind.Name())
		}
		if tokens[i].Literal != want {
			t.Errorf("token[%d]=%q", i, tokens[i].Literal)
		}
	}
}

func TestLexer_Punctuation(t *testing.T) {
	tokens := tokenize(t, `( ) [ ] ,`)
	want := []token.Kind{token.LPAREN, token.RPAREN, token.LBRACK, token.RBRACK, token.COMMA, token.EOF}
	got := kinds(tokens)
	for i, k := range want {
		if got[i] != k {
			t.Errorf("token[%d]=%s, want %s", i, got[i].Name(), k.Name())
		}
	}
}

func TestLexer_IllegalCharacter(t *testing.T) {
	tokens := tokenize(t, `name @ value`)
	foundError := false
	for _, tk := range tokens {
		if tk.Kind.Is(token.ERROR) {
			foundError = true
			break
		}
	}
	if !foundError {
		t.Fatal("@ must produce ERROR token")
	}
}

func TestLexer_IncompleteOperator(t *testing.T) {
	tokens := tokenize(t, `name = 'x'`) // single = is not valid
	if !tokens[1].Kind.Is(token.ERROR) {
		t.Fatalf("token[1]=%s, want ERROR (illegal '=')", tokens[1].Kind.Name())
	}
}

func TestLexer_DecimalRequiresDigit(t *testing.T) {
	tokens := tokenize(t, `1.x`)
	foundError := false
	for _, tk := range tokens {
		if tk.Kind.Is(token.ERROR) {
			foundError = true
			break
		}
	}
	if !foundError {
		t.Fatal("trailing dot without digit must error")
	}
}

func TestLexer_NumericNormalization(t *testing.T) {
	tokens := tokenize(t, `42.000`)
	if tokens[0].Literal != "42" {
		t.Errorf("42.000 normalized to %q, want 42", tokens[0].Literal)
	}
}

func TestLexer_PositionTracking(t *testing.T) {
	l, _ := lexer.NewLexer("a == 1\nb == 2")
	first := l.Scan() // a
	if first.Start.Line != 1 {
		t.Errorf("first.Start.Line = %d, want 1", first.Start.Line)
	}
	// Skip ==, 1, then b.
	l.Scan()           // ==
	l.Scan()           // 1
	second := l.Scan() // b on line 2
	if second.Start.Line != 2 {
		t.Errorf("b.Start.Line = %d, want 2", second.Start.Line)
	}
}

func TestLexer_Reset(t *testing.T) {
	l, _ := lexer.NewLexer(`a == 1`)
	first := l.Tokens()
	l.Reset()
	second := l.Tokens()
	if len(first) != len(second) {
		t.Fatalf("first=%d, second=%d", len(first), len(second))
	}
	for i := range first {
		if first[i].Kind != second[i].Kind {
			t.Errorf("token[%d] differs across Reset", i)
		}
	}
}

func TestLexer_TokenIteratorEarlyExit(t *testing.T) {
	l, _ := lexer.NewLexer(`a == 1`)
	count := 0
	for tk := range l.Token() {
		count++
		if tk.Kind.Is(token.EQ) {
			break
		}
	}
	if count != 2 {
		t.Errorf("yielded %d tokens before break, want 2", count)
	}
}

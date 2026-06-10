package token_test

import (
	"errors"
	"testing"

	"github.com/Tangerg/lynx/core/vectorstore/filter/token"
)

func TestPosition_String(t *testing.T) {
	p := token.NewPosition()
	if got := p.String(); got != "1:1" {
		t.Fatalf("String() = %q, want 1:1", got)
	}
}

func TestPosition_ResetAndAdjust(t *testing.T) {
	p := token.Position{Line: 3, Column: 7}
	p.ResetColumn()
	if p.Line != 3 || p.Column != 1 {
		t.Fatalf("after ResetColumn: %+v", p)
	}

	p.Reset()
	if p.Line != 1 || p.Column != 1 {
		t.Fatalf("after Reset: %+v", p)
	}
}

func TestKindOf_Keywords(t *testing.T) {
	for _, kw := range []string{"AND", "and", "Or", "NOT", "in", "LIKE", "true", "FALSE"} {
		if !token.KindOf(kw).IsKeyword() {
			t.Fatalf("KindOf(%q) was not a keyword", kw)
		}
	}
}

func TestKindOf_Identifier(t *testing.T) {
	if k := token.KindOf("category"); !k.Is(token.IDENT) {
		t.Fatalf("KindOf(category) = %s, want IDENT", k.Name())
	}
}

func TestIsIdentifier(t *testing.T) {
	cases := map[string]bool{
		"foo":     true,
		"foo_bar": true,
		"foo123":  true,
		"_x":      true,
		"":        false,
		"and":     false, // keyword
		"true":    false, // keyword
		"foo bar": false, // space
		"foo-bar": false, // hyphen
	}
	for input, want := range cases {
		if got := token.IsIdentifier(input); got != want {
			t.Errorf("IsIdentifier(%q) = %v, want %v", input, got, want)
		}
	}
}

func TestKindPredicates(t *testing.T) {
	t.Run("literals", func(t *testing.T) {
		for _, k := range []token.Kind{token.NUMBER, token.STRING, token.TRUE, token.FALSE} {
			if !k.IsLiteral() {
				t.Errorf("%s should be literal", k.Name())
			}
		}
		if token.IDENT.IsLiteral() {
			t.Error("IDENT must not be literal")
		}
	})

	t.Run("operators", func(t *testing.T) {
		if !token.AND.IsLogicalOperator() || !token.OR.IsLogicalOperator() {
			t.Error("AND/OR must be logical")
		}
		if !token.NOT.IsUnaryOperator() {
			t.Error("NOT must be unary")
		}
		if !token.EQ.IsEqualityOperator() || !token.NE.IsEqualityOperator() {
			t.Error("EQ/NE must be equality")
		}
		if !token.LT.IsOrderingOperator() || !token.GE.IsOrderingOperator() {
			t.Error("LT/GE must be ordering")
		}
		if !token.IN.IsMatchingOperator() || !token.LIKE.IsMatchingOperator() {
			t.Error("IN/LIKE must be matching")
		}
	})

	t.Run("delimiters", func(t *testing.T) {
		for _, k := range []token.Kind{token.LPAREN, token.RPAREN, token.LBRACK, token.RBRACK, token.COMMA} {
			if !k.IsDelimiter() {
				t.Errorf("%s must be delimiter", k.Name())
			}
		}
	})
}

func TestPrecedenceOrder(t *testing.T) {
	if token.OR.Precedence() >= token.AND.Precedence() {
		t.Error("OR must bind looser than AND")
	}
	if token.AND.Precedence() >= token.NOT.Precedence() {
		t.Error("AND must bind looser than NOT")
	}
	if token.NOT.Precedence() >= token.EQ.Precedence() {
		t.Error("NOT must bind looser than equality")
	}
	if token.EQ.Precedence() >= token.LIKE.Precedence() {
		t.Error("equality must bind looser than LIKE")
	}
	if token.LIKE.Precedence() >= token.LBRACK.Precedence() {
		t.Error("LIKE must bind looser than indexing")
	}
}

func TestOfNumericLiteral_Normalization(t *testing.T) {
	tk := token.OfNumericLiteral("123.000", token.NoPosition, token.NoPosition)
	if !tk.Kind.Is(token.NUMBER) {
		t.Fatalf("kind = %s, want NUMBER", tk.Kind.Name())
	}
	if tk.Literal != "123" {
		t.Fatalf("literal = %q, want 123", tk.Literal)
	}
}

func TestOfNumericLiteral_InvalidProducesError(t *testing.T) {
	tk := token.OfNumericLiteral("not-a-number", token.NoPosition, token.NoPosition)
	if !tk.Kind.Is(token.ERROR) {
		t.Fatalf("kind = %s, want ERROR", tk.Kind.Name())
	}
}

func TestOfError_NilFallback(t *testing.T) {
	tk := token.OfError(nil, token.NoPosition)
	if !tk.Kind.Is(token.ERROR) {
		t.Fatalf("kind = %s, want ERROR", tk.Kind.Name())
	}
	if tk.Literal == "" {
		t.Fatal("nil error must produce non-empty fallback message")
	}
}

func TestOfError_Wraps(t *testing.T) {
	tk := token.OfError(errors.New("something broke"), token.NoPosition)
	if tk.Literal != "something broke" {
		t.Fatalf("literal = %q", tk.Literal)
	}
}

func TestOfLiteral_DispatchesByKind(t *testing.T) {
	cases := []struct {
		kind  token.Kind
		input string
		want  string
	}{
		{token.STRING, "abc", "abc"},
		{token.TRUE, "ignored", "true"},
		{token.FALSE, "ignored", "false"},
	}
	for _, c := range cases {
		tk := token.OfLiteral(c.kind, c.input, token.NoPosition, token.NoPosition)
		if !tk.Kind.Is(c.kind) {
			t.Errorf("kind=%s, want %s", tk.Kind.Name(), c.kind.Name())
		}
		if tk.Literal != c.want {
			t.Errorf("literal=%q, want %q", tk.Literal, c.want)
		}
	}
}

func TestOfLiteral_UnsupportedKindIsError(t *testing.T) {
	tk := token.OfLiteral(token.AND, "and", token.NoPosition, token.NoPosition)
	if !tk.Kind.Is(token.ERROR) {
		t.Fatalf("kind = %s, want ERROR", tk.Kind.Name())
	}
}

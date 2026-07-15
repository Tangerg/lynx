package filter

import "testing"

func scanAll(t *testing.T, input string) []lexeme {
	t.Helper()
	scanner := newScanner(input)
	var tokens []lexeme
	for {
		token, err := scanner.next()
		if err != nil {
			t.Fatal(err)
		}
		tokens = append(tokens, token)
		if token.kind == tokenEOF {
			return tokens
		}
	}
}

func TestScannerVocabulary(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		kinds    []tokenKind
		literals []string
	}{
		{
			name:  "operators and keywords",
			input: `== != < <= > >= AND OR NOT IN LIKE IS NULL`,
			kinds: []tokenKind{
				tokenEqual, tokenNotEqual, tokenLess, tokenLessEqual,
				tokenGreater, tokenGreaterEqual, tokenAnd, tokenOr,
				tokenNot, tokenIn, tokenLike, tokenIs, tokenNull, tokenEOF,
			},
		},
		{
			name:     "literals",
			input:    `42 3.140 -7 1e3 -2.5E-2 true false 'it\'s'`,
			kinds:    []tokenKind{tokenNumber, tokenNumber, tokenNumber, tokenNumber, tokenNumber, tokenTrue, tokenFalse, tokenString, tokenEOF},
			literals: []string{"42", "3.14", "-7", "1000", "-0.025", "true", "false", "it's", ""},
		},
		{
			name:     "unicode identifier",
			input:    `分类_2 == '技术'`,
			kinds:    []tokenKind{tokenIdent, tokenEqual, tokenString, tokenEOF},
			literals: []string{"分类_2", "==", "技术", ""},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokens := scanAll(t, tt.input)
			if len(tokens) != len(tt.kinds) {
				t.Fatalf("token count = %d, want %d", len(tokens), len(tt.kinds))
			}
			for index, kind := range tt.kinds {
				if tokens[index].kind != kind {
					t.Fatalf("token[%d] = %s, want %s", index, tokens[index].kind.string(), kind.string())
				}
				if tt.literals != nil && tokens[index].literal != tt.literals[index] {
					t.Fatalf("token[%d].literal = %q, want %q", index, tokens[index].literal, tt.literals[index])
				}
			}
		})
	}
}

func TestScannerErrorsAndPositions(t *testing.T) {
	for _, input := range []string{`=`, `!`, `-x`, `1.`, `1e`, `1e+`, `'unterminated`, `'bad\q'`, `１２`, `@`} {
		t.Run(input, func(t *testing.T) {
			if _, err := newScanner(input).next(); err == nil {
				t.Fatal("expected scanner error")
			}
		})
	}

	scanner := newScanner("a == 1\nb == 2")
	for range 3 {
		if _, err := scanner.next(); err != nil {
			t.Fatal(err)
		}
	}
	secondLine, err := scanner.next()
	if err != nil {
		t.Fatal(err)
	}
	if secondLine.start != (Position{Line: 2, Column: 1}) {
		t.Fatalf("second line starts at %s", secondLine.start)
	}
}

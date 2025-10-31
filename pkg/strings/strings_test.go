package strings

import (
	"testing"
)

// TestIsQuoted tests the IsQuoted function
func TestIsQuoted(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		// Double quote cases
		{
			name:     "double quoted string",
			input:    `"hello"`,
			expected: true,
		},
		{
			name:     "double quoted empty string",
			input:    `""`,
			expected: true,
		},
		{
			name:     "double quoted single char",
			input:    `"a"`,
			expected: true,
		},
		{
			name:     "double quoted with spaces",
			input:    `"hello world"`,
			expected: true,
		},
		{
			name:     "double quoted with special chars",
			input:    `"hello@#$%"`,
			expected: true,
		},
		{
			name:     "double quoted multiline",
			input:    "\"hello\nworld\"",
			expected: true,
		},

		// Single quote cases
		{
			name:     "single quoted string",
			input:    "'hello'",
			expected: true,
		},
		{
			name:     "single quoted empty string",
			input:    "''",
			expected: true,
		},
		{
			name:     "single quoted single char",
			input:    "'a'",
			expected: true,
		},
		{
			name:     "single quoted with spaces",
			input:    "'hello world'",
			expected: true,
		},
		{
			name:     "single quoted with special chars",
			input:    "'hello@#$%'",
			expected: true,
		},

		// Not quoted cases
		{
			name:     "unquoted string",
			input:    "hello",
			expected: false,
		},
		{
			name:     "only opening double quote",
			input:    `"hello`,
			expected: false,
		},
		{
			name:     "only closing double quote",
			input:    `hello"`,
			expected: false,
		},
		{
			name:     "only opening single quote",
			input:    "'hello",
			expected: false,
		},
		{
			name:     "only closing single quote",
			input:    "hello'",
			expected: false,
		},
		{
			name:     "mixed quotes - double then single",
			input:    `"hello'`,
			expected: false,
		},
		{
			name:     "mixed quotes - single then double",
			input:    `'hello"`,
			expected: false,
		},
		{
			name:     "empty string",
			input:    "",
			expected: false,
		},
		{
			name:     "single character",
			input:    "a",
			expected: false,
		},
		{
			name:     "single double quote",
			input:    `"`,
			expected: false,
		},
		{
			name:     "single single quote",
			input:    "'",
			expected: false,
		},
		{
			name:     "double quote in middle",
			input:    `hel"lo`,
			expected: false,
		},
		{
			name:     "single quote in middle",
			input:    "hel'lo",
			expected: false,
		},
		{
			name:     "quotes inside but not wrapping",
			input:    `a"hello"b`,
			expected: false,
		},
		{
			name:     "nested double quotes",
			input:    `""hello""`,
			expected: true, // Outer quotes match
		},
		{
			name:     "nested single quotes",
			input:    "''hello''",
			expected: true, // Outer quotes match
		},
		{
			name:     "double quoted with internal single quotes",
			input:    `"it's fine"`,
			expected: true,
		},
		{
			name:     "single quoted with internal double quotes",
			input:    `'she said "hi"'`,
			expected: true,
		},

		// Edge cases
		{
			name:     "unicode string double quoted",
			input:    `"你好世界"`,
			expected: true,
		},
		{
			name:     "unicode string single quoted",
			input:    "'你好世界'",
			expected: true,
		},
		{
			name:     "emoji double quoted",
			input:    `"😀🎉"`,
			expected: true,
		},
		{
			name:     "emoji single quoted",
			input:    "'😀🎉'",
			expected: true,
		},
		{
			name:     "only whitespace double quoted",
			input:    `"   "`,
			expected: true,
		},
		{
			name:     "only whitespace single quoted",
			input:    "'   '",
			expected: true,
		},
		{
			name:     "escaped quotes inside (not really escaped by function)",
			input:    `"hello\"world"`,
			expected: true, // Function checks only first and last char
		},
		{
			name:     "tab and newline double quoted",
			input:    "\"\t\n\"",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsQuoted(tt.input)
			if result != tt.expected {
				t.Errorf("IsQuoted(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

// TestUnQuote tests the UnQuote function
func TestUnQuote(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		// Double quote cases
		{
			name:     "double quoted string",
			input:    `"hello"`,
			expected: "hello",
		},
		{
			name:     "double quoted empty string",
			input:    `""`,
			expected: "",
		},
		{
			name:     "double quoted single char",
			input:    `"a"`,
			expected: "a",
		},
		{
			name:     "double quoted with spaces",
			input:    `"hello world"`,
			expected: "hello world",
		},
		{
			name:     "double quoted with special chars",
			input:    `"hello@#$%^&*()"`,
			expected: "hello@#$%^&*()",
		},

		// Single quote cases
		{
			name:     "single quoted string",
			input:    "'hello'",
			expected: "hello",
		},
		{
			name:     "single quoted empty string",
			input:    "''",
			expected: "",
		},
		{
			name:     "single quoted single char",
			input:    "'a'",
			expected: "a",
		},
		{
			name:     "single quoted with spaces",
			input:    "'hello world'",
			expected: "hello world",
		},

		// Not quoted - should return as-is
		{
			name:     "unquoted string",
			input:    "hello",
			expected: "hello",
		},
		{
			name:     "only opening double quote",
			input:    `"hello`,
			expected: `"hello`,
		},
		{
			name:     "only closing double quote",
			input:    `hello"`,
			expected: `hello"`,
		},
		{
			name:     "only opening single quote",
			input:    "'hello",
			expected: "'hello",
		},
		{
			name:     "only closing single quote",
			input:    "hello'",
			expected: "hello'",
		},
		{
			name:     "mixed quotes",
			input:    `"hello'`,
			expected: `"hello'`,
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "single character",
			input:    "a",
			expected: "a",
		},
		{
			name:     "single double quote",
			input:    `"`,
			expected: `"`,
		},
		{
			name:     "single single quote",
			input:    "'",
			expected: "'",
		},

		// Nested quotes
		{
			name:     "nested double quotes",
			input:    `""hello""`,
			expected: `"hello"`,
		},
		{
			name:     "nested single quotes",
			input:    "''hello''",
			expected: "'hello'",
		},
		{
			name:     "double quoted with internal single quotes",
			input:    `"it's fine"`,
			expected: "it's fine",
		},
		{
			name:     "single quoted with internal double quotes",
			input:    `'she said "hi"'`,
			expected: `she said "hi"`,
		},

		// Edge cases
		{
			name:     "unicode double quoted",
			input:    `"你好世界"`,
			expected: "你好世界",
		},
		{
			name:     "unicode single quoted",
			input:    "'你好世界'",
			expected: "你好世界",
		},
		{
			name:     "emoji double quoted",
			input:    `"😀🎉"`,
			expected: "😀🎉",
		},
		{
			name:     "emoji single quoted",
			input:    "'😀🎉'",
			expected: "😀🎉",
		},
		{
			name:     "whitespace only double quoted",
			input:    `"   "`,
			expected: "   ",
		},
		{
			name:     "whitespace only single quoted",
			input:    "'   '",
			expected: "   ",
		},
		{
			name:     "multiline double quoted",
			input:    "\"line1\nline2\"",
			expected: "line1\nline2",
		},
		{
			name:     "tab character double quoted",
			input:    "\"\t\"",
			expected: "\t",
		},
		{
			name:     "carriage return double quoted",
			input:    "\"\r\"",
			expected: "\r",
		},
		{
			name:     "backslash double quoted",
			input:    `"\"`,
			expected: `\`,
		},
		{
			name:     "quotes at start but not end",
			input:    `"hello world`,
			expected: `"hello world`,
		},
		{
			name:     "quotes at end but not start",
			input:    `hello world"`,
			expected: `hello world"`,
		},
		{
			name:     "multiple quotes in content",
			input:    `"it's a "test" string"`,
			expected: `it's a "test" string`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := UnQuote(tt.input)
			if result != tt.expected {
				t.Errorf("UnQuote(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

// TestIsQuoted_UnQuote_RoundTrip tests that quoted strings can be unquoted correctly
func TestIsQuoted_UnQuote_RoundTrip(t *testing.T) {
	tests := []struct {
		name     string
		original string
		quoted   string
	}{
		{
			name:     "simple string double quoted",
			original: "hello",
			quoted:   `"hello"`,
		},
		{
			name:     "simple string single quoted",
			original: "hello",
			quoted:   "'hello'",
		},
		{
			name:     "empty string double quoted",
			original: "",
			quoted:   `""`,
		},
		{
			name:     "empty string single quoted",
			original: "",
			quoted:   "''",
		},
		{
			name:     "string with spaces double quoted",
			original: "hello world",
			quoted:   `"hello world"`,
		},
		{
			name:     "string with special chars double quoted",
			original: "hello@#$%",
			quoted:   `"hello@#$%"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Check that quoted string is detected as quoted
			if !IsQuoted(tt.quoted) {
				t.Errorf("IsQuoted(%q) = false, want true", tt.quoted)
			}

			// Check that unquoting returns original
			unquoted := UnQuote(tt.quoted)
			if unquoted != tt.original {
				t.Errorf("UnQuote(%q) = %q, want %q", tt.quoted, unquoted, tt.original)
			}

			// Check that original is not quoted
			if IsQuoted(tt.original) && tt.original != "" && tt.original != "''" && tt.original != `""` {
				t.Errorf("IsQuoted(%q) = true, want false", tt.original)
			}
		})
	}
}

// TestUnQuote_Idempotent tests that unquoting multiple times gives same result
func TestUnQuote_Idempotent(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "already unquoted",
			input: "hello",
		},
		{
			name:  "double quoted",
			input: `"hello"`,
		},
		{
			name:  "single quoted",
			input: "'hello'",
		},
		{
			name:  "nested double quotes",
			input: `""hello""`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			first := UnQuote(tt.input)
			second := UnQuote(first)

			// Unquoting twice should give same result as unquoting once
			// (unless first unquote reveals nested quotes)
			if IsQuoted(first) {
				// If still quoted after first unquote, second should differ
				if first == second {
					t.Logf("Note: nested quotes detected in %q", tt.input)
				}
			} else {
				// If not quoted after first unquote, second should be same
				if first != second {
					t.Errorf("UnQuote not idempotent: first=%q, second=%q", first, second)
				}
			}
		})
	}
}

// TestIsQuoted_EdgeCases tests edge cases
func TestIsQuoted_EdgeCases(t *testing.T) {
	t.Run("very long quoted string", func(t *testing.T) {
		longString := `"` + string(make([]byte, 10000)) + `"`
		if !IsQuoted(longString) {
			t.Error("IsQuoted failed on very long string")
		}

		unquoted := UnQuote(longString)
		if len(unquoted) != 10000 {
			t.Errorf("UnQuote length = %d, want 10000", len(unquoted))
		}
	})

	t.Run("string with null bytes", func(t *testing.T) {
		input := "\"hello\x00world\""
		if !IsQuoted(input) {
			t.Error("IsQuoted failed on string with null bytes")
		}

		unquoted := UnQuote(input)
		expected := "hello\x00world"
		if unquoted != expected {
			t.Errorf("UnQuote = %q, want %q", unquoted, expected)
		}
	})

	t.Run("only quotes", func(t *testing.T) {
		tests := []struct {
			input    string
			isQuoted bool
			unquoted string
		}{
			{`""`, true, ""},
			{`''`, true, ""},
			{`"`, false, `"`},
			{`'`, false, `'`},
			{`"""`, true, `"`},
			{`'''`, true, `'`},
		}

		for _, tt := range tests {
			if IsQuoted(tt.input) != tt.isQuoted {
				t.Errorf("IsQuoted(%q) = %v, want %v", tt.input, !tt.isQuoted, tt.isQuoted)
			}

			result := UnQuote(tt.input)
			if result != tt.unquoted {
				t.Errorf("UnQuote(%q) = %q, want %q", tt.input, result, tt.unquoted)
			}
		}
	})
}

// BenchmarkIsQuoted benchmarks the IsQuoted function
func BenchmarkIsQuoted(b *testing.B) {
	tests := []struct {
		name  string
		input string
	}{
		{"short quoted", `"hello"`},
		{"short unquoted", "hello"},
		{"long quoted", `"` + string(make([]byte, 1000)) + `"`},
		{"long unquoted", string(make([]byte, 1000))},
		{"empty", ""},
		{"single char", "a"},
	}

	for _, tt := range tests {
		b.Run(tt.name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				_ = IsQuoted(tt.input)
			}
		})
	}
}

// BenchmarkUnQuote benchmarks the UnQuote function
func BenchmarkUnQuote(b *testing.B) {
	tests := []struct {
		name  string
		input string
	}{
		{"short quoted", `"hello"`},
		{"short unquoted", "hello"},
		{"long quoted", `"` + string(make([]byte, 1000)) + `"`},
		{"long unquoted", string(make([]byte, 1000))},
		{"empty quoted", `""`},
		{"empty", ""},
	}

	for _, tt := range tests {
		b.Run(tt.name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				_ = UnQuote(tt.input)
			}
		})
	}
}

// BenchmarkIsQuotedAndUnQuote benchmarks the combined operation
func BenchmarkIsQuotedAndUnQuote(b *testing.B) {
	input := `"hello world"`
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if IsQuoted(input) {
			_ = UnQuote(input)
		}
	}
}

package strings

import "testing"

func TestIsQuoted(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{`"hello"`, true},
		{`""`, true},
		{`"a"`, true},
		{`"hello world"`, true},
		{"\"line\nbreak\"", true},
		{`'hello'`, true},
		{`''`, true},
		{`"hello'`, false}, // mismatched
		{`'hello"`, false},
		{`"hello`, false},
		{`hello"`, false},
		{`hello`, false},
		{``, false},
		{`"`, false},
		{`'`, false},
		{`""hello""`, true}, // outer quotes match
		{`"it's"`, true},
		{`'say "hi"'`, true},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			if got := IsQuoted(tt.in); got != tt.want {
				t.Errorf("IsQuoted(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestUnQuote(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{`"hello"`, `hello`},
		{`'hello'`, `hello`},
		{`""`, ``},
		{`''`, ``},
		{`"a"`, `a`},
		{`"hello`, `"hello`},     // not quoted, unchanged
		{`hello"`, `hello"`},     // not quoted, unchanged
		{`hello`, `hello`},       // not quoted, unchanged
		{``, ``},                 // empty unchanged
		{`"hello'`, `"hello'`},   // mismatched, unchanged
		{`""hello""`, `"hello"`}, // outer pair removed
		{`"it's fine"`, `it's fine`},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			if got := UnQuote(tt.in); got != tt.want {
				t.Errorf("UnQuote(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func BenchmarkIsQuoted(b *testing.B) {
	for b.Loop() {
		_ = IsQuoted(`"hello world"`)
	}
}

func BenchmarkUnQuote(b *testing.B) {
	for b.Loop() {
		_ = UnQuote(`"hello world"`)
	}
}

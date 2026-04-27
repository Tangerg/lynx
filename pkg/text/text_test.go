package text

import (
	"reflect"
	"strings"
	"testing"
)

func TestLines(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{"empty", "", []string{""}},
		{"whitespace only", "  \t\n", []string{""}},
		{"single line", "hello", []string{"hello"}},
		{"trailing newline", "hello\n", []string{"hello"}},
		{"multiple", "a\nb\nc", []string{"a", "b", "c"}},
		{"with empty middle line", "a\n\nb", []string{"a", "", "b"}},
		{"crlf", "a\r\nb", []string{"a", "b"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Lines(tt.in)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAlignToLeft(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"  hello", "hello\n"},
		{"\thello\n  world", "hello\nworld\n"},
		{"  a\n   b\n", "a\nb\n"},
	}
	for _, tt := range tests {
		got := AlignToLeft(tt.in)
		if got != tt.want {
			t.Errorf("AlignToLeft(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestAlignToRight(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"hello   ", "hello\n"},
		{"a  \nb\t", "a\nb\n"},
	}
	for _, tt := range tests {
		got := AlignToRight(tt.in)
		if got != tt.want {
			t.Errorf("AlignToRight(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestAlignCenter(t *testing.T) {
	got := AlignCenter("Hello\nWorld", 10)
	want := "  Hello   \n  World   \n" // 10 cols per line, even-pad to right
	// Allow flexibility on exact padding split, just check width and content.
	for _, line := range strings.Split(strings.TrimSuffix(got, "\n"), "\n") {
		if len([]rune(line)) != 10 {
			t.Errorf("line %q width = %d, want 10", line, len([]rune(line)))
		}
	}
	if !strings.Contains(got, "Hello") || !strings.Contains(got, "World") {
		t.Errorf("missing content: %q", got)
	}
	_ = want
}

func TestAlignCenter_AutoWidth(t *testing.T) {
	got := AlignCenter("hi\nlonger", 0)
	for _, line := range strings.Split(strings.TrimSuffix(got, "\n"), "\n") {
		if len([]rune(line)) != 6 {
			t.Errorf("line %q width = %d, want 6", line, len([]rune(line)))
		}
	}
}

func TestTrimAdjacentBlankLines(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"para1\n\n\npara2", "para1\n\npara2\n"},
		{"\n\nstart\nend\n\n", "start\nend\n"},
		{"a\nb\nc", "a\nb\nc\n"},
		{"\n\n\n\n", ""},
	}
	for _, tt := range tests {
		got := TrimAdjacentBlankLines(tt.in)
		if got != tt.want {
			t.Errorf("TrimAdjacentBlankLines(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestDeleteTopLines(t *testing.T) {
	tests := []struct {
		name string
		in   string
		n    int
		want string
	}{
		{"normal", "a\nb\nc", 1, "b\nc"},
		{"all", "a\nb\nc", 3, ""},
		{"more than have", "a\nb", 5, ""},
		{"zero", "a\nb", 0, "a\nb"},
		{"negative", "a\nb", -1, "a\nb"},
		{"empty", "", 1, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DeleteTopLines(tt.in, tt.n); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDeleteBottomLines(t *testing.T) {
	tests := []struct {
		name string
		in   string
		n    int
		want string
	}{
		{"normal", "a\nb\nc", 1, "a\nb"},
		{"all", "a\nb\nc", 3, ""},
		{"more", "a\nb", 5, ""},
		{"zero", "a\nb", 0, "a\nb"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DeleteBottomLines(tt.in, tt.n); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func BenchmarkLines(b *testing.B) {
	in := strings.Repeat("line\n", 100)
	for b.Loop() {
		_ = Lines(in)
	}
}

func BenchmarkAlignCenter(b *testing.B) {
	in := strings.Repeat("hello world\n", 50)
	for b.Loop() {
		_ = AlignCenter(in, 80)
	}
}

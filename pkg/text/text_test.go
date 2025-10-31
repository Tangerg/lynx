package text

import (
	"reflect"
	"strings"
	"testing"
)

// TestLines tests the Lines function
func TestLines(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "empty string",
			input:    "",
			expected: []string{""},
		},
		{
			name:     "only whitespace",
			input:    "   ",
			expected: []string{""},
		},
		{
			name:     "only tabs",
			input:    "\t\t\t",
			expected: []string{""},
		},
		{
			name:     "only newlines",
			input:    "\n\n\n",
			expected: []string{""},
		},
		{
			name:     "mixed whitespace",
			input:    " \t \n \t ",
			expected: []string{""},
		},
		{
			name:     "single line",
			input:    "Hello World",
			expected: []string{"Hello World"},
		},
		{
			name:     "single line with trailing newline",
			input:    "Hello World\n",
			expected: []string{"Hello World"},
		},
		{
			name:     "two lines",
			input:    "Line 1\nLine 2",
			expected: []string{"Line 1", "Line 2"},
		},
		{
			name:     "two lines with trailing newline",
			input:    "Line 1\nLine 2\n",
			expected: []string{"Line 1", "Line 2"},
		},
		{
			name:     "multiple lines",
			input:    "Line 1\nLine 2\nLine 3\nLine 4",
			expected: []string{"Line 1", "Line 2", "Line 3", "Line 4"},
		},
		{
			name:     "lines with CRLF",
			input:    "Line 1\r\nLine 2\r\nLine 3",
			expected: []string{"Line 1", "Line 2", "Line 3"},
		},
		{
			name:     "lines with CR only",
			input:    "Line 1\rLine 2\rLine 3",
			expected: []string{"Line 1\rLine 2\rLine 3"}, // CR alone doesn't split
		},
		{
			name:     "empty lines between content",
			input:    "Line 1\n\nLine 2",
			expected: []string{"Line 1", "", "Line 2"},
		},
		{
			name:     "multiple empty lines",
			input:    "Line 1\n\n\n\nLine 2",
			expected: []string{"Line 1", "", "", "", "Line 2"},
		},
		{
			name:     "lines with spaces",
			input:    "  Line 1  \n  Line 2  ",
			expected: []string{"  Line 1  ", "  Line 2  "},
		},
		{
			name:     "lines with tabs",
			input:    "\tLine 1\t\n\tLine 2\t",
			expected: []string{"\tLine 1\t", "\tLine 2\t"},
		},
		{
			name:     "unicode content",
			input:    "你好\n世界",
			expected: []string{"你好", "世界"},
		},
		{
			name:     "emoji content",
			input:    "😀\n🎉",
			expected: []string{"😀", "🎉"},
		},
		{
			name:     "very long line",
			input:    strings.Repeat("a", 10000) + "\n" + strings.Repeat("b", 10000),
			expected: []string{strings.Repeat("a", 10000), strings.Repeat("b", 10000)},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Lines(tt.input)
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("Lines() = %v, want %v", result, tt.expected)
			}
		})
	}
}

// TestAlignToLeft tests the AlignToLeft function
func TestAlignToLeft(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty string",
			input:    "",
			expected: "\n",
		},
		{
			name:     "single line no leading space",
			input:    "Hello",
			expected: "Hello\n",
		},
		{
			name:     "single line with leading spaces",
			input:    "   Hello",
			expected: "Hello\n",
		},
		{
			name:     "single line with leading tabs",
			input:    "\t\tHello",
			expected: "Hello\n",
		},
		{
			name:     "single line with mixed leading whitespace",
			input:    " \t Hello",
			expected: "Hello\n",
		},
		{
			name:     "multiple lines with leading spaces",
			input:    "   Line 1\n  Line 2\n    Line 3",
			expected: "Line 1\nLine 2\nLine 3\n",
		},
		{
			name:     "multiple lines some with leading spaces",
			input:    "Line 1\n  Line 2\nLine 3",
			expected: "Line 1\nLine 2\nLine 3\n",
		},
		{
			name:     "preserves trailing spaces",
			input:    "   Hello   \n   World   ",
			expected: "Hello   \nWorld   \n",
		},
		{
			name:     "empty lines",
			input:    "   Line 1\n\n   Line 2",
			expected: "Line 1\n\nLine 2\n",
		},
		{
			name:     "only whitespace lines",
			input:    "   Line 1\n   \n   Line 2",
			expected: "Line 1\n\nLine 2\n",
		},
		{
			name:     "unicode with leading spaces",
			input:    "   你好\n   世界",
			expected: "你好\n世界\n",
		},
		{
			name:     "indented code block",
			input:    "    func main() {\n        fmt.Println(\"Hello\")\n    }",
			expected: "func main() {\nfmt.Println(\"Hello\")\n}\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := AlignToLeft(tt.input)
			if result != tt.expected {
				t.Errorf("AlignToLeft() = %q, want %q", result, tt.expected)
			}
		})
	}
}

// TestAlignToRight tests the AlignToRight function
func TestAlignToRight(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty string",
			input:    "",
			expected: "\n",
		},
		{
			name:     "single line no trailing space",
			input:    "Hello",
			expected: "Hello\n",
		},
		{
			name:     "single line with trailing spaces",
			input:    "Hello   ",
			expected: "Hello\n",
		},
		{
			name:     "single line with trailing tabs",
			input:    "Hello\t\t",
			expected: "Hello\n",
		},
		{
			name:     "single line with mixed trailing whitespace",
			input:    "Hello \t ",
			expected: "Hello\n",
		},
		{
			name:     "multiple lines with trailing spaces",
			input:    "Line 1   \nLine 2  \nLine 3    ",
			expected: "Line 1\nLine 2\nLine 3\n",
		},
		{
			name:     "multiple lines some with trailing spaces",
			input:    "Line 1\nLine 2  \nLine 3",
			expected: "Line 1\nLine 2\nLine 3\n",
		},
		{
			name:     "preserves leading spaces",
			input:    "   Hello   \n   World   ",
			expected: "   Hello\n   World\n",
		},
		{
			name:     "empty lines",
			input:    "Line 1   \n\nLine 2   ",
			expected: "Line 1\n\nLine 2\n",
		},
		{
			name:     "only whitespace lines",
			input:    "Line 1   \n   \nLine 2   ",
			expected: "Line 1\n\nLine 2\n",
		},
		{
			name:     "unicode with trailing spaces",
			input:    "你好   \n世界   ",
			expected: "你好\n世界\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := AlignToRight(tt.input)
			if result != tt.expected {
				t.Errorf("AlignToRight() = %q, want %q", result, tt.expected)
			}
		})
	}
}

// TestAlignCenter tests the AlignCenter function
func TestAlignCenter(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		maxWidth int
		expected string
	}{
		{
			name:     "empty string with width 0",
			input:    "",
			maxWidth: 0,
			expected: "\n",
		},
		{
			name:     "empty string with width 10",
			input:    "",
			maxWidth: 10,
			expected: "          \n",
		},
		{
			name:     "single line exact width",
			input:    "Hello",
			maxWidth: 5,
			expected: "Hello\n",
		},
		{
			name:     "single line narrower than width",
			input:    "Hi",
			maxWidth: 10,
			expected: "    Hi    \n",
		},
		{
			name:     "single line odd width odd text",
			input:    "Hello",
			maxWidth: 11,
			expected: "   Hello   \n",
		},
		{
			name:     "single line even width odd text",
			input:    "Hello",
			maxWidth: 10,
			expected: "  Hello   \n",
		},
		{
			name:     "single line odd width even text",
			input:    "Hi",
			maxWidth: 9,
			expected: "   Hi    \n",
		},
		{
			name:     "auto width from longest line",
			input:    "Short\nMedium Line\nLong",
			maxWidth: 0,
			expected: "   Short   \nMedium Line\n   Long    \n",
		},
		{
			name:     "multiple lines same width",
			input:    "Line 1\nLine 2\nLine 3",
			maxWidth: 10,
			expected: "  Line 1  \n  Line 2  \n  Line 3  \n",
		},
		{
			name:     "multiple lines different widths",
			input:    "A\nBB\nCCC",
			maxWidth: 5,
			expected: "  A  \n BB  \n CCC \n",
		},
		{
			name:     "with leading/trailing spaces removed",
			input:    "  Hello  \n  World  ",
			maxWidth: 10,
			expected: "  Hello   \n  World   \n",
		},
		{
			name:     "empty lines",
			input:    "Line 1\n\nLine 2",
			maxWidth: 10,
			expected: "  Line 1  \n          \n  Line 2  \n",
		},
		{
			name:     "unicode content",
			input:    "你好\n世界",
			maxWidth: 8,
			expected: "   你好   \n   世界   \n",
		},
		{
			name:     "line wider than maxWidth",
			input:    "Very Long Line",
			maxWidth: 5,
			expected: "Very Long Line\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := AlignCenter(tt.input, tt.maxWidth)
			if result != tt.expected {
				t.Errorf("AlignCenter() = %q, want %q", result, tt.expected)
			}
		})
	}
}

// TestTrimAdjacentBlankLines tests the TrimAdjacentBlankLines function
func TestTrimAdjacentBlankLines(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "only whitespace",
			input:    "   \n\t\n  ",
			expected: "",
		},
		{
			name:     "single line",
			input:    "Hello",
			expected: "Hello\n",
		},
		{
			name:     "two lines no blank between",
			input:    "Line 1\nLine 2",
			expected: "Line 1\nLine 2\n",
		},
		{
			name:     "two lines one blank between",
			input:    "Line 1\n\nLine 2",
			expected: "Line 1\n\nLine 2\n",
		},
		{
			name:     "two lines multiple blanks between",
			input:    "Line 1\n\n\n\nLine 2",
			expected: "Line 1\n\nLine 2\n",
		},
		{
			name:     "leading blank lines",
			input:    "\n\nLine 1\nLine 2",
			expected: "Line 1\nLine 2\n",
		},
		{
			name:     "trailing blank lines",
			input:    "Line 1\nLine 2\n\n\n",
			expected: "Line 1\nLine 2\n",
		},
		{
			name:     "leading and trailing blank lines",
			input:    "\n\nLine 1\nLine 2\n\n\n",
			expected: "Line 1\nLine 2\n",
		},
		{
			name:     "multiple paragraphs",
			input:    "Para 1\n\n\nPara 2\n\n\nPara 3",
			expected: "Para 1\n\nPara 2\n\nPara 3\n",
		},
		{
			name:     "blank lines with spaces",
			input:    "Line 1\n  \n\t\n   \nLine 2",
			expected: "Line 1\n\nLine 2\n",
		},
		{
			name:     "preserve single blank between paragraphs",
			input:    "Para 1 Line 1\nPara 1 Line 2\n\nPara 2 Line 1\nPara 2 Line 2",
			expected: "Para 1 Line 1\nPara 1 Line 2\n\nPara 2 Line 1\nPara 2 Line 2\n",
		},
		{
			name:     "complex document",
			input:    "\n\nTitle\n\n\n\nParagraph 1\nLine 2\n\n\n\nParagraph 2\n\n\n",
			expected: "Title\n\nParagraph 1\nLine 2\n\nParagraph 2\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := TrimAdjacentBlankLines(tt.input)
			if result != tt.expected {
				t.Errorf("TrimAdjacentBlankLines() = %q, want %q", result, tt.expected)
			}
		})
	}
}

// TestDeleteTopLines tests the DeleteTopLines function
func TestDeleteTopLines(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		linesToDelete int
		expected      string
	}{
		{
			name:          "empty string",
			input:         "",
			linesToDelete: 1,
			expected:      "",
		},
		{
			name:          "only whitespace",
			input:         "   ",
			linesToDelete: 1,
			expected:      "   ",
		},
		{
			name:          "delete 0 lines",
			input:         "Line 1\nLine 2\nLine 3",
			linesToDelete: 0,
			expected:      "Line 1\nLine 2\nLine 3",
		},
		{
			name:          "delete 1 line from 3",
			input:         "Line 1\nLine 2\nLine 3",
			linesToDelete: 1,
			expected:      "Line 2\nLine 3",
		},
		{
			name:          "delete 2 lines from 3",
			input:         "Line 1\nLine 2\nLine 3",
			linesToDelete: 2,
			expected:      "Line 3",
		},
		{
			name:          "delete all lines",
			input:         "Line 1\nLine 2\nLine 3",
			linesToDelete: 3,
			expected:      "",
		},
		{
			name:          "delete more than available",
			input:         "Line 1\nLine 2",
			linesToDelete: 5,
			expected:      "",
		},
		{
			name:          "single line delete 1",
			input:         "Only Line",
			linesToDelete: 1,
			expected:      "",
		},
		{
			name:          "preserve empty lines",
			input:         "Line 1\n\nLine 3",
			linesToDelete: 1,
			expected:      "\nLine 3",
		},
		{
			name:          "with trailing newline",
			input:         "Line 1\nLine 2\nLine 3\n",
			linesToDelete: 1,
			expected:      "Line 2\nLine 3",
		},
		{
			name:          "negative lines to delete",
			input:         "Line 1\nLine 2\nLine 3",
			linesToDelete: -1,
			expected:      "Line 1\nLine 2\nLine 3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DeleteTopLines(tt.input, tt.linesToDelete)
			if result != tt.expected {
				t.Errorf("DeleteTopLines() = %q, want %q", result, tt.expected)
			}
		})
	}
}

// TestDeleteBottomLines tests the DeleteBottomLines function
func TestDeleteBottomLines(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		linesToDelete int
		expected      string
	}{
		{
			name:          "empty string",
			input:         "",
			linesToDelete: 1,
			expected:      "",
		},
		{
			name:          "only whitespace",
			input:         "   ",
			linesToDelete: 1,
			expected:      "   ",
		},
		{
			name:          "delete 0 lines",
			input:         "Line 1\nLine 2\nLine 3",
			linesToDelete: 0,
			expected:      "Line 1\nLine 2\nLine 3",
		},
		{
			name:          "delete 1 line from 3",
			input:         "Line 1\nLine 2\nLine 3",
			linesToDelete: 1,
			expected:      "Line 1\nLine 2",
		},
		{
			name:          "delete 2 lines from 3",
			input:         "Line 1\nLine 2\nLine 3",
			linesToDelete: 2,
			expected:      "Line 1",
		},
		{
			name:          "delete all lines",
			input:         "Line 1\nLine 2\nLine 3",
			linesToDelete: 3,
			expected:      "",
		},
		{
			name:          "delete more than available",
			input:         "Line 1\nLine 2",
			linesToDelete: 5,
			expected:      "",
		},
		{
			name:          "single line delete 1",
			input:         "Only Line",
			linesToDelete: 1,
			expected:      "",
		},
		{
			name:          "preserve empty lines",
			input:         "Line 1\n\nLine 3",
			linesToDelete: 1,
			expected:      "Line 1\n",
		},
		{
			name:          "with trailing newline",
			input:         "Line 1\nLine 2\nLine 3\n",
			linesToDelete: 1,
			expected:      "Line 1\nLine 2",
		},
		{
			name:          "negative lines to delete",
			input:         "Line 1\nLine 2\nLine 3",
			linesToDelete: -1,
			expected:      "Line 1\nLine 2\nLine 3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DeleteBottomLines(tt.input, tt.linesToDelete)
			if result != tt.expected {
				t.Errorf("DeleteBottomLines() = %q, want %q", result, tt.expected)
			}
		})
	}
}

// TestEdgeCases tests edge cases across all functions
func TestEdgeCases(t *testing.T) {
	t.Run("very long text", func(t *testing.T) {
		longText := strings.Repeat("Line\n", 10000)

		// Lines should handle it
		lines := Lines(longText)
		if len(lines) != 10000 {
			t.Errorf("Lines() returned %d lines, want 10000", len(lines))
		}

		// AlignToLeft should handle it
		result := AlignToLeft(longText)
		if len(result) == 0 {
			t.Error("AlignToLeft() returned empty string for long text")
		}
	})

	t.Run("unicode and emoji", func(t *testing.T) {
		text := "Hello 世界 😀\n你好 World 🎉"

		lines := Lines(text)
		if len(lines) != 2 {
			t.Errorf("Lines() returned %d lines, want 2", len(lines))
		}

		centered := AlignCenter(text, 20)
		if len(centered) == 0 {
			t.Error("AlignCenter() returned empty string for unicode text")
		}
	})

	t.Run("mixed line endings", func(t *testing.T) {
		text := "Line 1\nLine 2\r\nLine 3\n"

		lines := Lines(text)
		if len(lines) < 3 {
			t.Errorf("Lines() returned %d lines, want at least 3", len(lines))
		}
	})
}

// BenchmarkLines benchmarks the Lines function
func BenchmarkLines(b *testing.B) {
	text := "Line 1\nLine 2\nLine 3\nLine 4\nLine 5"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Lines(text)
	}
}

// BenchmarkAlignToLeft benchmarks the AlignToLeft function
func BenchmarkAlignToLeft(b *testing.B) {
	text := "   Line 1\n  Line 2\n    Line 3"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = AlignToLeft(text)
	}
}

// BenchmarkAlignCenter benchmarks the AlignCenter function
func BenchmarkAlignCenter(b *testing.B) {
	text := "Line 1\nLine 2\nLine 3"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = AlignCenter(text, 20)
	}
}

// BenchmarkTrimAdjacentBlankLines benchmarks the TrimAdjacentBlankLines function
func BenchmarkTrimAdjacentBlankLines(b *testing.B) {
	text := "Line 1\n\n\nLine 2\n\n\nLine 3"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = TrimAdjacentBlankLines(text)
	}
}

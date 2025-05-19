package text

import (
	"strings"
	"testing"
)

func TestLines(t *testing.T) {
	complexText := "First line with content\n   Indented line   \n\n\rMixed line endings\r\nTabs\tand\tspaces\n\t  \nLast line"
	result := Lines(complexText)

	expectedCount := 7
	if len(result) != expectedCount {
		t.Errorf("Expected %d lines, got %d", expectedCount, len(result))
	}

	t.Logf("Split result: %v", result)
}

func TestAlignToLeft(t *testing.T) {
	complexText := "    Leading spaces should be removed\n\t\tIndented with tabs\n   Mixed   spacing   \n\n    After blank line"
	result := AlignToLeft(complexText)

	t.Logf("Result:\n%s", result)

	// Check that first line has no leading spaces
	if result[0] == ' ' || result[0] == '\t' {
		t.Errorf("First line still has leading whitespace")
	}
}

func TestAlignToRight(t *testing.T) {
	complexText := "Line with trailing spaces    \n\tTabbed line with spaces at end   \r\nCarriage return line  \n\nBlank line after    "
	result := AlignToRight(complexText)

	t.Logf("Result:\n%s", result)

	lines := Lines(result)
	for i, line := range lines {
		if strings.HasSuffix(line, " ") || strings.HasSuffix(line, "\t") {
			t.Errorf("Line %d still has trailing whitespace: '%s'", i+1, line)
		}
	}
}

func TestAlignCenter(t *testing.T) {
	text := "Hello\nWorld\nThis is a longer line"
	result := AlignCenter(text, 0)
	t.Logf("Result:\n%s", result)
}

func TestTrimAdjacentBlankLines(t *testing.T) {
	complexText := "\n\n\n\nParagraph 1 text.\n\n\n\nParagraph 2 text.\n    \n\r\n\n\nParagraph 3 with mixed blank lines.\n\n\n"
	result := TrimAdjacentBlankLines(complexText)

	t.Logf("Result:\n%s", result)

	// Check that we don't have consecutive blank lines
	if strings.Contains(result, "\n\n\n") {
		t.Errorf("Result still contains more than one consecutive blank line")
	}
}

func TestDeleteTopLines(t *testing.T) {
	complexText := "Header 1\nHeader 2\nHeader 3\nImportant content starts here\nMore content\nEven more content\nAnd final line"

	// Test with different numbers of lines to delete
	result1 := DeleteTopLines(complexText, 3)
	t.Logf("After deleting 3 top lines:\n%s", result1)

	result2 := DeleteTopLines(complexText, 0)
	if result2 != complexText {
		t.Errorf("Deleting 0 lines should return original text")
	}

	result3 := DeleteTopLines(complexText, 10)
	if result3 != "" {
		t.Errorf("Deleting more lines than available should return empty string")
	}
}

func TestDeleteBottomLines(t *testing.T) {
	complexText := "First line of content\nImportant content\nMore relevant stuff\nFootnote 1\nFootnote 2\nFootnote 3\nCopyright notice"

	// Test with different numbers of lines to delete
	result1 := DeleteBottomLines(complexText, 4)
	t.Logf("After deleting 4 bottom lines:\n%s", result1)

	result2 := DeleteBottomLines(complexText, 0)
	if result2 != complexText {
		t.Errorf("Deleting 0 lines should return original text")
	}

	result3 := DeleteBottomLines(complexText, 10)
	if result3 != "" {
		t.Errorf("Deleting more lines than available should return empty string")
	}
}

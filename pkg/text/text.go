package text

import (
	"bufio"
	"strings"
	"unicode"
)

// Lines splits the input text into separate lines.
// It returns:
// - An array with a single empty string if the input is empty or contains only whitespace
// - An array of strings representing each line in the original text otherwise
// Each line in the returned array does not include line terminators (\n, \r\n).
func Lines(text string) []string {
	if strings.TrimSpace(text) == "" {
		return []string{""}
	}
	scanner := bufio.NewScanner(strings.NewReader(text))
	lines := make([]string, 0)
	for scanner.Scan() {
		line := scanner.Text()
		lines = append(lines, line)
	}
	return lines
}

// AlignToLeft removes leading whitespace from all lines in the text.
// This function:
// 1. Splits the text into individual lines
// 2. Trims all leading whitespace characters from each line
// 3. Rejoins the lines with newline characters
// The result is text with all content aligned to the left margin.
func AlignToLeft(text string) string {
	lines := Lines(text)
	sb := strings.Builder{}
	for _, line := range lines {
		sb.WriteString(strings.TrimLeftFunc(line, unicode.IsSpace))
		sb.WriteString("\n")
	}
	return sb.String()
}

// AlignToRight removes trailing whitespace from all lines in the text.
// This function:
// 1. Splits the text into individual lines
// 2. Trims all trailing whitespace characters from each line
// 3. Rejoins the lines with newline characters
// The result is text with no trailing whitespace on any line.
func AlignToRight(text string) string {
	lines := Lines(text)
	sb := strings.Builder{}
	for _, line := range lines {
		sb.WriteString(strings.TrimRightFunc(line, unicode.IsSpace))
		sb.WriteString("\n")
	}
	return sb.String()
}

// AlignCenter centers all lines of text within a specified width.
// If maxWidth is 0 or negative, it automatically finds the width of the longest line.
// Parameters:
// - text: The input text to center
// - maxWidth: The maximum width to center within (optional)
//
// The function:
// 1. Splits the text into lines and trims leading/trailing whitespace from each line
// 2. Determines the width of the longest line if maxWidth is not specified
// 3. For each line, calculates padding needed on both sides (left and right)
// 4. Creates a new text where each line is centered within the maximum width
// 5. Each line in the output has exactly the same width (maxWidth)
// 6. Returns the centered text with lines joined by newlines
//
// Example:
//
//	Input text: "Hello\nWorld" with maxWidth = 10
//	Output:    "   Hello  \n   World  "
func AlignCenter(text string, maxWidth int) string {
	lines := Lines(text)
	for i, line := range lines {
		lines[i] = strings.TrimSpace(line)
		lineWidth := len([]rune(line))
		if lineWidth > maxWidth {
			maxWidth = lineWidth
		}
	}
	sb := strings.Builder{}
	sb.Grow((maxWidth + 1) * len(lines))
	for _, line := range lines {
		lineWidth := len([]rune(line))
		totalPadding := maxWidth - lineWidth
		leftPadding := totalPadding / 2
		rightPadding := totalPadding - leftPadding

		for range leftPadding {
			sb.WriteString(" ")
		}
		sb.WriteString(line)
		for range rightPadding {
			sb.WriteString(" ")
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

// TrimAdjacentBlankLines removes consecutive blank lines from text while preserving paragraph structure.
// The function follows these rules:
//
//  1. If the current line is non-blank:
//     1.1. Check the previous line and if content has been seen before
//     1.1.1. If the previous line was blank AND we've already seen content before,
//     add exactly one blank line to preserve paragraph separation
//     1.1.2. If this is the first content line or follows another content line,
//     add the current line directly without a preceding blank line
//     1.2. Add the current non-blank line to the result
//     1.3. Set prevLineIsBlank flag to false and contentFlag to true
//
//  2. If the current line is blank:
//     2.1. Do not add it directly to the result
//     2.2. Set prevLineIsBlank flag to true to track consecutive blank lines
//
// This ensures that:
// - All leading blank lines are removed completely
// - Multiple consecutive blank lines between paragraphs are reduced to at most one blank line
// - Paragraph structure is maintained while removing excessive whitespace
// - No trailing blank lines are preserved
func TrimAdjacentBlankLines(text string) string {
	lines := Lines(text)

	sb := strings.Builder{}
	prevLineIsBlank := true
	contentFlag := false
	for _, line := range lines {
		curLineIsBlank := strings.TrimSpace(line) == ""
		if !curLineIsBlank {
			if prevLineIsBlank && contentFlag {
				sb.WriteString("\n")
			}
			sb.WriteString(line)
			sb.WriteString("\n")
			prevLineIsBlank = false
			contentFlag = true
			continue
		}
		prevLineIsBlank = true
	}

	return sb.String()
}

// DeleteTopLines removes a specified number of lines from the beginning of the text.
// Parameters:
// - text: The input text to process
// - numberOfLines: The number of lines to remove from the top
//
// Returns:
// - Empty string if the input text has fewer or equal lines than the specified number to delete
// - The remaining text with the specified number of top lines removed otherwise
// - Original text if the input is empty or contains only whitespace
func DeleteTopLines(text string, numberOfLines int) string {
	if strings.TrimSpace(text) == "" {
		return text
	}
	lines := Lines(text)
	if len(lines) <= numberOfLines {
		return ""
	}
	return strings.Join(lines[numberOfLines:], "\n")
}

// DeleteBottomLines removes a specified number of lines from the end of the text.
// Parameters:
// - text: The input text to process
// - numberOfLines: The number of lines to remove from the bottom
//
// Returns:
// - Empty string if the input text has fewer or equal lines than the specified number to delete
// - The remaining text with the specified number of bottom lines removed otherwise
// - Original text if the input is empty or contains only whitespace
func DeleteBottomLines(text string, numberOfLines int) string {
	if strings.TrimSpace(text) == "" {
		return text
	}
	lines := Lines(text)
	if len(lines) <= numberOfLines {
		return ""
	}
	return strings.Join(lines[:len(lines)-numberOfLines], "\n")
}

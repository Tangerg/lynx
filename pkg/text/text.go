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
func Lines(inputText string) []string {
	if strings.TrimSpace(inputText) == "" {
		return []string{""}
	}

	textScanner := bufio.NewScanner(strings.NewReader(inputText))
	textLines := make([]string, 0)

	for textScanner.Scan() {
		currentLine := textScanner.Text()
		textLines = append(textLines, currentLine)
	}

	return textLines
}

// AlignToLeft removes leading whitespace from all lines in the text.
// This function:
// 1. Splits the text into individual lines
// 2. Trims all leading whitespace characters from each line
// 3. Rejoins the lines with newline characters
// The result is text with all content aligned to the left margin.
func AlignToLeft(inputText string) string {
	textLines := Lines(inputText)
	outputBuilder := strings.Builder{}

	for _, textLine := range textLines {
		outputBuilder.WriteString(strings.TrimLeftFunc(textLine, unicode.IsSpace))
		outputBuilder.WriteString("\n")
	}

	return outputBuilder.String()
}

// AlignToRight removes trailing whitespace from all lines in the text.
// This function:
// 1. Splits the text into individual lines
// 2. Trims all trailing whitespace characters from each line
// 3. Rejoins the lines with newline characters
// The result is text with no trailing whitespace on any line.
func AlignToRight(inputText string) string {
	textLines := Lines(inputText)
	outputBuilder := strings.Builder{}

	for _, textLine := range textLines {
		outputBuilder.WriteString(strings.TrimRightFunc(textLine, unicode.IsSpace))
		outputBuilder.WriteString("\n")
	}

	return outputBuilder.String()
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
func AlignCenter(inputText string, maxWidth int) string {
	textLines := Lines(inputText)

	// Trim lines and determine maximum width
	for lineIndex, textLine := range textLines {
		textLines[lineIndex] = strings.TrimSpace(textLine)
		lineWidth := len([]rune(textLine))
		if lineWidth > maxWidth {
			maxWidth = lineWidth
		}
	}

	// Build centered output
	outputBuilder := strings.Builder{}
	outputBuilder.Grow((maxWidth + 1) * len(textLines))

	for _, textLine := range textLines {
		lineWidth := len([]rune(textLine))
		totalPadding := maxWidth - lineWidth
		leftPadding := totalPadding / 2
		rightPadding := totalPadding - leftPadding

		// Add left padding
		for range leftPadding {
			outputBuilder.WriteString(" ")
		}

		// Add line content
		outputBuilder.WriteString(textLine)

		// Add right padding
		for range rightPadding {
			outputBuilder.WriteString(" ")
		}

		outputBuilder.WriteString("\n")
	}

	return outputBuilder.String()
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
func TrimAdjacentBlankLines(inputText string) string {
	textLines := Lines(inputText)

	outputBuilder := strings.Builder{}
	previousLineIsBlank := true
	hasContentBeenSeen := false

	for _, currentLine := range textLines {
		currentLineIsBlank := strings.TrimSpace(currentLine) == ""

		if !currentLineIsBlank {
			// Add paragraph separator if needed
			if previousLineIsBlank && hasContentBeenSeen {
				outputBuilder.WriteString("\n")
			}

			// Add current line
			outputBuilder.WriteString(currentLine)
			outputBuilder.WriteString("\n")

			previousLineIsBlank = false
			hasContentBeenSeen = true
			continue
		}

		previousLineIsBlank = true
	}

	return outputBuilder.String()
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
func DeleteTopLines(inputText string, linesToDelete int) string {
	if strings.TrimSpace(inputText) == "" {
		return inputText
	}

	textLines := Lines(inputText)
	if len(textLines) <= linesToDelete {
		return ""
	}

	return strings.Join(textLines[linesToDelete:], "\n")
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
func DeleteBottomLines(inputText string, linesToDelete int) string {
	if strings.TrimSpace(inputText) == "" {
		return inputText
	}

	textLines := Lines(inputText)
	if len(textLines) <= linesToDelete {
		return ""
	}

	return strings.Join(textLines[:len(textLines)-linesToDelete], "\n")
}

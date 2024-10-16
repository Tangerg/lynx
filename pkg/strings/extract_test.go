package strings

import (
	"fmt"
	"testing"
)

func Test_AlignToLeft(t *testing.T) {
	pageText := "   This is a test.  \n\n\n   Another line.   with spaces\n"
	alignedText := AlignToLeft(pageText)
	t.Log(alignedText)
}

func Test_TrimAdjacentBlankLines(t *testing.T) {
	text := "\n\nYour input text with\n\n\nmultiple blank lines\n    \n\nhere.\n\n"
	result := TrimAdjacentBlankLines(text)
	fmt.Println(result)
}

func TestDeleteTopTextLines(t *testing.T) {
	text := "Line 1\nLine 2\nLine 3\nLine 4"
	result := DeleteTopTextLines(text, 5)
	t.Log(result)
}

func TestDeleteBottomTextLines(t *testing.T) {
	text := "Line 1\nLine 2\nLine 3\nLine 4\r\nLine 5"
	result := DeleteBottomTextLines(text, 1)
	t.Log(result)
}

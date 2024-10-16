package strings

import (
	"regexp"
	"strings"

	pkgSystem "github.com/Tangerg/lynx/pkg/system"
)

var (
	spacesRegex             = regexp.MustCompile(`(?m)^\s+|\s+$`)
	emptyLinesRegex         = regexp.MustCompile(`(?m)^\s*[\r\n]+`)
	whitespaceLineRegex     = regexp.MustCompile(`(?m)^[ \t]*[\r\n]+`)
	multipleBlankLinesRegex = regexp.MustCompile(`(?m)([\r\n]{2,})`)
)

func AlignToLeft(text string) string {
	text = spacesRegex.ReplaceAllString(text, "")
	text = emptyLinesRegex.ReplaceAllString(text, "\n")
	return text
}

func TrimAdjacentBlankLines(text string) string {
	result := whitespaceLineRegex.ReplaceAllString(text, "\n")
	result = multipleBlankLinesRegex.ReplaceAllString(result, "\n\n")
	return result
}

func DeleteTopTextLines(text string, numberOfLines int) string {
	if strings.TrimSpace(text) == "" {
		return text
	}

	lines := strings.Split(text, pkgSystem.LineSeparator())
	if len(lines) <= numberOfLines {
		return ""
	}
	return strings.Join(lines[numberOfLines:], pkgSystem.LineSeparator())
}

func DeleteBottomTextLines(text string, numberOfLines int) string {
	if strings.TrimSpace(text) == "" {
		return text
	}

	lines := strings.Split(text, pkgSystem.LineSeparator())
	if len(lines) <= numberOfLines {
		return ""
	}
	return strings.Join(lines[:len(lines)-numberOfLines], pkgSystem.LineSeparator())
}

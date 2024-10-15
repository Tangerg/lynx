package reader

import (
	pkgStrings "github.com/Tangerg/lynx/pkg/strings"
)

type ExtractedTextFormatter struct {
	leftAlignment                      bool
	numberOfTopPagesToSkipBeforeDelete int
	numberOfTopTextLinesToDelete       int
	numberOfBottomTextLinesToDelete    int
}

func (e *ExtractedTextFormatter) Format(text string, pageNumberOption ...int) string {
	text = pkgStrings.TrimAdjacentBlankLines(text)

	var pageNumber = 0
	if len(pageNumberOption) > 0 {
		pageNumber = pageNumberOption[0]
	}
	if pageNumber > e.numberOfTopPagesToSkipBeforeDelete {
		text = pkgStrings.DeleteTopTextLines(text, e.numberOfTopTextLinesToDelete)
		text = pkgStrings.DeleteBottomTextLines(text, e.numberOfBottomTextLinesToDelete)
	}

	if e.leftAlignment {
		text = pkgStrings.AlignToLeft(text)
	}
	return text
}

type ExtractedTextFormatterBuilder struct {
	extractedTextFormatter *ExtractedTextFormatter
}

func (e *ExtractedTextFormatterBuilder) WithLeftAlignment(leftAlignment bool) *ExtractedTextFormatterBuilder {
	e.extractedTextFormatter.leftAlignment = leftAlignment
	return e
}
func (e *ExtractedTextFormatterBuilder) WithNumberOfTopPagesToSkipBeforeDelete(numberOfTopPagesToSkipBeforeDelete int) *ExtractedTextFormatterBuilder {
	e.extractedTextFormatter.numberOfTopPagesToSkipBeforeDelete = numberOfTopPagesToSkipBeforeDelete
	return e
}
func (e *ExtractedTextFormatterBuilder) WithNumberOfTopTextLinesToDelete(numberOfTopTextLinesToDelete int) *ExtractedTextFormatterBuilder {
	e.extractedTextFormatter.numberOfTopTextLinesToDelete = numberOfTopTextLinesToDelete
	return e
}
func (e *ExtractedTextFormatterBuilder) WithNumberOfBottomTextLinesToDelete(numberOfBottomTextLinesToDelete int) *ExtractedTextFormatterBuilder {
	e.extractedTextFormatter.numberOfBottomTextLinesToDelete = numberOfBottomTextLinesToDelete
	return e
}
func (e *ExtractedTextFormatterBuilder) Build() *ExtractedTextFormatter {
	return e.extractedTextFormatter
}

func NewExtractedTextFormatterBuilder() *ExtractedTextFormatterBuilder {
	return &ExtractedTextFormatterBuilder{
		extractedTextFormatter: &ExtractedTextFormatter{
			leftAlignment:                      false,
			numberOfTopPagesToSkipBeforeDelete: 0,
			numberOfTopTextLinesToDelete:       0,
			numberOfBottomTextLinesToDelete:    0,
		},
	}
}

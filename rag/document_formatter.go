package rag

import "github.com/Tangerg/scope/core/document"

// DocumentFormatter renders one retrieved document for model input.
type DocumentFormatter interface {
	Format(*document.Document) (string, error)
}

// DocumentFormatterFunc adapts a function to [DocumentFormatter].
type DocumentFormatterFunc func(*document.Document) (string, error)

// Format invokes d.
func (d DocumentFormatterFunc) Format(document *document.Document) (string, error) {
	return d(document)
}

type textDocumentFormatter struct{}

func (textDocumentFormatter) Format(document *document.Document) (string, error) {
	return document.Text, nil
}

package rag

import "github.com/Tangerg/scope/core/document"

// DocumentFormatter renders one retrieved document for model input.
type DocumentFormatter interface {
	// Format renders one valid document without mutating or retaining it. The
	// returned text is inserted into model context, so implementations must be
	// deterministic for the same document and return an error for unsupported
	// media rather than silently dropping evidence.
	Format(doc *document.Document) (string, error)
}

type DocumentFormatterFunc func(*document.Document) (string, error)

func (d DocumentFormatterFunc) Format(doc *document.Document) (string, error) {
	return d(doc)
}

type textDocumentFormatter struct{}

func (textDocumentFormatter) Format(doc *document.Document) (string, error) {
	return doc.Text, nil
}

package documentpipeline

import (
	"fmt"

	"github.com/Tangerg/lynx/core/document"
)

// Formatter renders a document according to one frozen formatting policy.
// Consumers that need different representations should use independently
// configured formatters instead of passing consumer-specific modes per call.
type Formatter interface {
	Format(*document.Document) (string, error)
}

// FormatterFunc adapts a function to [Formatter].
type FormatterFunc func(*document.Document) (string, error)

func (f FormatterFunc) Format(doc *document.Document) (string, error) {
	return f(doc)
}

// TextFormatter renders only document text. Its zero value is ready to use.
type TextFormatter struct{}

func (TextFormatter) Format(doc *document.Document) (string, error) {
	if doc == nil {
		return "", ErrNilDocument
	}
	if err := doc.Validate(); err != nil {
		return "", fmt.Errorf("document pipeline: format document: %w", err)
	}
	return doc.Text, nil
}

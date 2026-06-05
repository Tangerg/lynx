package evaluation

import (
	"strings"

	"github.com/Tangerg/lynx/core/document"
)

// Request bundles every input an evaluator needs: the original user
// query, the AI-generated response under review, and the reference
// documents the response was supposed to draw on.
type Request struct {
	// Prompt is the user's original query.
	Prompt string `json:"prompt,omitempty"`

	// Generation is the AI-produced response to score.
	Generation string `json:"generation,omitempty"`

	// Documents is the supporting context (typically RAG-retrieved).
	Documents []*document.Document `json:"documents,omitzero"`
}

// documentsText concatenates non-empty document texts with newline
// separators — fed into evaluator prompts as the "context" variable.
func (req *Request) documentsText() string {
	var texts []string
	for _, doc := range req.Documents {
		if doc.Text != "" {
			texts = append(texts, doc.Text)
		}
	}
	return strings.Join(texts, "\n")
}

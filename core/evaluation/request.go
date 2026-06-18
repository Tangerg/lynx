package evaluation

import (
	"strings"

	"github.com/Tangerg/lynx/core/document"
)

// Request bundles every input an evaluator needs: the original user
// query, the AI-generated response under review, and the reference
// documents the response was supposed to draw on.
type Request struct {
	Prompt string `json:"prompt,omitempty"`
	Generation string `json:"generation,omitempty"`
	Documents []*document.Document `json:"documents,omitzero"`
}

func (r *Request) documentsText() string {
	var texts []string
	for _, doc := range r.Documents {
		if doc == nil || doc.Text == "" {
			continue
		}
		texts = append(texts, doc.Text)
	}
	return strings.Join(texts, "\n")
}

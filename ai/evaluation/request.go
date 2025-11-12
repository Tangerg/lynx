package evaluation

import (
	"strings"

	"github.com/Tangerg/lynx/ai/media/document"
)

// Request encapsulates all the data needed for evaluating an AI response.
// It contains the original user query, the reference documents, and the
// response content to be evaluated.
type Request struct {
	// Prompt The original query/prompt from the user
	Prompt string

	// Generation The AI-generated response content to evaluate
	Generation string

	// Documents Reference documents used to generate the response
	Documents []*document.Document
}

func extractDocuments(req *Request) string {
	var texts []string
	for _, doc := range req.Documents {
		text := doc.Text
		if text != "" {
			texts = append(texts, text)
		}
	}
	return strings.Join(texts, "\n")
}

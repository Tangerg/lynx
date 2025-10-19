package evaluation

import (
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

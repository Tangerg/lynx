package evaluation

import (
	"context"
	"strings"
)

// Evaluator defines the interface for components that evaluate AI responses.
// Implementations can use different strategies and criteria to assess the quality,
// relevance, factual correctness, or other aspects of a response.
type Evaluator interface {
	// Evaluate performs an assessment of an AI response based on the provided request.
	//
	// Parameters:
	//   ctx - Context for request cancellation and timeout
	//   r   - The evaluation request containing the user query, reference documents,
	//         and the response content to be evaluated
	//
	// Returns:
	//   An evaluation response containing the assessment results and nil error if successful
	//   nil and an error if the evaluation fails
	Evaluate(ctx context.Context, r *Request) (*Response, error)
}

// GetSupportingData extracts text content from all documents in the request and
// joins them into a single string for easier processing during evaluation.
//
// This helper function is useful for evaluators that need to analyze the
// relationship between the source documents and the generated response.
//
// Parameters:
//
//	req - The evaluation request containing documents to extract text from
//
// Returns:
//
//	A string containing the concatenated text of all documents, separated by newlines
func GetSupportingData(req *Request) string {
	var texts []string
	for _, document := range req.dataList {
		text := document.Text()
		if text != "" {
			texts = append(texts, text)
		}
	}
	return strings.Join(texts, "\n")
}

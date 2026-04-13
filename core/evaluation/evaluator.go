package evaluation

import (
	"context"
)

// Evaluator defines the interface for components that evaluate AI responses.
// Implementations can use different strategies and criteria to assess the quality,
// relevance, factual correctness, or other aspects of a response.
type Evaluator interface {
	// Evaluate performs an assessment of an AI response based on the provided request.
	//
	// Parameters:
	//   ctx - Context for request cancellation and timeout
	//   req   - The evaluation request containing the user query, reference documents,
	//         and the response content to be evaluated
	//
	// Returns:
	//   An evaluation response containing the assessment results and nil error if successful
	//   nil and an error if the evaluation fails
	Evaluate(ctx context.Context, req *Request) (*Response, error)
}

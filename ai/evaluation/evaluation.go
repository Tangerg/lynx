package evaluation

import "github.com/Tangerg/lynx/ai/content/document"

// Request encapsulates all the data needed for evaluating an AI response.
// It contains the original user query, the reference documents, and the
// response content to be evaluated.
type Request struct {
	userText        string               // The original query/prompt from the user
	dataList        []*document.Document // Reference documents used to generate the response
	responseContent string               // The AI-generated response content to evaluate
}

// NewRequest creates a new evaluation request with the provided data.
//
// Parameters:
//
//	userText        - The original query/prompt from the user
//	dataList        - Reference documents used to generate the response
//	responseContent - The AI-generated response content to evaluate
//
// Returns:
//
//	A new Request instance with the provided data
func NewRequest(userText string, dataList []*document.Document, responseContent string) *Request {
	return &Request{
		userText:        userText,
		dataList:        dataList,
		responseContent: responseContent,
	}
}

// UserText returns the original user query/prompt
func (r *Request) UserText() string {
	return r.userText
}

// DataList returns the reference documents used for generating the response
func (r *Request) DataList() []*document.Document {
	return r.dataList
}

// ResponseContent returns the AI-generated response content being evaluated
func (r *Request) ResponseContent() string {
	return r.responseContent
}

// Response represents the result of an evaluation.
// It includes a pass/fail status, numerical score, textual feedback,
// and additional metadata about the evaluation.
type Response struct {
	pass     bool           // Whether the response passed the evaluation criteria
	score    float32        // Numerical score for the evaluation (typically 0.0-1.0)
	feedback string         // Textual feedback explaining the evaluation results
	metadata map[string]any // Additional evaluation metadata as key-value pairs
}

// NewResponse creates a new evaluation response.
//
// Parameters:
//
//	pass     - Boolean indicating whether the response passed evaluation criteria
//	score    - Numerical score for the evaluation
//	feedback - Textual feedback explaining the evaluation results
//	metadata - Additional evaluation metadata
//
// Returns:
//
//	A new Response instance with the provided evaluation results
func NewResponse(pass bool, score float32, feedback string, metadata map[string]any) *Response {
	if metadata == nil {
		metadata = make(map[string]any)
	}
	return &Response{
		pass:     pass,
		score:    score,
		feedback: feedback,
		metadata: metadata,
	}
}

// Pass returns whether the response passed the evaluation criteria
func (r *Response) Pass() bool {
	return r.pass
}

// Score returns the numerical evaluation score
func (r *Response) Score() float32 {
	return r.score
}

// Feedback returns the textual feedback explaining the evaluation results
func (r *Response) Feedback() string {
	return r.feedback
}

// Metadata returns additional evaluation metadata
func (r *Response) Metadata() map[string]any {
	return r.metadata
}

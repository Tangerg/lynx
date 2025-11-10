package evaluation

// Response represents the result of an evaluation.
// It includes a Pass/fail status, numerical score, textual feedback,
// and additional metadata about the evaluation.
type Response struct {
	// Pass Whether the response passed the evaluation criteria
	Pass bool

	// Score Numerical score for the evaluation (typically 0.0-1.0)
	Score float32

	// Feedback Textual feedback explaining the evaluation results
	Feedback string

	// Metadata Additional evaluation metadata as key-value pairs
	Metadata map[string]any
}

func (r *Response) ensureMetadata() {
	if r.Metadata == nil {
		r.Metadata = make(map[string]any)
	}
}

func (r *Response) Get(key string) (any, bool) {
	r.ensureMetadata()

	v, ok := r.Metadata[key]
	return v, ok
}

func (r *Response) Set(key string, value any) {
	r.ensureMetadata()

	r.Metadata[key] = value
}

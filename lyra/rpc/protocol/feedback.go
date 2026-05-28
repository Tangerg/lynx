package protocol

import "context"

// Feedback is the feedback.* method group — RLHF / quality
// signals. The same data flows can also arrive as a lyra.meta
// CUSTOM event during a run; this REST-shaped path is the
// out-of-band channel.
type Feedback interface {
	SubmitFeedback(ctx context.Context, in FeedbackRequest) error
}

// FeedbackRequest — feedback.submit body.
type FeedbackRequest struct {
	Kind  string         `json:"kind"`            // "thumbs-up" | "thumbs-down" | "note" | "bookmark"
	RefID string         `json:"refId"`           // message id / run id this feedback attaches to
	Value any            `json:"value,omitempty"` // free-form per kind
	Note  string         `json:"note,omitempty"`
}

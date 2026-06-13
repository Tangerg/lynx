package protocol

import "context"

// Feedback is the feedback.* method group — quality signals (API.md §7.7).
type Feedback interface {
	CreateFeedback(ctx context.Context, in FeedbackRequest) error
}

// FeedbackRating is the quality signal on feedback.create (API.md §7.7).
type FeedbackRating string

const (
	FeedbackPositive FeedbackRating = "positive"
	FeedbackNegative FeedbackRating = "negative"
)

// Valid reports whether r is a known rating (empty = unrated).
func (r FeedbackRating) Valid() bool {
	return r == "" || r == FeedbackPositive || r == FeedbackNegative
}

// FeedbackRequest — feedback.create body (API.md §7.7).
type FeedbackRequest struct {
	SessionID string         `json:"sessionId,omitempty"`
	RunID     string         `json:"runId,omitempty"`
	ItemID    string         `json:"itemId,omitempty"`
	Rating    FeedbackRating `json:"rating,omitempty"` // "positive" | "negative"
	Text      string         `json:"text,omitempty"`
}

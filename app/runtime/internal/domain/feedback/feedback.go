// Package feedback defines the durable quality signal collected for runtime
// interactions. A feedback entry is an immutable observation: it may be tied
// to a session, run, or item, but textual feedback without one of those
// references is also useful and therefore valid.
package feedback

import (
	"errors"
	"strings"
	"time"
)

var ErrInvalid = errors.New("feedback: invalid entry")

// Rating is the optional directional signal accompanying an entry.
type Rating string

const (
	RatingPositive Rating = "positive"
	RatingNegative Rating = "negative"
)

// Valid reports whether r is a recognized rating. Empty is valid so callers
// can submit textual feedback without forcing a directional judgement.
func (r Rating) Valid() bool {
	switch r {
	case "", RatingPositive, RatingNegative:
		return true
	default:
		return false
	}
}

// Entry is one append-only quality signal. IDs are intentionally optional
// references rather than foreign keys: feedback can arrive after cleanup, and
// a user may report a general runtime issue without selecting an item.
type Entry struct {
	SessionID string
	RunID     string
	ItemID    string
	Rating    Rating
	Text      string
	CreatedAt time.Time
}

// NewEntry validates and timestamps one feedback observation.
func NewEntry(sessionID, runID, itemID string, rating Rating, text string, createdAt time.Time) (Entry, error) {
	entry := Entry{
		SessionID: sessionID,
		RunID:     runID,
		ItemID:    itemID,
		Rating:    rating,
		Text:      text,
		CreatedAt: createdAt,
	}
	if err := entry.Validate(); err != nil {
		return Entry{}, err
	}
	return entry, nil
}

// Validate protects the durable feedback vocabulary at every persistence
// boundary. A signal needs either a known rating or non-blank text; an empty
// write would carry no product information.
func (e Entry) Validate() error {
	if !e.Rating.Valid() {
		return ErrInvalid
	}
	if e.Rating == "" && strings.TrimSpace(e.Text) == "" {
		return ErrInvalid
	}
	if e.CreatedAt.IsZero() {
		return ErrInvalid
	}
	return nil
}

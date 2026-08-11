// Package feedback defines user-authored quality signals attached to the
// current runtime conversation.
package feedback

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

type Rating string

const (
	Positive Rating = "positive"
	Negative Rating = "negative"
)

func ParseRating(value string) (Rating, error) {
	rating := Rating(strings.TrimSpace(value))
	if err := rating.Validate(); err != nil {
		return "", err
	}
	return rating, nil
}

func (rating Rating) Validate() error {
	if rating != "" && rating != Positive && rating != Negative {
		return fmt.Errorf("feedback rating %q is invalid", rating)
	}
	return nil
}

type Signal struct {
	SessionID string
	RunID     string
	ItemID    string
	Rating    Rating
	Text      string
}

func (signal Signal) Validate() error {
	if err := signal.Rating.Validate(); err != nil {
		return err
	}
	if signal.Rating == "" && strings.TrimSpace(signal.Text) == "" {
		return errors.New("feedback requires a rating or text")
	}
	return nil
}

type Service interface {
	Record(context.Context, Signal) error
}

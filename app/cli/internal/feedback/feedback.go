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

func (r Rating) Validate() error {
	if r != "" && r != Positive && r != Negative {
		return fmt.Errorf("feedback rating %q is invalid", r)
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

func (s Signal) Validate() error {
	if err := s.Rating.Validate(); err != nil {
		return err
	}
	if s.Rating == "" && strings.TrimSpace(s.Text) == "" {
		return errors.New("feedback requires a rating or text")
	}
	return nil
}

type Service interface {
	Record(context.Context, Signal) error
}

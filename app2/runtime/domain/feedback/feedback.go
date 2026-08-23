// Package feedback owns durable user quality signals and their canonical
// Session/Run/Item attribution. Feedback is operational material; it never
// changes the outcome of the Run it describes.
package feedback

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

var ErrInvalid = errors.New("feedback: invalid record")

const MaxTextBytes = 4_000

type Rating string

const (
	Positive Rating = "positive"
	Negative Rating = "negative"
)

func (rating Rating) Valid() bool {
	return rating == Positive || rating == Negative
}

type Subject struct {
	SessionID string
	RunID     string
	ItemID    string
}

func (subject Subject) Empty() bool {
	return subject.SessionID == "" && subject.RunID == "" && subject.ItemID == ""
}

func (subject Subject) MostSpecific() string {
	switch {
	case subject.ItemID != "":
		return "item"
	case subject.RunID != "":
		return "run"
	case subject.SessionID != "":
		return "session"
	default:
		return ""
	}
}

type Attribution struct {
	SessionID string
	RunID     string
	ItemID    string
}

func (attribution Attribution) Matches(subject Subject) bool {
	return (subject.SessionID == "" || subject.SessionID == attribution.SessionID) &&
		(subject.RunID == "" || subject.RunID == attribution.RunID) &&
		(subject.ItemID == "" || subject.ItemID == attribution.ItemID)
}

type Create struct {
	ID          string
	Attribution Attribution
	Rating      Rating
	Text        string
	Now         time.Time
}

type Record struct {
	id          string
	attribution Attribution
	rating      Rating
	text        string
	createdAt   time.Time
}

func New(command Create) (Record, error) {
	value := Record{
		id:          strings.TrimSpace(command.ID),
		attribution: command.Attribution,
		rating:      command.Rating,
		text:        Redact(command.Text),
		createdAt:   command.Now.UTC(),
	}
	if err := value.Validate(); err != nil {
		return Record{}, err
	}
	return value, nil
}

func (record Record) Validate() error {
	if record.id == "" || !record.rating.Valid() || record.createdAt.IsZero() {
		return fmt.Errorf("%w: identity, rating, and timestamp are required", ErrInvalid)
	}
	if !utf8.ValidString(record.text) || len(record.text) > MaxTextBytes {
		return fmt.Errorf("%w: text exceeds the UTF-8 byte limit", ErrInvalid)
	}
	if record.attribution.SessionID == "" &&
		(record.attribution.RunID != "" || record.attribution.ItemID != "") {
		return fmt.Errorf("%w: attribution has no Session owner", ErrInvalid)
	}
	if record.attribution.RunID == "" && record.attribution.ItemID != "" {
		return fmt.Errorf("%w: Item attribution has no Run owner", ErrInvalid)
	}
	if record.attribution.SessionID == "" && record.text == "" {
		return fmt.Errorf("%w: general feedback requires text", ErrInvalid)
	}
	return nil
}

func (record Record) ID() string               { return record.id }
func (record Record) Attribution() Attribution { return record.attribution }
func (record Record) Rating() Rating           { return record.rating }
func (record Record) Text() string             { return record.text }
func (record Record) CreatedAt() time.Time      { return record.createdAt }

var secretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)((?:api[_-]?key|access[_-]?token|password|secret)\s*[:=]\s*)[^\s,;]+`),
	regexp.MustCompile(`(?i)(authorization\s*[:=]\s*)(?:bearer\s+)?[^\s,;]+`),
	regexp.MustCompile(`\bsk-[A-Za-z0-9_-]{16,}\b`),
	regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{12,}\.[A-Za-z0-9_-]{12,}\.[A-Za-z0-9_-]{12,}\b`),
}

func Redact(value string) string {
	value = strings.TrimSpace(strings.Map(func(character rune) rune {
		if character == '\n' || character == '\t' || character >= ' ' {
			return character
		}
		return -1
	}, value))
	for index, pattern := range secretPatterns {
		if index < 2 {
			value = pattern.ReplaceAllString(value, `${1}[redacted]`)
		} else {
			value = pattern.ReplaceAllString(value, `[redacted]`)
		}
	}
	return value
}

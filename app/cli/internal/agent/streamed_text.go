package agent

import (
	"errors"
	"slices"
	"strings"
)

// StreamedText folds text deltas by content-block index. Its zero value is
// ready to use. Missing indices do not create visible separators; when an
// earlier block arrives after a later one, Apply requests an authoritative
// replacement instead of pretending the change can be appended.
type StreamedText struct {
	segments map[int]string
}

// TextMutation describes the cheapest correct way to update a mutable view.
// Replace is false only when Text can be appended to the current projection.
type TextMutation struct {
	Text    string
	Replace bool
}

func NewStreamedText(text string) StreamedText {
	var stream StreamedText
	if text != "" {
		stream.segments = map[int]string{0: text}
	}
	return stream
}

func (s *StreamedText) Apply(delta BlockDelta) (TextMutation, error) {
	if delta.Text == "" {
		return TextMutation{}, errors.New("streamed text delta is empty")
	}
	index := 0
	if delta.ContentIndex != nil {
		index = *delta.ContentIndex
		if index < 0 {
			return TextMutation{}, errors.New("streamed text content index is negative")
		}
	}

	last := s.lastPresentIndex()
	_, existed := s.segments[index]
	appendOnly := last < 0 || index == last || !existed && index > last
	if s.segments == nil {
		s.segments = make(map[int]string)
	}
	s.segments[index] += delta.Text

	if !appendOnly {
		return TextMutation{Text: s.String(), Replace: true}, nil
	}
	text := delta.Text
	if !existed && last >= 0 {
		text = "\n\n" + text
	}
	return TextMutation{Text: text}, nil
}

func (s StreamedText) String() string {
	parts := make([]string, 0, len(s.segments))
	indices := make([]int, 0, len(s.segments))
	for index := range s.segments {
		indices = append(indices, index)
	}
	slices.Sort(indices)
	for _, index := range indices {
		parts = append(parts, s.segments[index])
	}
	return strings.Join(parts, "\n\n")
}

func (s StreamedText) lastPresentIndex() int {
	last := -1
	for index := range s.segments {
		if index > last {
			last = index
		}
	}
	return last
}

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

func (stream *StreamedText) Apply(delta BlockDelta) (TextMutation, error) {
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

	last := stream.lastPresentIndex()
	_, existed := stream.segments[index]
	appendOnly := last < 0 || index == last || !existed && index > last
	if stream.segments == nil {
		stream.segments = make(map[int]string)
	}
	stream.segments[index] += delta.Text

	if !appendOnly {
		return TextMutation{Text: stream.String(), Replace: true}, nil
	}
	text := delta.Text
	if !existed && last >= 0 {
		text = "\n\n" + text
	}
	return TextMutation{Text: text}, nil
}

func (stream StreamedText) String() string {
	parts := make([]string, 0, len(stream.segments))
	indices := make([]int, 0, len(stream.segments))
	for index := range stream.segments {
		indices = append(indices, index)
	}
	slices.Sort(indices)
	for _, index := range indices {
		parts = append(parts, stream.segments[index])
	}
	return strings.Join(parts, "\n\n")
}

func (stream StreamedText) lastPresentIndex() int {
	last := -1
	for index := range stream.segments {
		if index > last {
			last = index
		}
	}
	return last
}

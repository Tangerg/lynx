// Package media defines the [Media] container shared by every modality
// that handles non-text payloads — images on chat / vision requests,
// audio on tts / transcription, attachments on user messages.
package media

import (
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/pkg/mime"
)

// Media wraps an opaque payload (Data) with its MIME type and metadata.
// Data is intentionally typed as any so a single struct can hold raw
// bytes, base64 strings, URLs, or [io.Reader]s — callers extract the
// concrete shape with [Media.DataAsBytes] / [Media.DataAsString].
type Media struct {
	// ID is an optional identifier — set when the caller wants to
	// correlate this payload across logs / audit trails.
	ID string

	// Name is an optional display name (filename, label, ...).
	Name string

	// MimeType identifies the content type. Required.
	MimeType *mime.MIME

	// Data is the actual payload — []byte, string URL, io.Reader,
	// whatever fits the modality. Required.
	Data any

	// Metadata carries free-form annotations.
	Metadata map[string]any
}

// NewMedia builds a [Media]. Both mimeType and data are required.
//
// Example:
//
//	mt, _ := mime.Parse("audio/mpeg")
//	m, err := media.NewMedia(mt, audioBytes)
func NewMedia(mimeType *mime.MIME, data any) (*Media, error) {
	if mimeType == nil {
		return nil, errors.New("media.NewMedia: mimeType must not be nil")
	}
	if data == nil {
		return nil, errors.New("media.NewMedia: data must not be nil")
	}

	return &Media{
		MimeType: mimeType,
		Data:     data,
		Metadata: make(map[string]any),
	}, nil
}

// DataAsBytes returns the payload as []byte. Returns an error when Data
// is nil or holds a non-bytes value.
func (m *Media) DataAsBytes() ([]byte, error) {
	if m.Data == nil {
		return nil, errors.New("media.DataAsBytes: data is nil")
	}

	data, ok := m.Data.([]byte)
	if !ok {
		return nil, fmt.Errorf("media.DataAsBytes: expected []byte, got %T", m.Data)
	}
	return data, nil
}

// DataAsString returns the payload as a string. Returns an error when
// Data is nil or holds a non-string value.
func (m *Media) DataAsString() (string, error) {
	if m.Data == nil {
		return "", errors.New("media.DataAsString: data is nil")
	}

	data, ok := m.Data.(string)
	if !ok {
		return "", fmt.Errorf("media.DataAsString: expected string, got %T", m.Data)
	}
	return data, nil
}

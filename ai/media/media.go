package media

import (
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/pkg/mime"
)

// Media represents a media object containing MIME type information,
// actual data content, and associated metadata.
type Media struct {
	ID       string
	Name     string
	MimeType *mime.MIME
	Data     any
	Metadata map[string]any
}

// NewMedia creates a new Media instance with the specified MIME type and data.
// Returns an error if either mimeType or data is nil.
func NewMedia(mimeType *mime.MIME, data any) (*Media, error) {
	if mimeType == nil {
		return nil, errors.New("mime type is required")
	}

	if data == nil {
		return nil, errors.New("data is required")
	}

	return &Media{
		MimeType: mimeType,
		Data:     data,
		Metadata: make(map[string]any),
	}, nil
}

// DataAsBytes attempts to convert and return the media data as a byte slice.
// Returns an error if the data is nil or not of type []byte.
func (m *Media) DataAsBytes() ([]byte, error) {
	if m.Data == nil {
		return nil, errors.New("data is nil")
	}

	data, ok := m.Data.([]byte)
	if !ok {
		return nil, fmt.Errorf("expected []byte, got %T", m.Data)
	}

	return data, nil
}

// DataAsString attempts to convert and return the media data as a string.
// Returns an error if the data is nil or not of type string.
func (m *Media) DataAsString() (string, error) {
	if m.Data == nil {
		return "", errors.New("data is nil")
	}

	data, ok := m.Data.(string)
	if !ok {
		return "", fmt.Errorf("expected string, got %T", m.Data)
	}

	return data, nil
}

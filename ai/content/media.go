package content

import (
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/Tangerg/lynx/pkg/assert"
	"github.com/Tangerg/lynx/pkg/mime"
)

// generateDefaultName creates a default name for a media object
// using its MIME subtype and a UUID
func generateDefaultName(mimeType *mime.MIME) string {
	return "media-" + mimeType.SubType() + "-" + uuid.New().String()
}

// Media represents a media object with an ID, name, MIME type, and data
type Media struct {
	id       string
	name     string
	mimeType *mime.MIME
	data     any
}

// NewMedia creates a new Media instance with the specified attributes.
// Returns an error if required fields are missing.
// TODO use MediaParam Config struct create
func NewMedia(id string, name string, mimeType *mime.MIME, data any) (*Media, error) {
	if mimeType == nil {
		return nil, errors.New("failed to create media: MIME type is required but was nil")
	}

	if data == nil {
		return nil, errors.New("failed to create media: data is required but was nil")
	}

	if name == "" {
		name = generateDefaultName(mimeType)
	}

	return &Media{
		id:       id,
		name:     name,
		mimeType: mimeType,
		data:     data,
	}, nil
}

// ID returns the media's identifier
func (m *Media) ID() string {
	return m.id
}

// Name returns the media's name
func (m *Media) Name() string {
	return m.name
}

// MimeType returns the media's MIME type
func (m *Media) MimeType() *mime.MIME {
	return m.mimeType
}

// Data returns the media's data as an interface{}
func (m *Media) Data() any {
	return m.data
}

// DataAsBytes attempts to return the media data as a byte slice.
// Returns an error if the data is not of the expected type.
func (m *Media) DataAsBytes() ([]byte, error) {
	data, ok := m.data.([]byte)
	if ok {
		return data, nil
	}

	return nil, fmt.Errorf("failed to convert media data to bytes: expected []byte, got %T", m.data)
}

// DataAsString attempts to return the media data as a string.
// Returns an error if the data is not of the expected type.
func (m *Media) DataAsString() (string, error) {
	data, ok := m.data.(string)
	if ok {
		return data, nil
	}

	return "", fmt.Errorf("failed to convert media data to string: expected string, got %T", m.data)
}

// MediaBuilder implements the builder pattern for creating Media objects
type MediaBuilder struct {
	id       string
	name     string
	mimeType *mime.MIME
	data     any
}

// NewMediaBuilder creates a new MediaBuilder instance
func NewMediaBuilder() *MediaBuilder {
	return &MediaBuilder{}
}

// WithID sets the ID for the media being built
func (b *MediaBuilder) WithID(id string) *MediaBuilder {
	b.id = id
	return b
}

// WithName sets the name for the media being built
func (b *MediaBuilder) WithName(name string) *MediaBuilder {
	b.name = name
	return b
}

// WithMimeType sets the MIME type for the media being built
func (b *MediaBuilder) WithMimeType(mime *mime.MIME) *MediaBuilder {
	b.mimeType = mime
	return b
}

// WithData sets the data for the media being built
func (b *MediaBuilder) WithData(data any) *MediaBuilder {
	b.data = data
	return b
}

// Build creates a new Media instance using the configured parameters.
// Returns an error if required fields are missing.
func (b *MediaBuilder) Build() (*Media, error) {
	return NewMedia(b.id, b.name, b.mimeType, b.data)
}

// MustBuild creates and returns a new Media instance,
// panicking if validation fails.
func (b *MediaBuilder) MustBuild() *Media {
	return assert.ErrorIsNil(b.Build())
}

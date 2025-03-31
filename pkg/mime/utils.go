// Package mime provides utilities for working with MIME types, including parsing,
// creation, detection, and type checking functionality.
package mime

import (
	"errors"
	"io"
	"mime"
	"path"
	"strings"

	"github.com/gabriel-vasile/mimetype"

	"github.com/Tangerg/lynx/pkg/kv"
)

var (
	// ErrorInvalidMimeType is returned when an invalid MIME type is encountered during parsing
	ErrorInvalidMimeType = errors.New("invalid mime type")
)

// New creates a new Mime instance with the specified type and subtype
// Returns the created Mime and any error that occurred during creation
func New(_type string, subType string) (*Mime, error) {
	return NewBuilder().
		WithType(_type).
		WithSubType(subType).
		Build()
}

// newMime is an internal helper that creates a new Mime instance
// Similar to New but ignores errors and always returns a Mime object
func newMime(_type string, subType string) *Mime {
	m, _ := New(_type, subType)
	return m
}

// Parse converts a string representation of a MIME type into a Mime object
// Handles parameters, wildcards, and validates the format according to standards
// Returns an error for malformed MIME type strings
func Parse(mime string) (*Mime, error) {
	index := strings.Index(mime, ";")
	fullType := mime
	if index >= 0 {
		fullType = mime[:index]
	}
	fullType = strings.TrimSpace(fullType)
	if fullType == "" {
		return nil, errors.Join(ErrorInvalidMimeType, errors.New("'mime type' must not be empty"))
	}
	if fullType == wildcardType {
		fullType = "*/*"
	}
	subIndex := strings.Index(fullType, "/")
	if subIndex == -1 {
		return nil, errors.Join(ErrorInvalidMimeType, errors.New("does not contain '/'"))
	}
	if subIndex == len(fullType)-1 {
		return nil, errors.Join(ErrorInvalidMimeType, errors.New("does not contain subtype after '/'"))
	}
	_type := fullType[:subIndex]
	subType := fullType[subIndex+1:]
	if _type == wildcardType && subType != wildcardType {
		return nil, errors.Join(ErrorInvalidMimeType, errors.New("wildcard type is legal only in '*/*' (all mime types)"))
	}
	params := kv.New[string, string]()
	for index < len(mime) {
		nextIndex := index + 1
		quoted := false
		for nextIndex < len(mime) {
			ch := mime[nextIndex]
			if ch == ';' {
				if !quoted {
					break
				}
			} else if ch == '"' {
				quoted = !quoted
			}
			nextIndex++
		}
		param := strings.TrimSpace(mime[index+1 : nextIndex])
		if len(param) > 0 {
			eqIndex := strings.Index(param, "=")
			if eqIndex > 0 {
				attr := strings.TrimSpace(param[:eqIndex])
				value := strings.TrimSpace(param[eqIndex+1:])
				params.Put(attr, value)
			}
		}
		index = nextIndex
	}
	m, err := NewBuilder().
		WithType(_type).
		WithSubType(subType).
		WithParams(params).
		Build()
	if err != nil {
		return nil, errors.Join(ErrorInvalidMimeType, err)
	}
	return m, nil
}

// Detect identifies the MIME type of a byte slice
// Uses the mimetype library for content-based detection
func Detect(b []byte) (*Mime, error) {
	m := mimetype.Detect(b)
	return Parse(m.String())
}

// DetectReader identifies the MIME type of content from an io.Reader
// Uses the mimetype library for content-based detection
func DetectReader(r io.Reader) (*Mime, error) {
	m, err := mimetype.DetectReader(r)
	if err != nil {
		return nil, err
	}
	return Parse(m.String())
}

// DetectFile identifies the MIME type of a file at the given path
// Uses the mimetype library for content-based detection
func DetectFile(path string) (*Mime, error) {
	m, err := mimetype.DetectFile(path)
	if err != nil {
		return nil, err
	}
	return Parse(m.String())
}

// StringTypeByExtension returns the MIME type string associated with a file extension
// Falls back to application/octet-stream if the extension is not recognized
func StringTypeByExtension(filePath string) string {
	m := mime.TypeByExtension(path.Ext(filePath))
	if m == "" {
		m = extToMimeTypeString[strings.ToLower(path.Ext(filePath))]
		if m == "" {
			m = "application/octet-stream"
		}
	}
	return m
}

// TypeByExtension returns a Mime object for the given file extension
// Returns the MIME type and a boolean indicating if the extension was recognized
func TypeByExtension(ext string) (*Mime, bool) {
	mimt, ok := extToMimeType[ext]
	if ok {
		return mimt.Clone(), ok
	}
	return nil, false
}

// IsVideo checks if the given MIME type belongs to the video category
func IsVideo(m *Mime) bool {
	return video.EqualsType(m)
}

// IsAudio checks if the given MIME type belongs to the audio category
func IsAudio(m *Mime) bool {
	return audio.EqualsType(m)
}

// IsImage checks if the given MIME type belongs to the image category
func IsImage(m *Mime) bool {
	return image.EqualsType(m)
}

// IsText checks if the given MIME type belongs to the text category
func IsText(m *Mime) bool {
	return text.EqualsType(m)
}

// IsApplication checks if the given MIME type belongs to the application category
func IsApplication(m *Mime) bool {
	return application.EqualsType(m)
}

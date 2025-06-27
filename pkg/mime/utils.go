// Package mime provides utilities for working with MIME types, including parsing,
// creation, detection, and type checking functionality. MIME (Multipurpose Internet Mail Extensions)
// types are standardized identifiers used to indicate the format of content in internet protocols.
// This package implements RFC standards for MIME type handling and provides tools for content-based
// type detection and extension-based lookups.
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
	// ErrorInvalidMimeType is returned when an invalid MIME type is encountered during parsing.
	// This error indicates that the format of the MIME type string does not conform to the
	// specifications outlined in RFC 2045 and RFC 2046.
	ErrorInvalidMimeType = errors.New("invalid mime type")
)

// New creates a new Mime instance with the specified type and subtype.
// It provides a simple way to create a MIME type with just the primary type and subtype components.
//
// Parameters:
//   - _type: The primary type component (e.g., "text", "application", "image")
//   - subType: The subtype component (e.g., "html", "json", "png")
//
// Examples:
//   - New("text", "html") creates a MIME type for HTML content
//   - New("application", "json") creates a MIME type for JSON data
//
// Returns the created Mime object and any error that occurred during validation.
func New(_type string, subType string) (*Mime, error) {
	return NewBuilder().
		WithType(_type).
		WithSubType(subType).
		Build()
}

// Parse converts a string representation of a MIME type into a Mime object.
// It handles the full MIME type syntax including parameters, validation of format,
// and special cases like wildcards. The function follows the MIME type parsing rules
// defined in RFC 2045.
//
// The function processes:
// 1. The type/subtype portion
// 2. Parameters in the format "key=value"
// 3. Special cases like wildcard types
// 4. Quoted parameter values
//
// Parameters:
//   - mime: The MIME type string to parse (e.g., "text/html; charset=UTF-8")
//
// Examples:
//   - Parse("text/html") returns a MIME type with type="text", subtype="html"
//   - Parse("application/json; charset=utf-8") returns a MIME type with parameters
//   - Parse("*/*") returns a wildcard MIME type matching any type
//
// Returns a Mime object representing the parsed MIME type, or an error if the string
// is malformed or does not conform to MIME type standards.
func Parse(mime string) (*Mime, error) {
	// Find the first semicolon, which separates the type/subtype from parameters
	index := strings.Index(mime, ";")
	fullType := mime
	if index >= 0 {
		fullType = mime[:index]
	}
	fullType = strings.TrimSpace(fullType)

	// Validate the type/subtype portion
	if fullType == "" {
		return nil, errors.Join(ErrorInvalidMimeType, errors.New("'mime type' must not be empty"))
	}

	// Handle the special case of "*" as shorthand for "*/*"
	if fullType == wildcardType {
		fullType = "*/*"
	}

	// Ensure the type/subtype contains a forward slash
	subIndex := strings.Index(fullType, "/")
	if subIndex == -1 {
		return nil, errors.Join(ErrorInvalidMimeType, errors.New("does not contain '/'"))
	}
	if subIndex == len(fullType)-1 {
		return nil, errors.Join(ErrorInvalidMimeType, errors.New("does not contain subtype after '/'"))
	}

	// Extract the type and subtype
	_type := fullType[:subIndex]
	subType := fullType[subIndex+1:]

	// Validate wildcard type usage
	if _type == wildcardType && subType != wildcardType {
		return nil, errors.Join(ErrorInvalidMimeType, errors.New("wildcard type is legal only in '*/*' (all mime types)"))
	}

	// Process parameters (if any)
	params := kv.New[string, string]()
	for index < len(mime) {
		nextIndex := index + 1
		quoted := false

		// Find the end of the current parameter
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

		// Extract and process the parameter
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

	// Build and validate the final MIME type
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

// Detect identifies the MIME type of a byte slice based on its content.
// It uses content analysis techniques from the mimetype library to determine
// the format of the data, which is more reliable than extension-based detection
// for many file formats.
//
// Parameters:
//   - b: The byte slice containing the data to analyze
//
// Examples:
//   - Detect(pngImageBytes) would return image/png MIME type
//   - Detect(jsonDataBytes) would return application/json MIME type
//
// Returns a Mime object representing the detected type, or an error if detection fails.
func Detect(b []byte) (*Mime, error) {
	m := mimetype.Detect(b)
	return Parse(m.String())
}

// DetectReader identifies the MIME type of content from an io.Reader.
// This allows for examining data from sources like files, network connections,
// or any other streaming source without loading the entire content into memory.
// It reads a small portion of data from the beginning of the stream for analysis.
//
// Parameters:
//   - r: An io.Reader providing access to the content to be analyzed
//
// Examples:
//   - DetectReader(fileHandle) would detect the MIME type of a file
//   - DetectReader(httpResponse.Body) would detect the MIME type of HTTP response content
//
// Returns a Mime object representing the detected type, or an error if detection fails
// or if reading from the provided reader fails.
func DetectReader(r io.Reader) (*Mime, error) {
	m, err := mimetype.DetectReader(r)
	if err != nil {
		return nil, err
	}
	return Parse(m.String())
}

// DetectFile identifies the MIME type of a file at the given path.
// It performs content-based detection by reading and analyzing a portion
// of the file's contents, providing more accurate results than extension-based
// detection for most file formats.
//
// Parameters:
//   - path: The file system path to the file to be analyzed
//
// Examples:
//   - DetectFile("/path/to/document.pdf") would detect application/pdf
//   - DetectFile("/path/to/image.jpg") would detect image/jpeg
//
// Returns a Mime object representing the detected type, or an error if the file
// cannot be read or if the detection process fails.
func DetectFile(path string) (*Mime, error) {
	m, err := mimetype.DetectFile(path)
	if err != nil {
		return nil, err
	}
	return Parse(m.String())
}

// StringTypeByExtension returns the MIME type string associated with a file extension.
// It uses both the Go standard library's mime package and an internal mapping to
// determine the appropriate MIME type. If the extension is not recognized,
// it falls back to "application/octet-stream" which is the standard default for
// binary content of unknown type.
//
// Parameters:
//   - filePath: A file path or filename from which to extract the extension
//
// Examples:
//   - StringTypeByExtension("document.pdf") returns "application/pdf"
//   - StringTypeByExtension("image.png") returns "image/png"
//   - StringTypeByExtension("file.unknown") returns "application/octet-stream"
//
// Returns a string representation of the MIME type associated with the file extension.
func StringTypeByExtension(filePath string) string {
	ext := strings.ToLower(path.Ext(filePath))
	m := mime.TypeByExtension(ext)
	if m != "" {
		return m
	}
	m = extMimetypeStringMappings[ext]
	if m == "" {
		m = "application/octet-stream"
	}
	return m
}

// TypeByExtension returns a Mime object for the given file path or filename.
// It extracts the extension from the file path and looks it up in an internal
// mapping of extensions to MIME types. This provides a way to determine the likely
// MIME type of a file based on its extension without examining the file's contents.
//
// Parameters:
//   - filePath: The file path or filename from which to extract the extension
//
// Examples:
//   - TypeByExtension("document.html") returns a Mime object for "text/html"
//   - TypeByExtension("images/photo.jpg") returns a Mime object for "image/jpeg"
//   - TypeByExtension("/path/to/data.json") returns a Mime object for "application/json"
//
// Returns a MIME type object and a boolean indicating if the extension was recognized.
// If the extension is not recognized, returns nil and false.
func TypeByExtension(filePath string) (*Mime, bool) {
	ext := strings.ToLower(path.Ext(filePath))
	mimt, ok := extToMimeTypeMappings[ext]
	if ok {
		return mimt.Clone(), true
	}
	return nil, false
}

// IsVideo checks if the given MIME type belongs to the video category.
// It compares the primary type of the MIME type against the video type.
// This is useful for determining if content represents video media.
//
// Parameters:
//   - m: The MIME type to check
//
// Examples:
//   - IsVideo for "video/mp4" returns true
//   - IsVideo for "video/quicktime" returns true
//   - IsVideo for "image/jpeg" returns false
//
// Returns true if the MIME type has a primary type of "video", false otherwise.
func IsVideo(m *Mime) bool {
	return video.EqualsType(m)
}

// IsAudio checks if the given MIME type belongs to the audio category.
// It compares the primary type of the MIME type against the audio type.
// This helps identify audio content for specialized handling.
//
// Parameters:
//   - m: The MIME type to check
//
// Examples:
//   - IsAudio for "audio/mp3" returns true
//   - IsAudio for "audio/ogg" returns true
//   - IsAudio for "video/mp4" returns false
//
// Returns true if the MIME type has a primary type of "audio", false otherwise.
func IsAudio(m *Mime) bool {
	return audio.EqualsType(m)
}

// IsImage checks if the given MIME type belongs to the image category.
// It compares the primary type of the MIME type against the image type.
// This is useful for identifying image content such as photos and graphics.
//
// Parameters:
//   - m: The MIME type to check
//
// Examples:
//   - IsImage for "image/jpeg" returns true
//   - IsImage for "image/svg+xml" returns true
//   - IsImage for "text/html" returns false
//
// Returns true if the MIME type has a primary type of "image", false otherwise.
func IsImage(m *Mime) bool {
	return image.EqualsType(m)
}

// IsText checks if the given MIME type belongs to the text category.
// It compares the primary type of the MIME type against the text type.
// This helps identify textual content that can be read as characters.
//
// Parameters:
//   - m: The MIME type to check
//
// Examples:
//   - IsText for "text/plain" returns true
//   - IsText for "text/html" returns true
//   - IsText for "application/json" returns false (even though JSON is text-based)
//
// Returns true if the MIME type has a primary type of "text", false otherwise.
func IsText(m *Mime) bool {
	return text.EqualsType(m)
}

// IsApplication checks if the given MIME type belongs to the application category.
// It compares the primary type of the MIME type against the application type.
// The application category includes a wide range of formats for structured data,
// documents, and executable content.
//
// Parameters:
//   - m: The MIME type to check
//
// Examples:
//   - IsApplication for "application/pdf" returns true
//   - IsApplication for "application/json" returns true
//   - IsApplication for "text/html" returns false
//
// Returns true if the MIME type has a primary type of "application", false otherwise.
func IsApplication(m *Mime) bool {
	return application.EqualsType(m)
}

// Package mime provides utilities for working with MIME types, including parsing,
// creation, detection, and type checking functionality. MIME (Multipurpose Internet Mail Extensions)
// types are standardized identifiers used to indicate the format of content in internet protocols.
// This package implements RFC standards for MIME type handling and provides tools for content-based
// type detection and extension-based lookups.
package mime

import (
	"errors"
	"fmt"
	"io"
	"mime"
	"path"
	"strings"

	"github.com/gabriel-vasile/mimetype"

	"github.com/Tangerg/lynx/pkg/assert"
	"github.com/Tangerg/lynx/pkg/maps"
)

var (
	// ErrorInvalidMimeType is returned when an invalid MIME type is encountered during parsing.
	// This error indicates that the format of the MIME type string does not conform to the
	// specifications outlined in RFC 2045 and RFC 2046.
	ErrorInvalidMimeType = errors.New("invalid mime type")
)

// New creates a new MIME instance with the specified type and subtype.
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
// Returns the created MIME object and any error that occurred during validation.
func New(mimeType string, subType string) (*MIME, error) {
	return NewBuilder().
		WithType(mimeType).
		WithSubType(subType).
		Build()
}

func MustNew(mimeType string, subType string) *MIME {
	return assert.Must(New(mimeType, subType))
}

// Parse converts a string representation of a MIME type into a MIME object.
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
// Returns a MIME object representing the parsed MIME type, or an error if the string
// is malformed or does not conform to MIME type standards.
func Parse(mimeString string) (*MIME, error) {
	// Find the first semicolon, which separates the type/subtype from parameters
	semicolonIndex := strings.Index(mimeString, ";")
	typeSubtypeString := mimeString
	if semicolonIndex >= 0 {
		typeSubtypeString = mimeString[:semicolonIndex]
	}
	typeSubtypeString = strings.TrimSpace(typeSubtypeString)

	// Validate the type/subtype portion
	if typeSubtypeString == "" {
		return nil, fmt.Errorf("%w: 'mime type' must not be empty", ErrorInvalidMimeType)
	}

	// Handle the special case of "*" as shorthand for "*/*"
	if typeSubtypeString == wildcardType {
		typeSubtypeString = "*/*"
	}

	// Ensure the type/subtype contains a forward slash
	slashIndex := strings.Index(typeSubtypeString, "/")
	if slashIndex == -1 {
		return nil, fmt.Errorf("%w: does not contain '/'", ErrorInvalidMimeType)
	}
	if slashIndex == len(typeSubtypeString)-1 {
		return nil, fmt.Errorf("%w: does not contain subtype after '/'", ErrorInvalidMimeType)
	}

	// Extract the type and subtype
	primaryType := typeSubtypeString[:slashIndex]
	subType := typeSubtypeString[slashIndex+1:]

	// Validate wildcard type usage
	if primaryType == wildcardType && subType != wildcardType {
		return nil, fmt.Errorf("%w: wildcard type is legal only in '*/*' (all mime types)", ErrorInvalidMimeType)
	}

	// Process parameters (if any)
	parameterMap := maps.NewHashMap[string, string]()
	for semicolonIndex < len(mimeString) {
		nextSemicolonIndex := semicolonIndex + 1
		isQuoted := false

		// Find the end of the current parameter
		for nextSemicolonIndex < len(mimeString) {
			currentChar := mimeString[nextSemicolonIndex]
			if currentChar == ';' {
				if !isQuoted {
					break
				}
			} else if currentChar == '"' {
				isQuoted = !isQuoted
			}
			nextSemicolonIndex++
		}

		// Extract and process the parameter
		parameterString := strings.TrimSpace(mimeString[semicolonIndex+1 : nextSemicolonIndex])
		if len(parameterString) > 0 {
			equalsIndex := strings.Index(parameterString, "=")
			if equalsIndex > 0 {
				paramKey := strings.TrimSpace(parameterString[:equalsIndex])
				paramValue := strings.TrimSpace(parameterString[equalsIndex+1:])
				parameterMap.Put(paramKey, paramValue)
			}
		}
		semicolonIndex = nextSemicolonIndex
	}

	// Build and validate the final MIME type
	mimeResult, err := NewBuilder().
		WithType(primaryType).
		WithSubType(subType).
		WithParams(parameterMap).
		Build()
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrorInvalidMimeType, err)
	}

	return mimeResult, nil
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
// Returns a MIME object representing the detected type, or an error if detection fails.
func Detect(dataBytes []byte) (*MIME, error) {
	detectedMime := mimetype.Detect(dataBytes)
	return Parse(detectedMime.String())
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
// Returns a MIME object representing the detected type, or an error if detection fails
// or if reading from the provided reader fails.
func DetectReader(reader io.Reader) (*MIME, error) {
	detectedMime, err := mimetype.DetectReader(reader)
	if err != nil {
		return nil, err
	}
	return Parse(detectedMime.String())
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
// Returns a MIME object representing the detected type, or an error if the file
// cannot be read or if the detection process fails.
func DetectFile(filePath string) (*MIME, error) {
	detectedMime, err := mimetype.DetectFile(filePath)
	if err != nil {
		return nil, err
	}
	return Parse(detectedMime.String())
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
	fileExtension := strings.ToLower(path.Ext(filePath))

	// First try the standard library's mime package
	mimeTypeString := mime.TypeByExtension(fileExtension)
	if mimeTypeString != "" {
		return mimeTypeString
	}

	// Fall back to internal mapping
	mimeTypeString = extMimetypeStringMappings[fileExtension]
	if mimeTypeString == "" {
		mimeTypeString = "application/octet-stream"
	}

	return mimeTypeString
}

// TypeByExtension returns a MIME object for the given file path or filename.
// It extracts the extension from the file path and looks it up in an internal
// mapping of extensions to MIME types. This provides a way to determine the likely
// MIME type of a file based on its extension without examining the file's contents.
//
// Parameters:
//   - filePath: The file path or filename from which to extract the extension
//
// Examples:
//   - TypeByExtension("document.html") returns a MIME object for "text/html"
//   - TypeByExtension("images/photo.jpg") returns a MIME object for "image/jpeg"
//   - TypeByExtension("/path/to/data.json") returns a MIME object for "application/json"
//
// Returns a MIME type object and a boolean indicating if the extension was recognized.
// If the extension is not recognized, returns nil and false.
func TypeByExtension(filePath string) (*MIME, bool) {
	fileExtension := strings.ToLower(path.Ext(filePath))

	mappedMime, extensionFound := extToMimeTypeMappings[fileExtension]
	if extensionFound {
		return mappedMime.Clone(), true
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
func IsVideo(mimeType *MIME) bool {
	return video.EqualsType(mimeType)
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
func IsAudio(mimeType *MIME) bool {
	return audio.EqualsType(mimeType)
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
func IsImage(mimeType *MIME) bool {
	return image.EqualsType(mimeType)
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
func IsText(mimeType *MIME) bool {
	return text.EqualsType(mimeType)
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
func IsApplication(mimeType *MIME) bool {
	return application.EqualsType(mimeType)
}

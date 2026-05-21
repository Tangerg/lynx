package mime

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/gabriel-vasile/mimetype"

	"github.com/Tangerg/lynx/pkg/assert"
	"github.com/Tangerg/lynx/pkg/maps"
)

// Category prototypes used by the Is* helpers; only their type and
// subtype fields are consulted.
var (
	all         = MIME{_type: wildcardType, subType: wildcardType}
	text        = MIME{_type: "text", subType: wildcardType}
	video       = MIME{_type: "video", subType: wildcardType}
	audio       = MIME{_type: "audio", subType: wildcardType}
	image       = MIME{_type: "image", subType: wildcardType}
	application = MIME{_type: "application", subType: wildcardType}
)

// ErrorInvalidMimeType is returned by [Parse] when the input does not
// conform to RFC 2045 / 2046 syntax.
var ErrorInvalidMimeType = errors.New("invalid mime type")

// New returns a [MIME] with the given primary type and subtype and no
// parameters.
func New(mimeType string, subType string) (*MIME, error) {
	return NewBuilder().
		WithType(mimeType).
		WithSubType(subType).
		Build()
}

// MustNew is like [New] but panics on error.
func MustNew(mimeType string, subType string) *MIME {
	return assert.Must(New(mimeType, subType))
}

// Parse decodes a MIME type string such as "text/html; charset=UTF-8"
// into a [MIME]. A bare "*" is treated as "*/*". Quoted parameter
// values are preserved verbatim. Returns [ErrorInvalidMimeType] if
// the input is malformed.
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

// Detect returns the MIME type inferred from the magic bytes of
// dataBytes.
func Detect(dataBytes []byte) (*MIME, error) {
	detectedMime := mimetype.Detect(dataBytes)
	return Parse(detectedMime.String())
}

// DetectReader returns the MIME type inferred from the leading bytes
// of reader. Only a small prefix is consumed.
func DetectReader(reader io.Reader) (*MIME, error) {
	detectedMime, err := mimetype.DetectReader(reader)
	if err != nil {
		return nil, err
	}
	return Parse(detectedMime.String())
}

// DetectFile returns the MIME type inferred from the contents of the
// file at filePath.
func DetectFile(filePath string) (*MIME, error) {
	detectedMime, err := mimetype.DetectFile(filePath)
	if err != nil {
		return nil, err
	}
	return Parse(detectedMime.String())
}

// IsVideo reports whether mimeType has primary type "video".
func IsVideo(mimeType *MIME) bool {
	return video.EqualsType(mimeType)
}

// IsAudio reports whether mimeType has primary type "audio".
func IsAudio(mimeType *MIME) bool {
	return audio.EqualsType(mimeType)
}

// IsImage reports whether mimeType has primary type "image".
func IsImage(mimeType *MIME) bool {
	return image.EqualsType(mimeType)
}

// IsText reports whether mimeType has primary type "text".
func IsText(mimeType *MIME) bool {
	return text.EqualsType(mimeType)
}

// IsApplication reports whether mimeType has primary type "application".
func IsApplication(mimeType *MIME) bool {
	return application.EqualsType(mimeType)
}

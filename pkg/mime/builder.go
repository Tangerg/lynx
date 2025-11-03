// Package mime provides comprehensive functionality for handling MIME (Multipurpose Internet Mail Extensions) types,
// which are standardized identifiers used to indicate the format and nature of data in internet protocols
// such as HTTP, email, and WebSockets. This package offers tools for parsing, validating, comparing,
// and manipulating MIME type strings according to RFC standards.
package mime

import (
	"fmt"
	"strings"

	"github.com/bits-and-blooms/bitset"

	"github.com/Tangerg/lynx/pkg/assert"
	"github.com/Tangerg/lynx/pkg/maps"
	pkgStrings "github.com/Tangerg/lynx/pkg/strings"
)

// tokenBitSet contains a bitset representing valid token characters in MIME specifications.
// MIME tokens are restricted to a specific set of ASCII characters according to RFC standards.
var tokenBitSet *bitset.BitSet

// init initializes the tokenBitSet with valid token characters according to MIME specifications.
// It marks valid characters by excluding control characters (0-31 and 127) and separator characters.
// This follows the MIME specification in RFC 2045, which defines the syntax for MIME headers.
func init() {
	// Initialize control characters bitset (0-31 and 127)
	controlChars := bitset.New(128)
	for i := uint(0); i <= 31; i++ {
		controlChars.Set(i)
	}
	controlChars.Set(127)

	// Initialize separator characters bitset
	separatorChars := bitset.New(128)
	separatorPositions := []uint{40, 41, 60, 62, 64, 44, 59, 58, 92, 34, 47, 91, 93, 63, 61, 123, 125, 32, 9}
	for _, position := range separatorPositions {
		separatorChars.Set(position)
	}

	// Create token bitset by excluding control and separator characters
	validTokenChars := bitset.New(128)
	for i := uint(0); i < 128; i++ {
		validTokenChars.Set(i)
	}

	validTokenChars = validTokenChars.Difference(controlChars)
	validTokenChars = validTokenChars.Difference(separatorChars)
	tokenBitSet = validTokenChars
}

// Builder is a utility for creating properly formatted MIME type instances.
// It provides a fluent API for constructing MIME types with validation at each step,
// ensuring that the resulting MIME type adheres to the standards defined in RFC 2045 and 2046.
type Builder struct {
	mime *MIME // The MIME type being constructed
}

// NewBuilder creates a new MIME type builder with default values.
// It initializes a builder with wildcard type and subtype, empty charset,
// and an empty parameters map, providing a starting point for building
// custom MIME types.
//
// Example:
// builder := NewBuilder() // Creates a builder for "*/*" MIME type
func NewBuilder() *Builder {
	return &Builder{
		mime: &MIME{
			_type:   wildcardType,
			subType: wildcardType,
			charset: "",
			params:  maps.NewHashMap[string, string](),
		},
	}
}

// checkToken validates that a token contains only characters allowed in MIME tokens.
// It returns an error if any character in the token is not permitted according to the MIME specification.
//
// Example:
// - "application" is a valid token
// - "content/type" is invalid (contains '/')
// - "image@png" is invalid (contains '@')
func (b *Builder) checkToken(token string) error {
	for _, char := range token {
		if !tokenBitSet.Test(uint(char)) {
			return fmt.Errorf("invalid character %s in token: %s", string(char), token)
		}
	}
	return nil
}

// checkParam validates both key and value of a MIME parameter.
// Parameter keys must always be valid tokens.
// Parameter values may be either tokens or quoted strings; quoted strings can contain
// characters that would otherwise be invalid in tokens.
//
// Example:
// - Key "charset" with value "UTF-8" is valid
// - Key "title" with value "\"My Document\"" is valid (quoted string)
// - Key "content@type" is invalid (key contains invalid character)
func (b *Builder) checkParam(paramKey string, paramValue string) error {
	// Validate parameter key (must be a valid token)
	if err := b.checkToken(paramKey); err != nil {
		return err
	}

	// Skip validation for quoted strings (they can contain any character)
	if pkgStrings.IsQuoted(paramValue) {
		return nil
	}

	// Validate parameter value if it's not quoted
	return b.checkToken(paramValue)
}

// checkParams validates all parameters in the MIME type.
// It iterates through all parameters, validating each key-value pair
// according to MIME parameter syntax rules.
func (b *Builder) checkParams() error {
	for paramKey, paramValue := range b.mime.params {
		if err := b.checkParam(paramKey, paramValue); err != nil {
			return err
		}
	}
	return nil
}

// WithType sets the primary type component of the MIME type.
// It normalizes the input by converting to lowercase and removing quotes if present.
//
// Examples:
// - WithType("text") sets the type to "text"
// - WithType("IMAGE") sets the type to "image" (normalized to lowercase)
// - WithType("\"application\"") sets the type to "application" (quotes removed)
func (b *Builder) WithType(mimeType string) *Builder {
	normalizedType := pkgStrings.UnQuote(strings.ToLower(mimeType))
	b.mime._type = normalizedType
	return b
}

// WithSubType sets the subtype component of the MIME type.
// Like WithType, it normalizes the input by converting to lowercase and removing quotes.
//
// Examples:
// - WithSubType("html") sets the subtype to "html"
// - WithSubType("JSON") sets the subtype to "json" (normalized to lowercase)
// - WithSubType("\"xml\"") sets the subtype to "xml" (quotes removed)
func (b *Builder) WithSubType(mimeSubType string) *Builder {
	normalizedSubType := pkgStrings.UnQuote(strings.ToLower(mimeSubType))
	b.mime.subType = normalizedSubType
	return b
}

// WithCharset sets the charset parameter of the MIME type.
// It normalizes the charset by converting to uppercase and storing it both in the
// dedicated charset field and in the parameters map.
//
// Examples:
// - WithCharset("utf-8") sets charset to "UTF-8"
// - WithCharset("\"ISO-8859-1\"") sets charset to "ISO-8859-1" (quotes removed)
func (b *Builder) WithCharset(charsetValue string) *Builder {
	normalizedCharset := pkgStrings.UnQuote(strings.ToUpper(charsetValue))
	if normalizedCharset == "" {
		return b
	}

	b.mime.charset = normalizedCharset
	b.mime.params.Put(paramCharset, normalizedCharset)
	return b
}

// WithParam adds a parameter to the MIME type.
// If the parameter is 'charset', it delegates to WithCharset instead.
// Otherwise, it normalizes the key to lowercase and adds the parameter to the MIME type.
//
// Examples:
// - WithParam("version", "1.0") adds the parameter "version=1.0"
// - WithParam("QUALITY", "high") adds the parameter "quality=high" (key normalized to lowercase)
// - WithParam("charset", "utf-8") delegates to WithCharset
func (b *Builder) WithParam(paramKey string, paramValue string) *Builder {
	normalizedKey := pkgStrings.UnQuote(strings.ToLower(paramKey))
	if normalizedKey == "" {
		return b
	}

	// Handle charset parameter specially
	if normalizedKey == paramCharset {
		return b.WithCharset(paramValue)
	}

	b.mime.params.Put(normalizedKey, paramValue)
	return b
}

// WithParams adds multiple parameters to the MIME type.
// It iterates through the provided map and calls WithParam for each key-value pair.
//
// Example:
// WithParams(map[string]string{
//
//	  "charset": "UTF-8",
//	  "version": "1.0",
//	  "q": "0.8",
//	})
func (b *Builder) WithParams(paramMap map[string]string) *Builder {
	for paramKey, paramValue := range paramMap {
		b.WithParam(paramKey, paramValue)
	}
	return b
}

// FromMime initializes the builder from an existing MIME instance.
// It performs a deep copy of all fields, ensuring that modifications to the builder
// do not affect the original MIME type.
//
// If the input MIME is nil, it returns the builder unchanged.
//
// Example:
// existingMime := &MIME{_type: "text", subType: "html", charset: "UTF-8", ...}
// builder.FromMime(existingMime) // Copies all properties from existingMime
func (b *Builder) FromMime(sourceMime *MIME) *Builder {
	if sourceMime == nil {
		return b
	}

	// Deep copy all fields from source MIME
	b.mime._type = sourceMime._type
	b.mime.subType = sourceMime.subType
	b.mime.charset = sourceMime.charset
	b.mime.params = sourceMime.params.Clone().(maps.HashMap[string, string])
	b.mime.cachedString = sourceMime.cachedString

	return b
}

// Build validates the MIME type and returns it if valid.
// It performs comprehensive validation of all components:
// - Checks that type and subtype contain only valid token characters
// - Validates the charset if present
// - Checks all parameters for validity
//
// If any validation fails, it returns nil and an appropriate error.
//
// Example usages:
// - NewBuilder().WithType("text").WithSubType("html").Build() returns a valid MIME
// - NewBuilder().WithType("text/html").Build() returns an error (invalid type)
func (b *Builder) Build() (*MIME, error) {
	// Validate and set default for type
	if b.mime._type == "" {
		b.mime._type = wildcardType
	} else {
		if err := b.checkToken(b.mime._type); err != nil {
			return nil, err
		}
	}

	// Validate and set default for subtype
	if b.mime.subType == "" {
		b.mime.subType = wildcardType
	} else {
		if err := b.checkToken(b.mime.subType); err != nil {
			return nil, err
		}
	}

	// Validate charset if present
	if b.mime.charset != "" {
		if err := b.checkToken(b.mime.charset); err != nil {
			return nil, err
		}
	}

	// Validate all parameters
	if err := b.checkParams(); err != nil {
		return nil, err
	}

	return b.mime, nil
}

// MustBuild panics if build err is not nil, otherwise returns a valid MIME
func (b *Builder) MustBuild() *MIME {
	return assert.Must(b.Build())
}

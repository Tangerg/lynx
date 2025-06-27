// Package mime provides comprehensive functionality for handling MIME (Multipurpose Internet Mail Extensions) types,
// which are standardized identifiers used to indicate the format and nature of data in internet protocols
// such as HTTP, email, and WebSockets. This package offers tools for parsing, validating, comparing,
// and manipulating MIME type strings according to RFC standards.
package mime

import (
	"fmt"
	"strings"

	"github.com/bits-and-blooms/bitset"

	"github.com/Tangerg/lynx/pkg/kv"
	pkgStrings "github.com/Tangerg/lynx/pkg/strings"
)

// tokenBitSet contains a bitset representing valid token characters in MIME specifications.
// MIME tokens are restricted to a specific set of ASCII characters according to RFC standards.
var tokenBitSet = bitset.New(128)

// init initializes the tokenBitSet with valid token characters according to MIME specifications.
// It marks valid characters by excluding control characters (0-31 and 127) and separator characters.
// This follows the MIME specification in RFC 2045, which defines the syntax for MIME headers.
func init() {
	ctl := bitset.New(128)
	for i := uint(0); i <= 31; i++ {
		ctl.Set(i)
	}
	ctl.Set(127)

	separators := bitset.New(128)
	separatorPositions := []uint{40, 41, 60, 62, 64, 44, 59, 58, 92, 34, 47, 91, 93, 63, 61, 123, 125, 32, 9}
	for _, pos := range separatorPositions {
		separators.Set(pos)
	}

	token := bitset.New(128)
	for i := uint(0); i < 128; i++ {
		token.Set(i)
	}

	token = token.Difference(ctl)
	token = token.Difference(separators)
	tokenBitSet = token
}

// Builder is a utility for creating properly formatted MIME type instances.
// It provides a fluent API for constructing MIME types with validation at each step,
// ensuring that the resulting MIME type adheres to the standards defined in RFC 2045 and 2046.
type Builder struct {
	mime *Mime // The MIME type being constructed
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
func (b *Builder) checkParam(k string, v string) error {
	err := b.checkToken(k)
	if err != nil {
		return err
	}
	if pkgStrings.IsQuoted(v) {
		return nil // Quoted strings can contain any character
	}
	return b.checkToken(v)
}

// checkParams validates all parameters in the MIME type.
// It iterates through all parameters, validating each key-value pair
// according to MIME parameter syntax rules.
func (b *Builder) checkParams() error {
	for k, v := range b.mime.params {
		err := b.checkParam(k, v)
		if err != nil {
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
func (b *Builder) WithType(typ string) *Builder {
	b.mime._type = pkgStrings.UnQuote(strings.ToLower(typ))
	return b
}

// WithSubType sets the subtype component of the MIME type.
// Like WithType, it normalizes the input by converting to lowercase and removing quotes.
//
// Examples:
// - WithSubType("html") sets the subtype to "html"
// - WithSubType("JSON") sets the subtype to "json" (normalized to lowercase)
// - WithSubType("\"xml\"") sets the subtype to "xml" (quotes removed)
func (b *Builder) WithSubType(subType string) *Builder {
	b.mime.subType = pkgStrings.UnQuote(strings.ToLower(subType))
	return b
}

// WithCharset sets the charset parameter of the MIME type.
// It normalizes the charset by converting to uppercase and storing it both in the
// dedicated charset field and in the parameters map.
//
// Examples:
// - WithCharset("utf-8") sets charset to "UTF-8"
// - WithCharset("\"ISO-8859-1\"") sets charset to "ISO-8859-1" (quotes removed)
func (b *Builder) WithCharset(charset string) *Builder {
	charset = pkgStrings.UnQuote(strings.ToUpper(charset))
	if charset == "" {
		return b
	}
	b.mime.charset = charset
	b.mime.params.Put(paramCharset, charset)
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
func (b *Builder) WithParam(key string, value string) *Builder {
	key = pkgStrings.UnQuote(strings.ToLower(key))
	if key == "" {
		return b
	}
	if key == paramCharset {
		return b.WithCharset(value)
	}
	b.mime.params.Put(key, value)
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
func (b *Builder) WithParams(params map[string]string) *Builder {
	for k, v := range params {
		b.WithParam(k, v)
	}
	return b
}

// FromMime initializes the builder from an existing Mime instance.
// It performs a deep copy of all fields, ensuring that modifications to the builder
// do not affect the original MIME type.
//
// If the input MIME is nil, it returns the builder unchanged.
//
// Example:
// existingMime := &Mime{_type: "text", subType: "html", charset: "UTF-8", ...}
// builder.FromMime(existingMime) // Copies all properties from existingMime
func (b *Builder) FromMime(mime *Mime) *Builder {
	if mime == nil {
		return b
	}
	b.mime._type = mime._type
	b.mime.subType = mime.subType
	b.mime.charset = mime.charset
	b.mime.params = mime.params.Clone()
	b.mime.stringValue = mime.stringValue
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
func (b *Builder) Build() (*Mime, error) {
	if b.mime._type == "" {
		b.mime._type = wildcardType
	} else {
		err := b.checkToken(b.mime._type)
		if err != nil {
			return nil, err
		}
	}

	if b.mime.subType == "" {
		b.mime.subType = wildcardType
	} else {
		err := b.checkToken(b.mime.subType)
		if err != nil {
			return nil, err
		}
	}

	if b.mime.charset != "" {
		err := b.checkToken(b.mime.charset)
		if err != nil {
			return nil, err
		}
	}

	err := b.checkParams()
	if err != nil {
		return nil, err
	}

	return b.mime, nil
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
		mime: &Mime{
			_type:   wildcardType,
			subType: wildcardType,
			charset: "",
			params:  kv.New[string, string](),
		},
	}
}

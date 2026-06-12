package mime

import (
	"fmt"
	"strings"

	"github.com/bits-and-blooms/bitset"

	"github.com/Tangerg/lynx/pkg/assert"
	"github.com/Tangerg/lynx/pkg/maps"
	pkgStrings "github.com/Tangerg/lynx/pkg/strings"
)

// tokenBitSet marks the ASCII characters allowed in a MIME token per
// RFC 2045 (control characters and tspecials are disallowed).
var tokenBitSet *bitset.BitSet

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

	validTokenChars := bitset.New(128)
	for i := uint(0); i < 128; i++ {
		validTokenChars.Set(i)
	}

	validTokenChars = validTokenChars.Difference(controlChars)
	validTokenChars = validTokenChars.Difference(separatorChars)
	tokenBitSet = validTokenChars
}

// Builder constructs a [MIME] using a fluent, validating API.
// A zero Builder is not usable; obtain one from [NewBuilder].
type Builder struct {
	mime *MIME
}

// NewBuilder returns a [Builder] initialized with wildcard type and
// subtype ("*/*") and no parameters.
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

// checkToken returns an error if token contains a non-token character.
func (b *Builder) checkToken(token string) error {
	for _, char := range token {
		if !tokenBitSet.Test(uint(char)) {
			return fmt.Errorf("invalid character %s in token: %s", string(char), token)
		}
	}
	return nil
}

// checkParam validates a parameter key/value pair. The value may be a
// quoted string or a token; the key must always be a token.
func (b *Builder) checkParam(paramKey string, paramValue string) error {
	if err := b.checkToken(paramKey); err != nil {
		return err
	}

	// Skip validation for quoted strings (they can contain any character)
	if pkgStrings.IsQuoted(paramValue) {
		return nil
	}

	return b.checkToken(paramValue)
}

// checkParams validates every parameter currently set on the builder.
func (b *Builder) checkParams() error {
	for paramKey, paramValue := range b.mime.params {
		if err := b.checkParam(paramKey, paramValue); err != nil {
			return err
		}
	}
	return nil
}

// WithType sets the primary type, lower-casing the input and stripping
// surrounding quotes.
func (b *Builder) WithType(mimeType string) *Builder {
	normalizedType := pkgStrings.UnQuote(strings.ToLower(mimeType))
	b.mime._type = normalizedType
	return b
}

// WithSubType sets the subtype, lower-casing the input and stripping
// surrounding quotes.
func (b *Builder) WithSubType(mimeSubType string) *Builder {
	normalizedSubType := pkgStrings.UnQuote(strings.ToLower(mimeSubType))
	b.mime.subType = normalizedSubType
	return b
}

// WithCharset sets the charset parameter, upper-casing the value and
// stripping surrounding quotes. An empty value is a no-op.
func (b *Builder) WithCharset(charsetValue string) *Builder {
	normalizedCharset := pkgStrings.UnQuote(strings.ToUpper(charsetValue))
	if normalizedCharset == "" {
		return b
	}

	b.mime.charset = normalizedCharset
	b.mime.params.Put(paramCharset, normalizedCharset)
	return b
}

// WithParam adds a parameter. The key is lower-cased and stripped of
// surrounding quotes; an empty key is a no-op. A "charset" key is
// forwarded to [Builder.WithCharset].
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

// WithParams adds every entry of paramMap via [Builder.WithParam].
func (b *Builder) WithParams(paramMap map[string]string) *Builder {
	for paramKey, paramValue := range paramMap {
		b.WithParam(paramKey, paramValue)
	}
	return b
}

// FromMime copies type, subtype, charset, and parameters from
// sourceMime into the builder. A nil source is a no-op.
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

// Build validates the configured components and returns the assembled
// [MIME]. Empty type or subtype default to "*". An invalid token in
// any component produces an error.
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

// MustBuild calls [Builder.Build] and panics on error.
func (b *Builder) MustBuild() *MIME {
	return assert.Must(b.Build())
}

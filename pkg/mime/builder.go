package mime

import (
	"fmt"
	"strings"

	"github.com/bits-and-blooms/bitset"

	"github.com/Tangerg/lynx/pkg/kv"
	pkgStrings "github.com/Tangerg/lynx/pkg/strings"
)

// tokenBitSet contains a bitset representing valid token characters in MIME specifications
var tokenBitSet = bitset.New(128)

// init initializes the tokenBitSet with valid token characters according to MIME specifications
// It excludes control characters (0-31 and 127) and separator characters
func init() {
	// Create a bitset for control characters
	ctl := bitset.New(128)
	for i := 0; i < 31; i++ {
		ctl.Set(uint(i))
	}
	ctl.Set(127)

	// Define separator characters as per MIME specification
	separatorChars := []rune{
		'(', ')', '<', '>', '@', ',', ';', ':', '\\', '"',
		'/', '[', ']', '?', '=', '{', '}', ' ', '\t',
	}
	separators := bitset.New(128)
	for _, char := range separatorChars {
		separators.Set(uint(char))
	}

	// Set all bits initially, then remove control chars and separators
	for i := uint(0); i < 128; i++ {
		tokenBitSet.Set(i)
	}
	tokenBitSet.InPlaceSymmetricDifference(ctl)
	tokenBitSet.InPlaceSymmetricDifference(separators)
}

// Builder is a utility for creating properly formatted MIME type instances
type Builder struct {
	mime *Mime
}

// checkToken validates that a token contains only characters allowed in MIME tokens
func (b *Builder) checkToken(token string) error {
	for _, char := range token {
		if !tokenBitSet.Test(uint(char)) {
			return fmt.Errorf("invalid character %s in token: %s", string(char), token)
		}
	}
	return nil
}

// checkParam validates both key and value of a MIME parameter
// Values may be quoted strings which bypass token character restrictions
func (b *Builder) checkParam(k string, v string) error {
	err := b.checkToken(k)
	if err != nil {
		return err
	}
	if pkgStrings.IsQuoted(v) {
		return nil
	}
	return b.checkToken(v)
}

// checkParams validates all parameters in the MIME type
func (b *Builder) checkParams() error {
	for k, v := range b.mime.params {
		err := b.checkParam(k, v)
		if err != nil {
			return err
		}
	}
	return nil
}

// WithType sets the primary type component of the MIME type
func (b *Builder) WithType(typ string) *Builder {
	b.mime._type = pkgStrings.UnQuote(strings.ToLower(typ))
	return b
}

// WithSubType sets the subtype component of the MIME type
func (b *Builder) WithSubType(subType string) *Builder {
	b.mime.subType = pkgStrings.UnQuote(strings.ToLower(subType))
	return b
}

// WithCharset sets the charset parameter of the MIME type
func (b *Builder) WithCharset(charset string) *Builder {
	charset = pkgStrings.UnQuote(strings.ToUpper(charset))
	b.mime.charset = charset
	b.mime.params.Put(paramCharset, charset)
	return b
}

// WithParam adds a parameter to the MIME type
// If the parameter is 'charset', it calls WithCharset instead
func (b *Builder) WithParam(key string, value string) *Builder {
	key = pkgStrings.UnQuote(strings.ToLower(key))
	if key == paramCharset {
		return b.WithCharset(value)
	}
	b.mime.params.Put(key, value)
	return b
}

// WithParams adds multiple parameters to the MIME type
func (b *Builder) WithParams(params map[string]string) *Builder {
	for k, v := range params {
		b.WithParam(k, v)
	}
	return b
}

// FromMime initializes the builder from an existing Mime instance
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

// Build validates the MIME type and returns it if valid
func (b *Builder) Build() (*Mime, error) {
	err := b.checkToken(b.mime._type)
	if err != nil {
		return nil, err
	}
	err = b.checkToken(b.mime.subType)
	if err != nil {
		return nil, err
	}
	if b.mime.charset != "" {
		err = b.checkToken(b.mime.charset)
		if err != nil {
			return nil, err
		}
	}
	err = b.checkParams()
	if err != nil {
		return nil, err
	}
	return b.mime, nil
}

// NewBuilder creates a new MIME type builder with default values
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

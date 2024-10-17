package mime

import (
	"fmt"
	"github.com/Tangerg/lynx/pkg/kv"
	pkgStrings "github.com/Tangerg/lynx/pkg/strings"
	"github.com/bits-and-blooms/bitset"
	"strings"
)

var tokenBitSet = bitset.New(128)

func init() {
	ctl := bitset.New(128)
	for i := 0; i < 31; i++ {
		ctl.Set(uint(i))
	}
	ctl.Set(127)
	separatorChars := []rune{
		'(', ')', '<', '>', '@', ',', ';', ':', '\\', '"',
		'/', '[', ']', '?', '=', '{', '}', ' ', '\t',
	}
	separators := bitset.New(128)
	for _, char := range separatorChars {
		separators.Set(uint(char))
	}
	for i := uint(0); i < 128; i++ {
		tokenBitSet.Set(i)
	}
	tokenBitSet.InPlaceSymmetricDifference(ctl)
	tokenBitSet.InPlaceSymmetricDifference(separators)
}

type Builder struct {
	mime *Mime
}

func (b *Builder) checkToken(token string) error {
	for _, char := range token {
		if !tokenBitSet.Test(uint(char)) {
			return fmt.Errorf("invalid character %s in token: %s", string(char), token)
		}
	}
	return nil
}

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

func (b *Builder) checkParams() error {
	for k, v := range b.mime.params {
		err := b.checkParam(k, v)
		if err != nil {
			return err
		}
	}
	return nil
}

func (b *Builder) WithType(typ string) *Builder {
	b.mime._type = pkgStrings.UnQuote(strings.ToLower(typ))
	return b
}
func (b *Builder) WithSubType(subType string) *Builder {
	b.mime.subType = pkgStrings.UnQuote(strings.ToLower(subType))
	return b
}
func (b *Builder) WithCharset(charset string) *Builder {
	charset = pkgStrings.UnQuote(strings.ToUpper(charset))
	b.mime.charset = charset
	b.mime.params.Put(paramCharset, charset)
	return b
}
func (b *Builder) WithParam(key string, value string) *Builder {
	key = pkgStrings.UnQuote(strings.ToLower(key))
	if key == paramCharset {
		return b.WithCharset(value)
	}
	b.mime.params.Put(key, value)
	return b
}
func (b *Builder) WithParams(params map[string]string) *Builder {
	for k, v := range params {
		b.WithParam(k, v)
	}
	return b
}
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

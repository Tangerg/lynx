package mime

import (
	"github.com/Tangerg/lynx/pkg/kv"
	pkgStrings "github.com/Tangerg/lynx/pkg/strings"
	"strings"
)

type Builder struct {
	mime *Mime
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
	err := b.mime.checkToken(b.mime._type)
	if err != nil {
		return nil, err
	}
	err = b.mime.checkToken(b.mime.subType)
	if err != nil {
		return nil, err
	}
	if b.mime.charset != "" {
		err = b.mime.checkToken(b.mime.charset)
		if err != nil {
			return nil, err
		}
	}
	err = b.mime.checkParams()
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

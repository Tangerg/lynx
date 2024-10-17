package mime

import (
	"errors"
	"github.com/Tangerg/lynx/pkg/kv"
	"github.com/gabriel-vasile/mimetype"
	"io"
	"strings"
)

var (
	ErrorInvalidMimeType = errors.New("invalid mime type")
)

func New(_type string, subType string) (*Mime, error) {
	return NewBuilder().
		WithType(_type).
		WithSubType(subType).
		Build()
}

func newMime(_type string, subType string) *Mime {
	mime, _ := NewBuilder().
		WithType(_type).
		WithSubType(subType).
		Build()
	return mime
}

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

func Detect(b []byte) (*Mime, error) {
	m := mimetype.Detect(b)
	return Parse(m.String())
}

func DetectReader(r io.Reader) (*Mime, error) {
	m, err := mimetype.DetectReader(r)
	if err != nil {
		return nil, err
	}
	return Parse(m.String())
}

func DetectFile(path string) (*Mime, error) {
	m, err := mimetype.DetectFile(path)
	if err != nil {
		return nil, err
	}
	return Parse(m.String())
}

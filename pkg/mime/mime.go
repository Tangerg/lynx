package mime

import "strings"

type Mime struct {
	_type       string
	subType     string
	charset     string
	params      map[string]string
	stringValue string
}

func (m *Mime) Type() string {
	return m._type
}
func (m *Mime) SubType() string {
	return m.subType
}
func (m *Mime) Charset() string {
	return m.charset
}
func (m *Mime) Param(key string) (string, bool) {
	val, ok := m.params[key]
	return val, ok
}
func (m *Mime) Params() map[string]string {
	return m.params
}
func (m *Mime) String() string {
	if m.stringValue == "" {
		m.formatStringValue()
	}
	return m.stringValue
}
func (m *Mime) formatStringValue() {
	sb := strings.Builder{}
	sb.WriteString(m._type)
	sb.WriteString("/")
	sb.WriteString(m.subType)
	for k, v := range m.params {
		sb.WriteString(";")
		sb.WriteString(k)
		sb.WriteString("=")
		sb.WriteString(v)
	}
	m.stringValue = sb.String()
}

type Builder struct {
	mime *Mime
}

func (b *Builder) WithType(typ string) *Builder {
	b.mime._type = typ
	return b
}
func (b *Builder) WithSubType(subType string) *Builder {
	b.mime.subType = subType
	return b
}
func (b *Builder) WithCharset(charset string) *Builder {
	b.mime.charset = charset
	return b
}
func (b *Builder) WithParam(key string, value string) *Builder {
	b.mime.params[key] = value
	return b
}
func (b *Builder) WithParams(params map[string]string) *Builder {
	for k, v := range params {
		b.mime.params[k] = v
	}
	return b
}
func (b *Builder) FromMime(mime *Mime) *Builder {
	b.mime._type = mime._type
	b.mime.subType = mime.subType
	b.mime.charset = mime.charset
	b.mime.params = mime.params
	b.mime.stringValue = mime.stringValue
	return b
}
func (b *Builder) Build() *Mime {
	return b.mime
}

func New() *Builder {
	return &Builder{
		mime: &Mime{
			params: make(map[string]string),
		},
	}
}

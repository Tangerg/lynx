package xml

import (
	"bytes"
	"strings"
)

var attrEscapeReplacer = strings.NewReplacer(
	"&", "&amp;",
	"<", "&lt;",
	">", "&gt;",
	`"`, "&quot;",
	"\n", "&#10;",
	"\r", "&#13;",
	"\t", "&#9;",
)

var textEscapeReplacer = strings.NewReplacer(
	"&", "&amp;",
	"<", "&lt;",
)

type Name struct {
	Local string
}

func (n Name) String() string {
	return n.Local
}

type Attr struct {
	Name  Name
	Value string
}

func (a Attr) String() string {
	return a.Name.String() + `="` + attrEscapeReplacer.Replace(a.Value) + `"`
}

type StartElement struct {
	Name  Name
	Attrs []Attr
}

func (e StartElement) String() string {
	sb := new(strings.Builder)
	sb.WriteString("<")
	sb.WriteString(e.Name.String())
	for _, attr := range e.Attrs {
		sb.WriteString(" ")
		sb.WriteString(attr.String())
	}
	sb.WriteString(">")
	return sb.String()
}

func (e StartElement) Copy() StartElement {
	attrs := make([]Attr, len(e.Attrs))
	copy(attrs, e.Attrs)
	e.Attrs = attrs
	return e
}

func (e StartElement) End() EndElement {
	return EndElement{e.Name}
}

type EndElement struct {
	Name Name
}

func (e EndElement) String() string {
	return "</" + e.Name.String() + ">"
}

func (e EndElement) Copy() EndElement {
	return EndElement{e.Name}
}

type Content interface {
	String() string
	content()
	copy() Content
}

type Element struct {
	Start    StartElement
	Contents []Content
	End      EndElement
}

func (e Element) Copy() Element {
	contents := make([]Content, len(e.Contents))
	for i, content := range e.Contents {
		contents[i] = content.copy()
	}
	return Element{
		e.Start.Copy(),
		contents,
		e.End.Copy(),
	}
}
func (e Element) content() {}
func (e Element) copy() Content {
	return e.Copy()
}
func (e Element) String() string {
	sb := new(strings.Builder)
	sb.WriteString(e.Start.String())
	for _, content := range e.Contents {
		sb.WriteString(content.String())
	}
	sb.WriteString(e.End.String())
	return sb.String()
}

type CharData []byte

func (c CharData) Copy() CharData { return CharData(bytes.Clone(c)) }
func (c CharData) content()       {}
func (c CharData) copy() Content {
	return c.Copy()
}
func (c CharData) String() string {
	return textEscapeReplacer.Replace(string(c))
}

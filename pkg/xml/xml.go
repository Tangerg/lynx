package xml

import (
	"bytes"
	"encoding/xml"
	"strings"
)

// Name represents an XML element name.
// It contains the local name of the element without namespace prefix.
type Name struct {
	Local string
}

// String returns the string representation of the name.
func (n Name) String() string {
	return n.Local
}

// Attr represents an XML attribute with a name and value.
type Attr struct {
	Name  Name
	Value string
}

// String returns the string representation of the attribute in the format: name="value".
// The value is XML-escaped to ensure valid XML output.
func (a Attr) String() string {
	sb := new(strings.Builder)
	xml.Escape(sb, []byte(a.Value))
	return a.Name.String() + `="` + sb.String() + `"`
}

// StartElement represents an XML start tag with its name and attributes.
// Example: <element attr1="value1" attr2="value2">
type StartElement struct {
	Name  Name
	Attrs []Attr
}

// String returns the string representation of the start element.
// It formats the element with all its attributes in valid XML syntax.
func (e StartElement) String() string {
	sb := new(strings.Builder)
	sb.WriteString("<")
	sb.WriteString(e.Name.String())

	// Append all attributes
	for _, attr := range e.Attrs {
		sb.WriteString(" ")
		sb.WriteString(attr.String())
	}

	sb.WriteString(">")
	return sb.String()
}

// Copy creates a deep copy of the StartElement.
// It duplicates the attributes slice to prevent shared references.
func (e StartElement) Copy() StartElement {
	attrs := make([]Attr, len(e.Attrs))
	copy(attrs, e.Attrs)
	e.Attrs = attrs
	return e
}

// End creates a corresponding EndElement for this StartElement.
func (e StartElement) End() EndElement {
	return EndElement{e.Name}
}

// EndElement represents an XML end tag.
// Example: </element>
type EndElement struct {
	Name Name
}

// String returns the string representation of the end element.
func (e EndElement) String() string {
	return "</" + e.Name.String() + ">"
}

// Copy creates a copy of the EndElement.
func (e EndElement) Copy() EndElement {
	return EndElement{e.Name}
}

// Content is an interface that represents any XML content.
// It can be either an Element or CharData.
type Content interface {
	String() string
	content()
	copy() Content
}

// Element represents a complete XML element with start tag, content, and end tag.
// Example: <element>content</element>
type Element struct {
	Start    StartElement
	Contents []Content
	End      EndElement
}

// Copy creates a deep copy of the Element.
// It recursively copies all nested contents.
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

// content implements the Content interface marker method.
func (e Element) content() {}

// copy implements the Content interface copy method.
func (e Element) copy() Content {
	return e.Copy()
}

// String returns the complete string representation of the element.
// It includes the start tag, all content, and the end tag.
func (e Element) String() string {
	sb := new(strings.Builder)
	sb.WriteString(e.Start.String())

	// Append all content
	for _, content := range e.Contents {
		sb.WriteString(content.String())
	}

	sb.WriteString(e.End.String())
	return sb.String()
}

// CharData represents character data (text content) in an XML element.
// It is stored as a byte slice and will be XML-escaped when converted to string.
type CharData []byte

// Copy creates a copy of the CharData.
func (c CharData) Copy() CharData {
	return CharData(bytes.Clone(c))
}

// content implements the Content interface marker method.
func (c CharData) content() {}

// copy implements the Content interface copy method.
func (c CharData) copy() Content {
	return c.Copy()
}

// String returns the XML-escaped string representation of the character data.
func (c CharData) String() string {
	sb := new(strings.Builder)
	xml.Escape(sb, c)
	return sb.String()
}

package markdown

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"

	"github.com/Tangerg/lynx/core/document"
	coremetadata "github.com/Tangerg/lynx/core/metadata"
)

// Metadata keys written onto emitted documents.
const (
	MetadataHeading      = "markdown.heading"
	MetadataHeadingLevel = "markdown.heading.level"
	MetadataHeadingPath  = "markdown.heading.path"
	MetadataSourceName   = "markdown.source"
)

// Config controls Markdown extraction. HeadingSplitLevel emits one document
// per section split on headings at or above that level (1 = H1, 2 = H1+H2).
// Zero disables splitting; non-zero values must be in [1, 6]. Metadata is
// cloned by NewReader, and reader-derived markdown.* keys take precedence.
type Config struct {
	HeadingSplitLevel int
	SourceName        string
	Metadata          coremetadata.Map
}

// Reader extracts documents from Markdown.
type Reader struct {
	reader            io.Reader
	parser            goldmark.Markdown
	headingSplitLevel int
	sourceName        string
	extraMetadata     coremetadata.Map
}

// NewReader builds a markdown reader over src.
func NewReader(src io.Reader, config Config) (*Reader, error) {
	if isNil(src) {
		return nil, errors.New("markdown reader: source must not be nil")
	}
	r := &Reader{
		reader:            src,
		parser:            goldmark.New(),
		headingSplitLevel: config.HeadingSplitLevel,
		sourceName:        config.SourceName,
		extraMetadata:     config.Metadata.Clone(),
	}
	if err := r.extraMetadata.Validate(); err != nil {
		return nil, fmt.Errorf("markdown reader: invalid metadata: %w", err)
	}
	if r.headingSplitLevel < 0 || r.headingSplitLevel > 6 {
		return nil, fmt.Errorf("markdown reader: heading split level %d is outside [1, 6]", r.headingSplitLevel)
	}
	return r, nil
}

func isNil(value any) bool {
	reflected := reflect.ValueOf(value)
	if !reflected.IsValid() {
		return true
	}
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}

// Read consumes the underlying reader and emits documents according to
// the configured mode. ctx cancellation is honored before parsing and
// between emitted sections.
func (r *Reader) Read(ctx context.Context) ([]*document.Document, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	raw, err := io.ReadAll(r.reader)
	if err != nil {
		return nil, fmt.Errorf("markdown: read source: %w", err)
	}

	if r.headingSplitLevel == 0 {
		return r.readWhole(raw)
	}
	return r.readSplit(ctx, raw)
}

// readWhole returns one document containing the entire markdown body.
// Blank input yields no documents — the same contract as the html and
// pdf readers, not an error.
func (r *Reader) readWhole(raw []byte) ([]*document.Document, error) {
	if strings.TrimSpace(string(raw)) == "" {
		return nil, nil
	}
	doc, err := document.NewDocument(string(raw), nil)
	if err != nil {
		return nil, fmt.Errorf("markdown: build document: %w", err)
	}
	doc.Metadata, err = r.baseMetadata()
	if err != nil {
		return nil, fmt.Errorf("markdown: encode metadata: %w", err)
	}
	return []*document.Document{doc}, nil
}

// readSplit walks the markdown AST and emits a document per section.
func (r *Reader) readSplit(ctx context.Context, raw []byte) ([]*document.Document, error) {
	return r.sectionsToDocuments(ctx, r.collectSections(raw))
}

// collectSections walks the top-level AST nodes and groups them into
// sections: a new section opens at every heading of level
// <= headingSplitLevel, while the heading-path stack tracks ancestry.
// Body content before the first heading lands in an unnamed lead-in
// section.
func (r *Reader) collectSections(raw []byte) []*section {
	root := r.parser.Parser().Parse(text.NewReader(raw))

	var (
		sections []*section
		stack    []sectionRef
	)

	for n := root.FirstChild(); n != nil; n = n.NextSibling() {
		heading, ok := n.(*ast.Heading)
		if !ok || heading.Level > r.headingSplitLevel {
			// Body content — attach to the most recent open section, or
			// create an unnamed lead-in section if none exists yet.
			if len(sections) == 0 {
				sections = append(sections, &section{})
			}
			sections[len(sections)-1].appendNodeSource(raw, n)
			continue
		}

		// New split-level heading: open a new section, manage the path stack.
		title := extractHeadingText(heading, raw)
		for len(stack) > 0 && stack[len(stack)-1].level >= heading.Level {
			stack = stack[:len(stack)-1]
		}
		stack = append(stack, sectionRef{level: heading.Level, title: title})

		sec := &section{
			heading: title,
			level:   heading.Level,
			path:    pathFromStack(stack),
		}
		sec.appendNodeSource(raw, n)
		sections = append(sections, sec)
	}
	return sections
}

// sectionsToDocuments materializes each non-empty section into a
// [document.Document], stamping heading metadata when present. ctx
// cancellation is honored between sections.
func (r *Reader) sectionsToDocuments(ctx context.Context, sections []*section) ([]*document.Document, error) {
	var docs []*document.Document
	for _, sec := range sections {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		body := strings.TrimSpace(sec.builder.String())
		if body == "" {
			continue
		}
		md, err := r.baseMetadata()
		if err != nil {
			return nil, fmt.Errorf("markdown: encode section metadata: %w", err)
		}
		if sec.heading != "" {
			if err := md.Set(MetadataHeading, sec.heading); err != nil {
				return nil, fmt.Errorf("markdown: encode section heading: %w", err)
			}
			if err := md.Set(MetadataHeadingLevel, sec.level); err != nil {
				return nil, fmt.Errorf("markdown: encode section heading level: %w", err)
			}
			if err := md.Set(MetadataHeadingPath, sec.path); err != nil {
				return nil, fmt.Errorf("markdown: encode section heading path: %w", err)
			}
		}
		doc, err := document.NewDocument(body, nil)
		if err != nil {
			return nil, fmt.Errorf("markdown: build section document: %w", err)
		}
		doc.Metadata = md
		docs = append(docs, doc)
	}
	return docs, nil
}

// extractHeadingText recovers the plain-text content of a Heading node
// by walking its inline children and concatenating *ast.Text values.
// Avoids the deprecated Heading.Text() API.
func extractHeadingText(h *ast.Heading, raw []byte) string {
	var b strings.Builder
	for c := h.FirstChild(); c != nil; c = c.NextSibling() {
		if t, ok := c.(*ast.Text); ok {
			b.Write(t.Segment.Value(raw))
		}
	}
	return b.String()
}

func (r *Reader) baseMetadata() (coremetadata.Map, error) {
	md := r.extraMetadata.Clone()
	if r.sourceName != "" {
		if err := md.Set(MetadataSourceName, r.sourceName); err != nil {
			return nil, err
		}
	}
	return md, nil
}

// section is the accumulated body of a single emitted document.
type section struct {
	heading string
	level   int
	path    string
	builder strings.Builder
}

// sectionRef is a single frame on the heading-path stack.
type sectionRef struct {
	level int
	title string
}

func pathFromStack(stack []sectionRef) string {
	titles := make([]string, len(stack))
	for i, ref := range stack {
		titles[i] = ref.title
	}
	return strings.Join(titles, " > ")
}

// appendNodeSource copies the raw markdown bytes backing n into this
// section's body. goldmark preserves byte offsets via Segments, which we
// can stitch together.
func (s *section) appendNodeSource(raw []byte, n ast.Node) {
	var buf bytes.Buffer
	collectSegments(&buf, raw, n)
	if buf.Len() == 0 {
		return
	}
	if s.builder.Len() > 0 {
		s.builder.WriteString("\n\n")
	}
	s.builder.Write(buf.Bytes())
}

// collectSegments walks the node and concatenates the raw bytes from
// every leaf text segment. This recovers the original markdown source
// for the subtree (close enough for embeddings — exact whitespace may
// drift).
func collectSegments(buf *bytes.Buffer, raw []byte, n ast.Node) {
	if n == nil {
		return
	}
	if n.Type() == ast.TypeBlock {
		lines := n.Lines()
		for i := range lines.Len() {
			seg := lines.At(i)
			buf.Write(seg.Value(raw))
		}
	}
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		collectSegments(buf, raw, c)
	}
}

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

const (
	MetadataHeading      = "markdown.heading"
	MetadataHeadingLevel = "markdown.heading.level"
	MetadataHeadingPath  = "markdown.heading.path"
	MetadataSourceName   = "markdown.source"
)

// Config controls Markdown extraction. HeadingSplitLevel emits one document
// per section split on headings at or above that level (1 = H1, 2 = H1+H2).
// Zero disables splitting; non-zero values must be in [1, 6]. Metadata is
// cloned by New, and reader-derived markdown.* keys take precedence.
type Config struct {
	HeadingSplitLevel int
	SourceName        string
	Metadata          coremetadata.Map
}

// Reader extracts documents from Markdown.
type Reader struct {
	source            io.Reader
	parser            goldmark.Markdown
	headingSplitLevel int
	sourceName        string
	extraMetadata     coremetadata.Map
}

// New builds a Markdown reader over source.
func New(source io.Reader, config Config) (*Reader, error) {
	if isNil(source) {
		return nil, errors.New("markdown reader: source must not be nil")
	}
	r := &Reader{
		source:            source,
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

// Read consumes the underlying reader and emits documents according to the
// configuration. Context cancellation is honored around parsing and between
// emitted sections.
func (r *Reader) Read(ctx context.Context) ([]*document.Document, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	raw, err := io.ReadAll(r.source)
	if err != nil {
		return nil, fmt.Errorf("markdown: read source: %w", err)
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return nil, ctxErr
	}

	if r.headingSplitLevel == 0 {
		return r.readWhole(raw)
	}
	sections, err := r.collectSections(ctx, raw)
	if err != nil {
		return nil, err
	}
	return r.sectionsToDocuments(ctx, sections)
}

// readWhole returns one document containing the entire markdown body.
// Blank input yields no documents — the same contract as the html and
// pdf readers, not an error.
func (r *Reader) readWhole(raw []byte) ([]*document.Document, error) {
	body := string(raw)
	if strings.TrimSpace(body) == "" {
		return nil, nil
	}
	doc, err := document.NewDocument(body, nil)
	if err != nil {
		return nil, fmt.Errorf("markdown: build document: %w", err)
	}
	doc.Metadata, err = r.baseMetadata()
	if err != nil {
		return nil, fmt.Errorf("markdown: encode metadata: %w", err)
	}
	return []*document.Document{doc}, nil
}

// collectSections walks the top-level AST nodes and groups them into
// sections: a new section opens at every heading of level
// <= headingSplitLevel, while the heading-path stack tracks ancestry.
// Body content before the first heading lands in an unnamed lead-in
// section.
func (r *Reader) collectSections(ctx context.Context, raw []byte) ([]*section, error) {
	root := r.parser.Parser().Parse(text.NewReader(raw))
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	var (
		sections []*section
		path     headingPath
	)

	for n := root.FirstChild(); n != nil; n = n.NextSibling() {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
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
		title := r.headingText(heading, raw)
		path.push(heading.Level, title)

		sec := &section{
			heading: title,
			level:   heading.Level,
			path:    path.String(),
		}
		sec.appendNodeSource(raw, n)
		sections = append(sections, sec)
	}
	return sections, nil
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
		md, err := r.baseMetadata()
		if err != nil {
			return nil, fmt.Errorf("markdown: encode section metadata: %w", err)
		}
		doc, err := sec.document(md)
		if err != nil {
			return nil, fmt.Errorf("markdown: materialize section: %w", err)
		}
		if doc == nil {
			continue
		}
		docs = append(docs, doc)
	}
	return docs, nil
}

// headingText avoids Goldmark's deprecated Heading.Text API.
func (*Reader) headingText(h *ast.Heading, raw []byte) string {
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

type sectionRef struct {
	level int
	title string
}

type headingPath []sectionRef

func (h *headingPath) push(level int, title string) {
	for len(*h) > 0 && (*h)[len(*h)-1].level >= level {
		*h = (*h)[:len(*h)-1]
	}
	*h = append(*h, sectionRef{level: level, title: title})
}

func (h headingPath) String() string {
	titles := make([]string, len(h))
	for i, ref := range h {
		titles[i] = ref.title
	}
	return strings.Join(titles, " > ")
}

func (s *section) document(metadata coremetadata.Map) (*document.Document, error) {
	body := strings.TrimSpace(s.builder.String())
	if body == "" {
		return nil, nil
	}
	if s.heading != "" {
		if err := metadata.Set(MetadataHeading, s.heading); err != nil {
			return nil, err
		}
		if err := metadata.Set(MetadataHeadingLevel, s.level); err != nil {
			return nil, err
		}
		if err := metadata.Set(MetadataHeadingPath, s.path); err != nil {
			return nil, err
		}
	}
	doc, err := document.NewDocument(body, nil)
	if err != nil {
		return nil, err
	}
	doc.Metadata = metadata
	return doc, nil
}

// appendNodeSource copies the raw markdown bytes backing n into this
// section's body. goldmark preserves byte offsets via Segments, which we
// can stitch together.
func (s *section) appendNodeSource(raw []byte, n ast.Node) {
	var buf bytes.Buffer
	s.collectSegments(&buf, raw, n)
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
func (s *section) collectSegments(buf *bytes.Buffer, raw []byte, n ast.Node) {
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
		s.collectSegments(buf, raw, c)
	}
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

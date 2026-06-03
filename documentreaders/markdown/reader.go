package markdown

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"

	"github.com/Tangerg/lynx/core/document"
)

// Metadata keys written onto emitted documents.
const (
	MetadataHeading      = "markdown.heading"
	MetadataHeadingLevel = "markdown.heading.level"
	MetadataHeadingPath  = "markdown.heading.path"
	MetadataSourceName   = "markdown.source"
)

// Option configures a [Reader].
type Option func(*Reader)

// WithHeadingSplit makes the reader emit one document per section,
// splitting on headings of level <= maxLevel (e.g. 2 = split on H1+H2,
// 1 = split on H1 only). maxLevel must be in [1, 6]; outside that
// range falls back to no-split.
func WithHeadingSplit(maxLevel int) Option {
	return func(r *Reader) {
		if maxLevel < 1 || maxLevel > 6 {
			r.headingSplitLevel = 0
			return
		}
		r.headingSplitLevel = maxLevel
	}
}

// WithSourceName stamps every emitted document with the given
// `markdown.source` metadata entry — useful when the underlying io.Reader
// doesn't carry path information.
func WithSourceName(name string) Option {
	return func(r *Reader) { r.sourceName = name }
}

// WithMetadata adds caller-supplied metadata to every emitted document
// (source URI, tenant, doc type, ...). The map is cloned, so later
// caller mutations don't leak in. Reader-derived `markdown.*` keys take
// precedence on conflict.
func WithMetadata(md map[string]any) Option {
	return func(r *Reader) {
		if len(md) > 0 {
			r.extraMetadata = maps.Clone(md)
		}
	}
}

var _ document.Reader = (*Reader)(nil)

// Reader is a markdown-aware [document.Reader].
type Reader struct {
	reader            io.Reader
	parser            goldmark.Markdown
	headingSplitLevel int
	sourceName        string
	extraMetadata     map[string]any
}

// NewReader builds a markdown reader over src.
func NewReader(src io.Reader, opts ...Option) (*Reader, error) {
	if src == nil {
		return nil, errors.New("markdown: NewReader: src must not be nil")
	}
	r := &Reader{
		reader: src,
		parser: goldmark.New(),
	}
	for _, opt := range opts {
		opt(r)
	}
	return r, nil
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
func (r *Reader) readWhole(raw []byte) ([]*document.Document, error) {
	doc, err := document.NewDocument(string(raw), nil)
	if err != nil {
		return nil, fmt.Errorf("markdown: build document: %w", err)
	}
	if md := r.baseMetadata(); len(md) > 0 {
		doc.Metadata = md
	}
	return []*document.Document{doc}, nil
}

// readSplit walks the markdown AST and emits a document per section.
func (r *Reader) readSplit(ctx context.Context, raw []byte) ([]*document.Document, error) {
	root := r.parser.Parser().Parse(text.NewReader(raw))

	var (
		docs     []*document.Document
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

	for _, sec := range sections {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		body := strings.TrimSpace(sec.builder.String())
		if body == "" {
			continue
		}
		md := r.baseMetadata()
		if sec.heading != "" {
			md[MetadataHeading] = sec.heading
			md[MetadataHeadingLevel] = sec.level
			md[MetadataHeadingPath] = sec.path
		}
		doc, err := document.NewDocument(body, nil)
		if err != nil {
			return nil, fmt.Errorf("markdown: build section document: %w", err)
		}
		if len(md) > 0 {
			doc.Metadata = md
		}
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

func (r *Reader) baseMetadata() map[string]any {
	md := maps.Clone(r.extraMetadata)
	if md == nil {
		md = map[string]any{}
	}
	if r.sourceName != "" {
		md[MetadataSourceName] = r.sourceName
	}
	return md
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
		for i := 0; i < lines.Len(); i++ {
			seg := lines.At(i)
			buf.Write(seg.Value(raw))
		}
	}
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		collectSegments(buf, raw, c)
	}
}

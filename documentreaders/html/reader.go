package html

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/PuerkitoBio/goquery"

	"github.com/Tangerg/lynx/core/document"
)

// Metadata keys written onto emitted documents.
const (
	MetadataTitle       = "html.title"
	MetadataDescription = "html.description"
	MetadataCanonical   = "html.canonical"
	MetadataSelector    = "html.selector"
	MetadataSourceName  = "html.source"
)

// Option configures a [Reader].
type Option func(*Reader)

// WithSelector makes the reader emit one document per element matched
// by the CSS selector (e.g. "article", "div.post"). Standard goquery
// selector syntax applies.
func WithSelector(selector string) Option {
	return func(r *Reader) { r.selector = selector }
}

// WithSourceName stamps every document with the supplied source name.
func WithSourceName(name string) Option {
	return func(r *Reader) { r.sourceName = name }
}

// WithStripWhitespace controls whether consecutive whitespace runs in
// the extracted text are collapsed to a single space. Default true.
func WithStripWhitespace(strip bool) Option {
	return func(r *Reader) { r.stripWhitespace = strip }
}

var _ document.Reader = (*Reader)(nil)

// Reader is an HTML-aware [document.Reader].
type Reader struct {
	reader          io.Reader
	selector        string
	sourceName      string
	stripWhitespace bool
}

// NewReader builds an HTML reader over src.
func NewReader(src io.Reader, opts ...Option) (*Reader, error) {
	if src == nil {
		return nil, errors.New("html: NewReader: src must not be nil")
	}
	r := &Reader{reader: src, stripWhitespace: true}
	for _, opt := range opts {
		opt(r)
	}
	return r, nil
}

// Read parses the source and emits documents according to the
// configured mode.
func (r *Reader) Read(_ context.Context) ([]*document.Document, error) {
	doc, err := goquery.NewDocumentFromReader(r.reader)
	if err != nil {
		return nil, fmt.Errorf("html: parse: %w", err)
	}

	page := pageMetadata(doc)

	if r.selector == "" {
		return r.readWhole(doc, page)
	}
	return r.readSelector(doc, page)
}

func (r *Reader) readWhole(doc *goquery.Document, page pageInfo) ([]*document.Document, error) {
	body := r.extractText(doc.Selection)
	if body == "" {
		return nil, nil
	}
	d, err := document.NewDocument(body, nil)
	if err != nil {
		return nil, fmt.Errorf("html: build document: %w", err)
	}
	d.Metadata = r.buildMetadata(page, "")
	return []*document.Document{d}, nil
}

func (r *Reader) readSelector(doc *goquery.Document, page pageInfo) ([]*document.Document, error) {
	var (
		docs []*document.Document
		errs []error
	)
	doc.Find(r.selector).EachWithBreak(func(_ int, sel *goquery.Selection) bool {
		body := r.extractText(sel)
		if body == "" {
			return true
		}
		d, err := document.NewDocument(body, nil)
		if err != nil {
			errs = append(errs, err)
			return false
		}
		d.Metadata = r.buildMetadata(page, r.selector)
		docs = append(docs, d)
		return true
	})
	if len(errs) > 0 {
		return nil, fmt.Errorf("html: build selector document: %w", errs[0])
	}
	return docs, nil
}

func (r *Reader) buildMetadata(page pageInfo, selector string) map[string]any {
	md := map[string]any{}
	if page.title != "" {
		md[MetadataTitle] = page.title
	}
	if page.description != "" {
		md[MetadataDescription] = page.description
	}
	if page.canonical != "" {
		md[MetadataCanonical] = page.canonical
	}
	if r.sourceName != "" {
		md[MetadataSourceName] = r.sourceName
	}
	if selector != "" {
		md[MetadataSelector] = selector
	}
	if len(md) == 0 {
		return nil
	}
	return md
}

func (r *Reader) extractText(sel *goquery.Selection) string {
	// Drop script / style / noscript / template content so we don't pick
	// up code or hidden text in embeddings.
	clone := sel.Clone()
	clone.Find("script, style, noscript, template, head").Remove()
	text := clone.Text()
	if r.stripWhitespace {
		text = collapseWhitespace(text)
	}
	return strings.TrimSpace(text)
}

type pageInfo struct {
	title       string
	description string
	canonical   string
}

func pageMetadata(doc *goquery.Document) pageInfo {
	var p pageInfo
	p.title = strings.TrimSpace(doc.Find("head > title").First().Text())

	doc.Find(`head > meta[name="description"]`).Each(func(_ int, s *goquery.Selection) {
		if c, ok := s.Attr("content"); ok && p.description == "" {
			p.description = strings.TrimSpace(c)
		}
	})
	doc.Find(`head > link[rel="canonical"]`).Each(func(_ int, s *goquery.Selection) {
		if href, ok := s.Attr("href"); ok && p.canonical == "" {
			p.canonical = strings.TrimSpace(href)
		}
	})
	return p
}

// collapseWhitespace replaces runs of whitespace (space, tab, newline)
// with a single space — keeps the text embedding-friendly without
// preserving HTML formatting.
func collapseWhitespace(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	prevSpace := false
	for _, r := range s {
		switch r {
		case ' ', '\t', '\n', '\r', '\v', '\f':
			if !prevSpace {
				b.WriteByte(' ')
				prevSpace = true
			}
		default:
			b.WriteRune(r)
			prevSpace = false
		}
	}
	return b.String()
}

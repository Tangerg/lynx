package html

import (
	"context"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/andybalholm/cascadia"

	"github.com/Tangerg/lynx/core/document"
	coremetadata "github.com/Tangerg/lynx/core/metadata"
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

// WithMetadata adds validated caller-supplied metadata to every emitted
// document. The map is deep-cloned, and reader-derived html.* keys take
// precedence on conflict.
func WithMetadata(md coremetadata.Map) Option {
	return func(r *Reader) {
		r.extraMetadata = md.Clone()
	}
}

// Reader extracts documents from HTML.
type Reader struct {
	reader          io.Reader
	selector        string
	matcher         goquery.Matcher
	sourceName      string
	stripWhitespace bool
	extraMetadata   coremetadata.Map
}

// NewReader builds an HTML reader over src.
func NewReader(src io.Reader, opts ...Option) (*Reader, error) {
	if isNil(src) {
		return nil, errors.New("html reader: source must not be nil")
	}
	r := &Reader{reader: src, stripWhitespace: true}
	for index, opt := range opts {
		if opt == nil {
			return nil, fmt.Errorf("html reader: option %d is nil", index)
		}
		opt(r)
	}
	if err := r.extraMetadata.Validate(); err != nil {
		return nil, fmt.Errorf("html reader: invalid metadata: %w", err)
	}
	if r.selector != "" {
		matcher, err := cascadia.Compile(r.selector)
		if err != nil {
			return nil, fmt.Errorf("html reader: invalid selector %q: %w", r.selector, err)
		}
		r.matcher = matcher
	}
	return r, nil
}

// Read parses the source and emits documents according to the
// configured mode. ctx cancellation is honored before parsing and
// between matched elements.
func (r *Reader) Read(ctx context.Context) ([]*document.Document, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	doc, err := goquery.NewDocumentFromReader(r.reader)
	if err != nil {
		return nil, fmt.Errorf("html: parse: %w", err)
	}

	page := pageMetadata(doc)

	if r.selector == "" {
		return r.readWhole(doc, page)
	}
	return r.readSelector(ctx, doc, page)
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
	d.Metadata, err = r.buildMetadata(page, "")
	if err != nil {
		return nil, fmt.Errorf("html: encode metadata: %w", err)
	}
	return []*document.Document{d}, nil
}

func (r *Reader) readSelector(ctx context.Context, doc *goquery.Document, page pageInfo) ([]*document.Document, error) {
	var (
		docs     []*document.Document
		buildErr error
	)
	doc.FindMatcher(r.matcher).EachWithBreak(func(_ int, sel *goquery.Selection) bool {
		if ctx.Err() != nil {
			return false // cancellation is reported after the loop
		}
		body := r.extractText(sel)
		if body == "" {
			return true
		}
		d, err := document.NewDocument(body, nil)
		if err != nil {
			buildErr = err
			return false
		}
		d.Metadata, err = r.buildMetadata(page, r.selector)
		if err != nil {
			buildErr = fmt.Errorf("encode metadata: %w", err)
			return false
		}
		docs = append(docs, d)
		return true
	})
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if buildErr != nil {
		return nil, fmt.Errorf("html: build selector document: %w", buildErr)
	}
	return docs, nil
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

func (r *Reader) buildMetadata(page pageInfo, selector string) (coremetadata.Map, error) {
	md := r.extraMetadata.Clone()
	if page.title != "" {
		if err := md.Set(MetadataTitle, page.title); err != nil {
			return nil, err
		}
	}
	if page.description != "" {
		if err := md.Set(MetadataDescription, page.description); err != nil {
			return nil, err
		}
	}
	if page.canonical != "" {
		if err := md.Set(MetadataCanonical, page.canonical); err != nil {
			return nil, err
		}
	}
	if r.sourceName != "" {
		if err := md.Set(MetadataSourceName, r.sourceName); err != nil {
			return nil, err
		}
	}
	if selector != "" {
		if err := md.Set(MetadataSelector, selector); err != nil {
			return nil, err
		}
	}
	if len(md) == 0 {
		return nil, nil
	}
	return md, nil
}

func (r *Reader) extractText(sel *goquery.Selection) string {
	// Drop script / style / noscript / template content so code and
	// hidden text don't end up in embeddings.
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

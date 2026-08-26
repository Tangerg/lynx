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

const (
	MetadataTitle       = "html.title"
	MetadataDescription = "html.description"
	MetadataCanonical   = "html.canonical"
	MetadataSelector    = "html.selector"
	MetadataSourceName  = "html.source"
)

// Config controls HTML extraction. By default whitespace runs are collapsed;
// PreserveWhitespace retains the source spacing instead. Metadata is cloned
// by New, and reader-derived html.* keys take precedence on conflict.
type Config struct {
	Selector           string
	SourceName         string
	PreserveWhitespace bool
	Metadata           coremetadata.Map
}

// Reader extracts documents from HTML.
type Reader struct {
	source          io.Reader
	selector        string
	matcher         goquery.Matcher
	sourceName      string
	stripWhitespace bool
	extraMetadata   coremetadata.Map
}

// New builds an HTML reader over source.
func New(source io.Reader, config Config) (*Reader, error) {
	if isNil(source) {
		return nil, errors.New("html reader: source must not be nil")
	}
	r := &Reader{
		source:          source,
		selector:        config.Selector,
		sourceName:      config.SourceName,
		stripWhitespace: !config.PreserveWhitespace,
		extraMetadata:   config.Metadata.Clone(),
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

// Read parses the source and emits documents according to the configuration.
// Context cancellation is honored around parsing and between matches.
func (r *Reader) Read(ctx context.Context) ([]*document.Document, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	doc, err := goquery.NewDocumentFromReader(r.source)
	if err != nil {
		return nil, fmt.Errorf("html: parse: %w", err)
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}
	page := newPageInfo(doc)

	if r.selector == "" {
		return r.readWhole(ctx, doc, page)
	}
	return r.readSelector(ctx, doc, page)
}

func (r *Reader) readWhole(ctx context.Context, doc *goquery.Document, page pageInfo) ([]*document.Document, error) {
	body := r.extractText(doc.Selection)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if body == "" {
		return nil, nil
	}
	d, err := r.buildDocument(body, page, "")
	if err != nil {
		return nil, fmt.Errorf("html: build document: %w", err)
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
		d, err := r.buildDocument(body, page, r.selector)
		if err != nil {
			buildErr = err
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

func (r *Reader) buildDocument(body string, page pageInfo, selector string) (*document.Document, error) {
	doc, err := document.NewDocument(body, nil)
	if err != nil {
		return nil, err
	}
	doc.Metadata, err = r.buildMetadata(page, selector)
	if err != nil {
		return nil, fmt.Errorf("encode metadata: %w", err)
	}
	return doc, nil
}

func (r *Reader) buildMetadata(page pageInfo, selector string) (coremetadata.Map, error) {
	md := r.extraMetadata.Clone()
	if err := page.applyTo(&md); err != nil {
		return nil, err
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
		text = strings.Join(strings.Fields(text), " ")
	}
	return strings.TrimSpace(text)
}

type pageInfo struct {
	title       string
	description string
	canonical   string
}

func newPageInfo(doc *goquery.Document) pageInfo {
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

func (p pageInfo) applyTo(metadata *coremetadata.Map) error {
	for _, field := range [...]struct {
		key   string
		value string
	}{
		{MetadataTitle, p.title},
		{MetadataDescription, p.description},
		{MetadataCanonical, p.canonical},
	} {
		if field.value != "" {
			if err := metadata.Set(field.key, field.value); err != nil {
				return err
			}
		}
	}
	return nil
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

// Package pdf implements a [document.Reader] over PDF payloads using
// github.com/ledongthuc/pdf — a pure-Go PDF parser forked from rsc/pdf.
//
// The reader extracts plain text from each page. Two emission modes:
//
//   - Whole-document mode (default): one [*document.Document] holding
//     the concatenated text of every page.
//   - Per-page mode (opt in via [WithPerPage]): one document per page,
//     with `pdf.page` (1-indexed) and `pdf.pages.total` metadata stamped.
//
// Limitations: text-only extraction; tables, columns and exotic font
// encodings may yield imperfect order. PDFs that embed text as images
// (scanned documents) need OCR upstream.
//
// Example:
//
//	r, _ := pdf.NewReader(file, fileSize, pdf.WithPerPage())
//	docs, _ := r.Read(ctx)
package pdf

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	ledongthuc "github.com/ledongthuc/pdf"

	"github.com/Tangerg/lynx/core/document"
)

// Metadata keys written onto emitted documents.
const (
	MetadataPageIndex  = "pdf.page"
	MetadataPagesTotal = "pdf.pages.total"
	MetadataSourceName = "pdf.source"
)

// Option configures a [Reader].
type Option func(*Reader)

// WithPerPage switches to per-page emission — one document per PDF page.
func WithPerPage() Option {
	return func(r *Reader) { r.perPage = true }
}

// WithSourceName stamps every emitted document with the given source
// name (file path, URL, ...).
func WithSourceName(name string) Option {
	return func(r *Reader) { r.sourceName = name }
}

// WithPassword supplies a password used to decrypt the PDF if it is
// encrypted. The empty string means "no password" (default).
func WithPassword(pw string) Option {
	return func(r *Reader) { r.password = pw }
}

var _ document.Reader = (*Reader)(nil)

// Reader is a PDF-aware [document.Reader].
type Reader struct {
	src        io.ReaderAt
	size       int64
	perPage    bool
	sourceName string
	password   string
}

// NewReader builds a PDF reader. The underlying source must implement
// io.ReaderAt because pdfcpu parses PDF objects via random access.
// size is the total byte length of the PDF — pass file.Size() (from
// os.File.Stat) or len(buf) for in-memory data.
func NewReader(src io.ReaderAt, size int64, opts ...Option) (*Reader, error) {
	if src == nil {
		return nil, errors.New("pdf: NewReader: src must not be nil")
	}
	if size <= 0 {
		return nil, errors.New("pdf: NewReader: size must be > 0")
	}
	r := &Reader{src: src, size: size}
	for _, opt := range opts {
		opt(r)
	}
	return r, nil
}

// Read parses the source and emits documents according to the
// configured mode.
func (r *Reader) Read(_ context.Context) ([]*document.Document, error) {
	pdfReader, err := r.openReader()
	if err != nil {
		return nil, err
	}

	total := pdfReader.NumPage()
	if r.perPage {
		return r.readPages(pdfReader, total)
	}
	return r.readWhole(pdfReader, total)
}

func (r *Reader) openReader() (*ledongthuc.Reader, error) {
	if r.password != "" {
		pdfReader, err := ledongthuc.NewReaderEncrypted(r.src, r.size, func() string { return r.password })
		if err != nil {
			return nil, fmt.Errorf("pdf: open encrypted: %w", err)
		}
		return pdfReader, nil
	}
	pdfReader, err := ledongthuc.NewReader(r.src, r.size)
	if err != nil {
		return nil, fmt.Errorf("pdf: open: %w", err)
	}
	return pdfReader, nil
}

func (r *Reader) readWhole(pdfReader *ledongthuc.Reader, total int) ([]*document.Document, error) {
	body, err := readAllText(pdfReader)
	if err != nil {
		return nil, err
	}
	body = strings.TrimSpace(body)
	if body == "" {
		return nil, nil
	}
	doc, err := document.NewDocument(body, nil)
	if err != nil {
		return nil, fmt.Errorf("pdf: build document: %w", err)
	}
	doc.Metadata = r.baseMetadata(total)
	return []*document.Document{doc}, nil
}

func (r *Reader) readPages(pdfReader *ledongthuc.Reader, total int) ([]*document.Document, error) {
	docs := make([]*document.Document, 0, total)
	for i := 1; i <= total; i++ {
		page := pdfReader.Page(i)
		if page.V.IsNull() {
			continue
		}
		text, err := page.GetPlainText(nil)
		if err != nil {
			return nil, fmt.Errorf("pdf: page %d: %w", i, err)
		}
		text = strings.TrimSpace(text)
		if text == "" {
			continue
		}
		doc, err := document.NewDocument(text, nil)
		if err != nil {
			return nil, fmt.Errorf("pdf: page %d build: %w", i, err)
		}
		md := r.baseMetadata(total)
		md[MetadataPageIndex] = i
		doc.Metadata = md
		docs = append(docs, doc)
	}
	return docs, nil
}

func (r *Reader) baseMetadata(total int) map[string]any {
	md := map[string]any{
		MetadataPagesTotal: total,
	}
	if r.sourceName != "" {
		md[MetadataSourceName] = r.sourceName
	}
	return md
}

// readAllText streams every page through GetPlainText and concatenates
// the result. Using the per-page API instead of Reader.GetPlainText so
// we can recover from a single bad page without aborting the whole
// document.
func readAllText(pdfReader *ledongthuc.Reader) (string, error) {
	var b strings.Builder
	total := pdfReader.NumPage()
	for i := 1; i <= total; i++ {
		page := pdfReader.Page(i)
		if page.V.IsNull() {
			continue
		}
		text, err := page.GetPlainText(nil)
		if err != nil {
			return "", fmt.Errorf("pdf: page %d: %w", i, err)
		}
		if text == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString(text)
	}
	return b.String(), nil
}

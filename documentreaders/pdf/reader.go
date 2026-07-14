package pdf

import (
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"strings"

	ledongthuc "github.com/ledongthuc/pdf"

	"github.com/Tangerg/lynx/core/document"
	coremetadata "github.com/Tangerg/lynx/core/metadata"
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

// WithMetadata adds caller-supplied metadata to every emitted document
// (source URI, tenant, doc type, ...). The map is cloned, so later
// caller mutations don't leak in. Reader-derived `pdf.*` keys take
// precedence on conflict.
func WithMetadata(md map[string]any) Option {
	return func(r *Reader) {
		if len(md) > 0 {
			r.extraMetadata = maps.Clone(md)
		}
	}
}

var _ document.Reader = (*Reader)(nil)

// Reader is a PDF-aware [document.Reader].
type Reader struct {
	src           io.ReaderAt
	size          int64
	perPage       bool
	sourceName    string
	password      string
	extraMetadata map[string]any
}

// NewReader builds a PDF reader. The underlying source must implement
// io.ReaderAt because ledongthuc/pdf parses PDF objects via random access.
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
// configured mode. ctx cancellation is honored between pages.
//
// Pages that fail to parse are skipped (their errors are joined into
// the returned error only when NO page yielded text); a document-level
// parse failure returns an error. Both guard against the upstream
// library's panic-on-malformed-input style — see [pageText].
func (r *Reader) Read(ctx context.Context) (docs []*document.Document, err error) {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return nil, ctxErr
	}
	// ledongthuc/pdf (following rsc/pdf) reports malformed input by
	// panicking deep in the parser, and only its GetPlainText path
	// recovers internally. Convert document-level panics (trailer /
	// xref parsing in NewReader, NumPage) into errors at the module
	// boundary so a corrupt PDF can't crash the caller.
	defer func() {
		if rec := recover(); rec != nil {
			docs, err = nil, fmt.Errorf("pdf: malformed document: %v", rec)
		}
	}()
	pdfReader, err := r.openReader()
	if err != nil {
		return nil, err
	}

	total := pdfReader.NumPage()
	if r.perPage {
		return r.readPages(ctx, pdfReader, total)
	}
	return r.readWhole(ctx, pdfReader, total)
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

func (r *Reader) readWhole(ctx context.Context, pdfReader *ledongthuc.Reader, total int) ([]*document.Document, error) {
	body, err := readAllText(ctx, pdfReader, total)
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
	doc.Metadata, err = coremetadata.FromValues(r.baseMetadata(total))
	if err != nil {
		return nil, fmt.Errorf("pdf: encode metadata: %w", err)
	}
	return []*document.Document{doc}, nil
}

func (r *Reader) readPages(ctx context.Context, pdfReader *ledongthuc.Reader, total int) ([]*document.Document, error) {
	docs := make([]*document.Document, 0, total)
	// fonts caches parsed font charmaps across pages — GetPlainText
	// rebuilds every font per call when handed nil.
	fonts := make(map[string]*ledongthuc.Font)
	var pageErrs []error
	for i := 1; i <= total; i++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		text, err := pageText(pdfReader, i, fonts)
		if err != nil {
			// A bad page shouldn't abort the readable rest of the
			// document; its error surfaces only when nothing parsed.
			pageErrs = append(pageErrs, err)
			continue
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
		doc.Metadata, err = coremetadata.FromValues(md)
		if err != nil {
			return nil, fmt.Errorf("pdf: page %d metadata: %w", i, err)
		}
		docs = append(docs, doc)
	}
	if len(docs) == 0 && len(pageErrs) > 0 {
		return nil, fmt.Errorf("pdf: no readable pages: %w", errors.Join(pageErrs...))
	}
	return docs, nil
}

func (r *Reader) baseMetadata(total int) map[string]any {
	md := maps.Clone(r.extraMetadata)
	if md == nil {
		md = map[string]any{}
	}
	md[MetadataPagesTotal] = total
	if r.sourceName != "" {
		md[MetadataSourceName] = r.sourceName
	}
	return md
}

// readAllText streams every page through [pageText] and concatenates
// the result. Using the per-page API instead of Reader.GetPlainText so
// a single bad page is skipped without aborting the whole document;
// page errors surface only when no page yielded text. ctx cancellation
// is honored between pages.
func readAllText(ctx context.Context, pdfReader *ledongthuc.Reader, total int) (string, error) {
	var b strings.Builder
	fonts := make(map[string]*ledongthuc.Font)
	var pageErrs []error
	for i := 1; i <= total; i++ {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		text, err := pageText(pdfReader, i, fonts)
		if err != nil {
			pageErrs = append(pageErrs, err)
			continue
		}
		if text == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString(text)
	}
	if b.Len() == 0 && len(pageErrs) > 0 {
		return "", fmt.Errorf("pdf: no readable pages: %w", errors.Join(pageErrs...))
	}
	return b.String(), nil
}

// pageText extracts one page's plain text. The upstream parser panics
// on malformed page content (its panic-as-error style only recovers
// inside GetPlainText itself, not in Page / object resolution), so the
// recover here converts a bad page into an error the caller can skip.
// fonts is the cross-page font cache GetPlainText fills as it goes.
func pageText(pdfReader *ledongthuc.Reader, i int, fonts map[string]*ledongthuc.Font) (text string, err error) {
	defer func() {
		if rec := recover(); rec != nil {
			text, err = "", fmt.Errorf("page %d: malformed page: %v", i, rec)
		}
	}()
	page := pdfReader.Page(i)
	if page.V.IsNull() {
		return "", nil
	}
	text, err = page.GetPlainText(fonts)
	if err != nil {
		return "", fmt.Errorf("page %d: %w", i, err)
	}
	return text, nil
}

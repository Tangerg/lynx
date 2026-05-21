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

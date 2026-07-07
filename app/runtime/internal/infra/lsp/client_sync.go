package lsp

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
)

// ensureOpen makes the server aware of abs's current on-disk content: a
// didOpen the first time, a didChange when the content has changed since we
// last synced (the agent edits files out-of-band). It returns the document
// version now in effect, which a diagnostics wait uses to recognize fresh
// pushes. A no-op (content unchanged) returns the existing version.
func (c *client) ensureOpen(ctx context.Context, abs string) (int, error) {
	text, err := os.ReadFile(abs)
	if err != nil {
		return 0, fmt.Errorf("lsp: read %s: %w", abs, err)
	}
	uri := pathToURI(abs)
	hash := sha256.Sum256(text)

	// Hold c.mu across the Notify so the version bump and its didOpen/didChange
	// are atomic PER DOCUMENT. Two concurrent ensureOpen on the same file — the
	// `lsp` and `lsp_diagnostics` tools share one parallel segment
	// (ConcurrencyKey=true) and hit this shared client — would otherwise compute
	// v1 and v2 under the lock, release, then race the Notify: the server could
	// see didChange(v2) before didOpen(v1), or versions out of order, and desync
	// its in-memory document for the rest of the session. Notify is a buffered,
	// non-blocking write whose completion doesn't depend on the inbound
	// diagnostics handler (a separate goroutine), so holding the lock across it
	// can't deadlock. The map is updated only AFTER a successful send, so a failed
	// Notify doesn't record a version the server never saw.
	c.mu.Lock()
	defer c.mu.Unlock()
	prev, isOpen := c.open[uri]
	if isOpen && prev.hash == hash {
		return prev.version, nil
	}
	version := prev.version + 1
	if !isOpen {
		err = c.conn.Notify(ctx, "textDocument/didOpen", didOpenParams{
			TextDocument: textDocumentItem{URI: uri, LanguageID: c.spec.LanguageID, Version: version, Text: string(text)},
		})
	} else {
		err = c.conn.Notify(ctx, "textDocument/didChange", didChangeParams{
			TextDocument:   versionedTextDocumentIdentifier{URI: uri, Version: version},
			ContentChanges: []contentChange{{Text: string(text)}},
		})
	}
	if err != nil {
		return 0, fmt.Errorf("lsp: sync %s: %w", abs, err)
	}
	c.open[uri] = openDoc{version: version, hash: hash}
	return version, nil
}

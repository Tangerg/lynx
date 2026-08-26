package lsp

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
)

const maxDocumentBytes int64 = 8 << 20

// ErrDocumentTooLarge reports a workspace document that cannot be admitted to
// the in-memory language-server synchronization boundary.
var ErrDocumentTooLarge = errors.New("lsp: document exceeds the 8 MiB limit")

// ensureOpen makes the server aware of abs's current on-disk content: a
// didOpen the first time, a didChange when the content has changed since we
// last synced (the agent edits files out-of-band). It returns the document
// version now in effect, which a diagnostics wait uses to recognize fresh
// pushes. A no-op (content unchanged) returns the existing version.
func (c *client) ensureOpen(ctx context.Context, abs string) (int, error) {
	text, err := readDocument(ctx, abs)
	if err != nil {
		return 0, fmt.Errorf("lsp: read %s: %w", abs, err)
	}
	uri := pathToURI(abs)
	hash := sha256.Sum256(text)

	// Hold c.mu across the Notify so the version bump and its didOpen/didChange
	// are atomic PER DOCUMENT. Two concurrent ensureOpen on the same file — calls
	// to the `lsp` operation tool share one parallel segment
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

func readDocument(ctx context.Context, path string) (_ []byte, err error) {
	if cause := context.Cause(ctx); cause != nil {
		return nil, cause
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() {
		err = errors.Join(err, file.Close())
	}()

	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if info.Size() > maxDocumentBytes {
		return nil, fmt.Errorf("%w: %d bytes", ErrDocumentTooLarge, info.Size())
	}
	content, err := io.ReadAll(io.LimitReader(contextReader{ctx: ctx, reader: file}, maxDocumentBytes+1))
	if err != nil {
		return nil, err
	}
	if len(content) > int(maxDocumentBytes) {
		return nil, fmt.Errorf("%w: file grew while reading", ErrDocumentTooLarge)
	}
	return content, nil
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (c contextReader) Read(buffer []byte) (int, error) {
	if cause := context.Cause(c.ctx); cause != nil {
		return 0, cause
	}
	read, err := c.reader.Read(buffer)
	if cause := context.Cause(c.ctx); cause != nil {
		return read, cause
	}
	return read, err
}

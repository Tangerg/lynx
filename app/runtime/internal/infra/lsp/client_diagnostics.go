package lsp

import (
	"context"
	"time"
)

// diagnostics syncs abs and returns the server's diagnostics for it. Servers
// push diagnostics asynchronously after a change, so we wait up to settle for
// a push at least as fresh as the version we just synced before returning the
// cache (which may already hold a fresh-enough set, ending the wait at once).
func (c *client) diagnostics(ctx context.Context, abs string, settle time.Duration) ([]Diagnostic, error) {
	version, err := c.ensureOpen(ctx, abs)
	if err != nil {
		return nil, err
	}
	uri := pathToURI(abs)
	// Nudge servers that only publish on save (typescript-language-server) —
	// harmless for those that publish on change (gopls).
	_ = c.conn.Notify(ctx, "textDocument/didSave", didSaveParams{TextDocument: textDocumentIdentifier{URI: uri}})
	deadline := time.NewTimer(settle)
	defer deadline.Stop()
	for {
		c.mu.Lock()
		ds, ok := c.diags[uri]
		wait := c.updated
		c.mu.Unlock()
		if ok && ds.version >= version {
			return ds.diagnostics, nil
		}
		select {
		case <-wait: // a push arrived — re-check
		case <-deadline.C:
			c.mu.Lock()
			ds := c.diags[uri]
			c.mu.Unlock()
			return ds.diagnostics, nil // best effort: whatever we have
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

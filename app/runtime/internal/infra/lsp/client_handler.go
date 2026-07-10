package lsp

import (
	"context"
	"encoding/json"
	"slices"

	"github.com/sourcegraph/jsonrpc2"
)

// Handle implements jsonrpc2.Handler — the server→client direction. We cache
// diagnostics pushes and answer the handful of requests gopls makes during
// startup; everything else is acknowledged (requests) or ignored
// (notifications) so the server is never left blocking on us.
func (c *client) Handle(ctx context.Context, conn *jsonrpc2.Conn, req *jsonrpc2.Request) {
	switch req.Method {
	case "textDocument/publishDiagnostics":
		var p publishDiagnosticsParams
		if req.Params != nil {
			_ = json.Unmarshal(*req.Params, &p)
		}
		c.storeDiagnostics(p)
	case "workspace/configuration":
		// Reply one null per requested item — we expose no settings, and gopls
		// treats null as "use defaults".
		n := 1
		if req.Params != nil {
			var cp struct {
				Items []json.RawMessage `json:"items"`
			}
			if json.Unmarshal(*req.Params, &cp) == nil {
				n = len(cp.Items)
			}
		}
		_ = conn.Reply(ctx, req.ID, make([]*struct{}, n))
	default:
		// Acknowledge any other server request (registerCapability,
		// workDoneProgress/create, …) with null so it isn't left waiting;
		// ignore unknown notifications.
		if !req.Notif {
			_ = conn.Reply(ctx, req.ID, nil)
		}
	}
}

func (c *client) storeDiagnostics(p publishDiagnosticsParams) {
	c.mu.Lock()
	c.diags[p.URI] = diagSet{version: p.Version, diagnostics: slices.Clone(p.Diagnostics)}
	close(c.updated)
	c.updated = make(chan struct{})
	c.mu.Unlock()
}

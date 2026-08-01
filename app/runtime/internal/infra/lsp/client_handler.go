package lsp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
		params, err := decodePublishDiagnostics(req.Params)
		if err != nil {
			c.storeDiagnosticsError(err)
			return
		}
		c.storeDiagnostics(params)
	case "workspace/configuration":
		// Reply one null per requested item — we expose no settings, and gopls
		// treats null as "use defaults".
		itemCount, err := decodeConfigurationItemCount(req.Params)
		if err != nil {
			replyInvalidParams(ctx, conn, req, err)
			return
		}
		_ = conn.Reply(ctx, req.ID, make([]*struct{}, itemCount))
	default:
		// Acknowledge any other server request (registerCapability,
		// workDoneProgress/create, …) with null so it isn't left waiting;
		// ignore unknown notifications.
		if !req.Notif {
			_ = conn.Reply(ctx, req.ID, nil)
		}
	}
}

func decodePublishDiagnostics(raw *json.RawMessage) (publishDiagnosticsParams, error) {
	if raw == nil {
		return publishDiagnosticsParams{}, errors.New("publishDiagnostics params are missing")
	}
	var params publishDiagnosticsParams
	if err := json.Unmarshal(*raw, &params); err != nil {
		return publishDiagnosticsParams{}, fmt.Errorf("decode publishDiagnostics params: %w", err)
	}
	if err := params.validate(); err != nil {
		return publishDiagnosticsParams{}, fmt.Errorf("validate publishDiagnostics params: %w", err)
	}
	return params, nil
}

func decodeConfigurationItemCount(raw *json.RawMessage) (int, error) {
	if raw == nil {
		return 0, errors.New("configuration params are missing")
	}
	var params struct {
		Items *[]*configurationItem `json:"items"`
	}
	if err := json.Unmarshal(*raw, &params); err != nil {
		return 0, fmt.Errorf("decode configuration params: %w", err)
	}
	if params.Items == nil {
		return 0, errors.New("configuration items are missing or null")
	}
	for index, item := range *params.Items {
		if item == nil {
			return 0, fmt.Errorf("configuration item %d is null", index)
		}
	}
	return len(*params.Items), nil
}

func replyInvalidParams(ctx context.Context, conn *jsonrpc2.Conn, req *jsonrpc2.Request, err error) {
	if req.Notif {
		return
	}
	_ = conn.ReplyWithError(ctx, req.ID, &jsonrpc2.Error{
		Code:    jsonrpc2.CodeInvalidParams,
		Message: err.Error(),
	})
}

func (c *client) storeDiagnostics(p publishDiagnosticsParams) {
	c.mu.Lock()
	c.diags[p.URI] = diagSet{version: p.Version, diagnostics: slices.Clone(p.Diagnostics)}
	c.diagnosticsErr = nil
	c.signalDiagnosticsUpdateLocked()
	c.mu.Unlock()
}

func (c *client) storeDiagnosticsError(err error) {
	c.mu.Lock()
	c.diagnosticsErr = err
	c.signalDiagnosticsUpdateLocked()
	c.mu.Unlock()
}

func (c *client) signalDiagnosticsUpdateLocked() {
	close(c.updated)
	c.updated = make(chan struct{})
}

package lsp

import (
	"context"
	"encoding/json"
	"fmt"
)

func (c *client) definition(ctx context.Context, abs string, pos Position) ([]Location, error) {
	return c.positionLocations(ctx, abs, pos, "textDocument/definition", "definition")
}

func (c *client) references(ctx context.Context, abs string, pos Position) ([]Location, error) {
	if _, err := c.ensureOpen(ctx, abs); err != nil {
		return nil, err
	}
	var raw json.RawMessage
	if err := c.conn.Call(ctx, "textDocument/references", referenceParams{
		TextDocument: textDocumentIdentifier{URI: pathToURI(abs)},
		Position:     pos,
		Context:      referenceContext{IncludeDeclaration: true},
	}, &raw); err != nil {
		return nil, fmt.Errorf("lsp: references: %w", err)
	}
	return parseLocations(raw), nil
}

func (c *client) implementation(ctx context.Context, abs string, pos Position) ([]Location, error) {
	return c.positionLocations(ctx, abs, pos, "textDocument/implementation", "implementation")
}

// positionLocations executes an LSP position request whose response is a
// definition-shaped location payload. Definition and implementation differ
// only in the protocol method and diagnostic operation name.
func (c *client) positionLocations(ctx context.Context, abs string, pos Position, method, operation string) ([]Location, error) {
	if _, err := c.ensureOpen(ctx, abs); err != nil {
		return nil, err
	}
	var raw json.RawMessage
	if err := c.conn.Call(ctx, method, positionParams{
		TextDocument: textDocumentIdentifier{URI: pathToURI(abs)},
		Position:     pos,
	}, &raw); err != nil {
		return nil, fmt.Errorf("lsp: %s: %w", operation, err)
	}
	return parseLocations(raw), nil
}

// callHierarchy resolves the symbol at pos to a call-hierarchy item, then
// queries its callers (incoming) or callees (outgoing) in one shot — the LSP
// two-step (prepareCallHierarchy → incoming/outgoingCalls) is hidden from the
// caller. The callers/callees come back as symbols; empty when pos isn't a
// callable symbol (prepare returns nothing).
func (c *client) callHierarchy(ctx context.Context, abs string, pos Position, outgoing bool) ([]Symbol, error) {
	if _, err := c.ensureOpen(ctx, abs); err != nil {
		return nil, err
	}
	var items []callHierarchyItem
	if err := c.conn.Call(ctx, "textDocument/prepareCallHierarchy", positionParams{
		TextDocument: textDocumentIdentifier{URI: pathToURI(abs)},
		Position:     pos,
	}, &items); err != nil {
		return nil, fmt.Errorf("lsp: prepareCallHierarchy: %w", err)
	}
	if len(items) == 0 {
		return nil, nil
	}
	params := callHierarchyItemParams{Item: items[0]}
	if outgoing {
		var calls []callHierarchyOutgoingCall
		if err := c.conn.Call(ctx, "callHierarchy/outgoingCalls", params, &calls); err != nil {
			return nil, fmt.Errorf("lsp: outgoingCalls: %w", err)
		}
		out := make([]Symbol, 0, len(calls))
		for _, call := range calls {
			out = append(out, call.To.symbol())
		}
		return out, nil
	}
	var calls []callHierarchyIncomingCall
	if err := c.conn.Call(ctx, "callHierarchy/incomingCalls", params, &calls); err != nil {
		return nil, fmt.Errorf("lsp: incomingCalls: %w", err)
	}
	out := make([]Symbol, 0, len(calls))
	for _, call := range calls {
		out = append(out, call.From.symbol())
	}
	return out, nil
}

func (c *client) hover(ctx context.Context, abs string, pos Position) (string, error) {
	if _, err := c.ensureOpen(ctx, abs); err != nil {
		return "", err
	}
	var h struct {
		Contents json.RawMessage `json:"contents"`
	}
	if err := c.conn.Call(ctx, "textDocument/hover", positionParams{
		TextDocument: textDocumentIdentifier{URI: pathToURI(abs)},
		Position:     pos,
	}, &h); err != nil {
		return "", fmt.Errorf("lsp: hover: %w", err)
	}
	return hoverText(h.Contents), nil
}

func (c *client) documentSymbols(ctx context.Context, abs string) ([]Symbol, error) {
	if _, err := c.ensureOpen(ctx, abs); err != nil {
		return nil, err
	}
	uri := pathToURI(abs)
	var raw json.RawMessage
	if err := c.conn.Call(ctx, "textDocument/documentSymbol", documentSymbolParams{
		TextDocument: textDocumentIdentifier{URI: uri},
	}, &raw); err != nil {
		return nil, fmt.Errorf("lsp: documentSymbol: %w", err)
	}
	return parseSymbols(raw, uri), nil
}

func (c *client) workspaceSymbols(ctx context.Context, query string) ([]Symbol, error) {
	var infos []symbolInformation
	if err := c.conn.Call(ctx, "workspace/symbol", workspaceSymbolParams{Query: query}, &infos); err != nil {
		return nil, fmt.Errorf("lsp: workspace/symbol: %w", err)
	}
	out := make([]Symbol, 0, len(infos))
	for _, s := range infos {
		out = append(out, Symbol{Name: s.Name, Kind: s.Kind, Location: s.Location, Container: s.ContainerName})
	}
	return out, nil
}

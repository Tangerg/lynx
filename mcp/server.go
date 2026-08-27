package mcp

import (
	"cmp"
	"context"
	"fmt"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	corechat "github.com/Tangerg/scope/core/chat"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

// Register installs every [tool.Tool] in tools onto server using
// the low-level [(*sdkmcp.Server).AddTool] API.
//
// Registration is all-or-nothing: definitions are snapshotted, duplicate names
// within the batch are rejected, and every tool is built before any is added.
// A bad entry mid-list therefore never leaves the server half-registered, and
// handlers use the same identity the server advertised even when a Tool
// implementation is mutable.
//
// The generic sdkmcp.AddTool[In, Out] form is deliberately avoided:
// tools already supply a hand-authored JSON schema, and the
// generic API would otherwise reflect over a Go In type and overwrite
// it.
func Register(server *sdkmcp.Server, tools ...toolcontract.Tool) error {
	if server == nil {
		return ErrNilServer
	}

	registry, err := toolcontract.NewRegistry(tools...)
	if err != nil {
		return fmt.Errorf("mcp: register tools: %w", err)
	}

	prepared := make([]serverTool, 0, len(tools))
	for _, definition := range registry.Definitions() {
		executable, _ := registry.Resolve(definition.Name)
		prepared = append(prepared, serverTool{executable: executable, definition: definition})
	}
	for _, tool := range prepared {
		server.AddTool(tool.descriptor(), tool.handle)
	}
	return nil
}

type serverTool struct {
	executable toolcontract.Tool
	definition corechat.ToolDefinition
}

func (s serverTool) descriptor() *sdkmcp.Tool {
	return new(sdkmcp.Tool{
		Name:        s.definition.Name,
		Description: s.definition.Description,
		InputSchema: s.definition.InputSchema,
	})
}

// handle routes a tools/call RPC into a [tool.Tool]. Errors
// from the tool surface via [sdkmcp.CallToolResult.IsError] plus
// a [*sdkmcp.TextContent] body — never as a Go error from the handler
// — because the latter would be promoted to a JSON-RPC protocol error
// and hide the failure from the LLM's view.
//
// The MCP server session is stamped onto the context so tool authors
// can use the reverse-capability helpers ([ReportProgress] and [Elicit])
// without taking a direct dependency on the SDK session.
func (s serverTool) handle(ctx context.Context, req *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
	toolName := s.definition.Name
	ctx, span := mcpTracer.Start(ctx, "mcp.tool.serve "+toolName,
		trace.WithSpanKind(trace.SpanKindServer),
		trace.WithAttributes(attribute.String(attrToolName, toolName)),
	)
	defer span.End()

	ctx = withServerCall(ctx, req)

	var rawArgs string
	if req != nil && req.Params != nil {
		rawArgs = string(req.Params.Arguments)
	}

	out, err := s.executable.Call(ctx, cmp.Or(rawArgs, "{}"))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return &sdkmcp.CallToolResult{
			Content: []sdkmcp.Content{&sdkmcp.TextContent{Text: err.Error()}},
			IsError: true,
		}, nil
	}
	return &sdkmcp.CallToolResult{
		Content: []sdkmcp.Content{&sdkmcp.TextContent{Text: out}},
	}, nil
}

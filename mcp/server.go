package mcp

import (
	"cmp"
	"context"
	"fmt"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	corechat "github.com/Tangerg/lynx/core/chat"
	toolcontract "github.com/Tangerg/lynx/tools"
)

// Register installs every [tools.Tool] in tools onto server using
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
		return fmt.Errorf("mcp.Register: %w", err)
	}

	prepared := make([]preparedTool, 0, len(tools))
	for _, definition := range registry.Definitions() {
		tool, _ := registry.Resolve(definition.Name)
		pt, err := prepareOne(tool, definition)
		if err != nil {
			return err
		}
		prepared = append(prepared, pt)
	}
	for _, pt := range prepared {
		server.AddTool(pt.tool, pt.handler)
	}
	return nil
}

// preparedTool is one validated, ready-to-add registration — the unit
// Register builds in its first pass.
type preparedTool struct {
	tool    *sdkmcp.Tool
	handler sdkmcp.ToolHandler
}

func prepareOne(tool toolcontract.Tool, definition corechat.ToolDefinition) (preparedTool, error) {
	schema, err := schemaToAny(definition.InputSchema)
	if err != nil {
		return preparedTool{}, fmt.Errorf("mcp.Register: tool %q input schema: %w", definition.Name, err)
	}

	return preparedTool{
		tool: &sdkmcp.Tool{
			Name:        definition.Name,
			Description: definition.Description,
			InputSchema: schema,
		},
		handler: serverHandler(tool, definition.Name),
	}, nil
}

// serverHandler routes a tools/call RPC into a [tools.Tool]. Errors
// from the tool surface via [sdkmcp.CallToolResult.IsError] plus
// a [*sdkmcp.TextContent] body — never as a Go error from the handler
// — because the latter would be promoted to a JSON-RPC protocol error
// and hide the failure from the LLM's view.
//
// The MCP server session is stamped onto the context so tool authors
// can use the reverse-capability helpers ([ReportProgress],
// [ElicitFromClient], [LogToClient]) without taking a direct
// dependency on the SDK.
func serverHandler(tool toolcontract.Tool, toolName string) sdkmcp.ToolHandler {
	return func(ctx context.Context, req *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
		ctx, span := mcpTracer.Start(ctx, "mcp.tool.serve "+toolName,
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(attribute.String(attrToolName, toolName)),
		)
		defer span.End()

		// The SDK doesn't guarantee a non-nil request / params — guard like
		// withProgressToken does rather than dereferencing raw.
		ctx = withToolCall(ctx, req)

		var rawArgs string
		if req != nil && req.Params != nil {
			rawArgs = string(req.Params.Arguments)
		}

		args := cmp.Or(rawArgs, "{}")
		out, err := tool.Call(ctx, args)
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
}

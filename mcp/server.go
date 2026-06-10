package mcp

import (
	"cmp"
	"context"
	"errors"
	"fmt"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/Tangerg/lynx/core/model/chat"
)

// RegisterTools installs every [chat.Tool] in tools onto server using
// the low-level [(*sdkmcp.Server).AddTool] API.
//
// Registration is all-or-nothing: every tool is validated and built
// before any is added, so a bad entry mid-list never leaves the server
// half-registered.
//
// The generic sdkmcp.AddTool[In, Out] form is deliberately avoided:
// lynx tools already supply a hand-authored JSON schema, and the
// generic API would otherwise reflect over a Go In type and overwrite
// it.
func RegisterTools(server *sdkmcp.Server, tools ...chat.Tool) error {
	if server == nil {
		return ErrNilServer
	}

	prepared := make([]preparedTool, 0, len(tools))
	for i, tool := range tools {
		if tool == nil {
			return fmt.Errorf("mcp.RegisterTools: tools[%d] must not be nil", i)
		}
		pt, err := prepareOne(tool)
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
// RegisterTools builds in its first pass.
type preparedTool struct {
	tool    *sdkmcp.Tool
	handler sdkmcp.ToolHandler
}

func prepareOne(tool chat.Tool) (preparedTool, error) {
	def := tool.Definition()
	if def.Name == "" {
		return preparedTool{}, errors.New("mcp.RegisterTools: tool has empty name")
	}

	schema, err := stringSchemaToAny(def.InputSchema)
	if err != nil {
		return preparedTool{}, fmt.Errorf("mcp.RegisterTools: convert input schema for tool %q: %w", def.Name, err)
	}

	return preparedTool{
		tool: &sdkmcp.Tool{
			Name:        def.Name,
			Description: def.Description,
			InputSchema: schema,
		},
		handler: serverHandler(tool),
	}, nil
}

// serverHandler routes a tools/call RPC into a [chat.Tool]. Errors
// from the lynx tool surface via [sdkmcp.CallToolResult.IsError] plus
// a [*sdkmcp.TextContent] body — never as a Go error from the handler
// — because the latter would be promoted to a JSON-RPC protocol error
// and hide the failure from the LLM's view.
//
// The MCP server session is stamped onto the context so tool authors
// can use the reverse-capability helpers ([ReportProgress],
// [ElicitFromClient], [LogToClient]) without taking a direct
// dependency on the SDK.
func serverHandler(tool chat.Tool) sdkmcp.ToolHandler {
	return func(ctx context.Context, req *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
		toolName := tool.Definition().Name
		ctx, span := mcpTracer.Start(ctx, "mcp.tool.serve "+toolName,
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(attribute.String(attrToolName, toolName)),
		)
		defer span.End()

		// The SDK doesn't guarantee a non-nil request / params — guard like
		// withProgressToken does rather than dereferencing raw.
		var rawArgs string
		if req != nil {
			ctx = WithServerSession(ctx, req.Session)
			if req.Params != nil {
				rawArgs = string(req.Params.Arguments)
			}
		}
		ctx = withProgressToken(ctx, req)

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

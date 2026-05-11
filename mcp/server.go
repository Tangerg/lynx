package mcp

import (
	"cmp"
	"context"
	"errors"
	"fmt"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Tangerg/lynx/core/model/chat"
)

// RegisterTools installs every [chat.CallableTool] in tools onto
// server using the low-level [(*sdkmcp.Server).AddTool] API.
//
// The generic sdkmcp.AddTool[In, Out] form is deliberately avoided:
// lynx tools already supply a hand-authored JSON schema, and the
// generic API would otherwise reflect over a Go In type and overwrite
// it.
//
// Tools that only implement [chat.Tool] (delegation placeholders
// without an execution function) are rejected — an MCP server cannot
// serve an unrunnable tool.
func RegisterTools(server *sdkmcp.Server, tools ...chat.Tool) error {
	if server == nil {
		return ErrNilServer
	}

	for i, tool := range tools {
		if tool == nil {
			return fmt.Errorf("mcp.RegisterTools: tools[%d] must not be nil", i)
		}
		callable, ok := tool.(chat.CallableTool)
		if !ok {
			return fmt.Errorf("mcp.RegisterTools: tool %q is not a chat.CallableTool", tool.Definition().Name)
		}
		if err := registerOne(server, callable); err != nil {
			return err
		}
	}
	return nil
}

func registerOne(server *sdkmcp.Server, tool chat.CallableTool) error {
	def := tool.Definition()
	if def.Name == "" {
		return errors.New("mcp.RegisterTools: tool has empty name")
	}

	schema, err := stringSchemaToAny(def.InputSchema)
	if err != nil {
		return fmt.Errorf("mcp.RegisterTools: convert input schema for tool %q: %w", def.Name, err)
	}

	server.AddTool(
		&sdkmcp.Tool{
			Name:        def.Name,
			Description: def.Description,
			InputSchema: schema,
		},
		serverHandler(tool),
	)
	return nil
}

// serverHandler routes a tools/call RPC into a [chat.CallableTool].
// Errors from the lynx tool surface via [sdkmcp.CallToolResult.IsError]
// plus a [*sdkmcp.TextContent] body — never as a Go error from the
// handler — because the latter would be promoted to a JSON-RPC
// protocol error and hide the failure from the LLM's view.
func serverHandler(tool chat.CallableTool) sdkmcp.ToolHandler {
	return func(ctx context.Context, req *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
		args := cmp.Or(string(req.Params.Arguments), "{}")
		out, err := tool.Call(ctx, args)
		if err != nil {
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

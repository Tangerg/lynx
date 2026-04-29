package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Tangerg/lynx/core/model/chat"
)

// RegisterTools installs every chat.CallableTool in tools onto server using
// the low-level (*sdkmcp.Server).AddTool API.
//
// We deliberately avoid the generic sdkmcp.AddTool[In, Out] form: lynx tools
// already supply a hand-authored JSON schema string, and the generic API
// would otherwise reflect over a Go In type and overwrite that schema.
//
// Tools that only implement chat.Tool (delegation placeholders without an
// execution function) are rejected: an MCP server cannot serve an unrunnable
// tool.
func RegisterTools(server *sdkmcp.Server, tools ...chat.Tool) error {
	if server == nil {
		return errors.New("mcp server must not be nil")
	}

	for i, tool := range tools {
		if tool == nil {
			return fmt.Errorf("tools[%d] must not be nil", i)
		}

		callable, ok := tool.(chat.CallableTool)
		if !ok {
			return fmt.Errorf("tool %q cannot be exposed via MCP: not a chat.CallableTool", tool.Definition().Name)
		}

		if err := registerOne(server, callable); err != nil {
			return err
		}
	}
	return nil
}

// registerOne wires a single chat.CallableTool onto the MCP server.
func registerOne(server *sdkmcp.Server, tool chat.CallableTool) error {
	def := tool.Definition()
	if def.Name == "" {
		return errors.New("cannot register tool with empty name")
	}

	schema, err := stringSchemaToAny(def.InputSchema)
	if err != nil {
		return fmt.Errorf("convert input schema for tool %q: %w", def.Name, err)
	}

	descriptor := &sdkmcp.Tool{
		Name:        def.Name,
		Description: def.Description,
		InputSchema: schema,
	}
	server.AddTool(descriptor, makeServerHandler(tool))
	return nil
}

// makeServerHandler builds a low-level sdkmcp.ToolHandler that routes a
// tools/call RPC into a chat.CallableTool.
//
// Errors returned by the underlying lynx tool are surfaced via
// CallToolResult.IsError + a TextContent body — never as a Go error from
// the handler — because the latter would be promoted to a JSON-RPC
// protocol error and hide the failure from the LLM's view.
func makeServerHandler(tool chat.CallableTool) sdkmcp.ToolHandler {
	return func(ctx context.Context, req *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
		args := string(req.Params.Arguments)
		if args == "" {
			args = "{}"
		}

		out, err := tool.Call(ctx, args)
		if err != nil {
			return errorResult(err), nil
		}
		return textResult(out), nil
	}
}

// textResult wraps a single string as a successful CallToolResult.
func textResult(text string) *sdkmcp.CallToolResult {
	return &sdkmcp.CallToolResult{
		Content: []sdkmcp.Content{&sdkmcp.TextContent{Text: text}},
	}
}

// errorResult wraps an error as a CallToolResult with IsError=true.
func errorResult(err error) *sdkmcp.CallToolResult {
	return &sdkmcp.CallToolResult{
		Content: []sdkmcp.Content{&sdkmcp.TextContent{Text: err.Error()}},
		IsError: true,
	}
}

// stringSchemaToAny adapts a lynx ToolDefinition.InputSchema (always a JSON
// string in lynx) to the heterogeneous sdkmcp.Tool.InputSchema field
// (declared `any`). The SDK accepts json.RawMessage on the low-level
// AddTool path, which is exactly what we have.
func stringSchemaToAny(schema string) (any, error) {
	if schema == "" {
		return json.RawMessage(emptyObjectSchema), nil
	}
	if !json.Valid([]byte(schema)) {
		return nil, errors.New("schema is not valid JSON")
	}
	return json.RawMessage(schema), nil
}

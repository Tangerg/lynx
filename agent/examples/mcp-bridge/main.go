// mcp-bridge shows the reverse direction: lynx exposes a chat.CallableTool
// as an MCP server so external hosts (Claude Desktop, Cursor, …) can
// drive a lynx agent's tools.
//
// Run as a stdio MCP server (the host spawns the binary and talks over
// stdin/stdout):
//
//	go run ./agent/examples/mcp-bridge
//
// The example uses an in-memory transport pair so it runs offline; for
// real deployments swap to mcp.StdioTransport{} (or a streamable HTTP
// handler) — see lynx/mcp/DESIGN.md §5.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Tangerg/lynx/core/model/chat"
	lynxmcp "github.com/Tangerg/lynx/mcp"
)

func main() {
	ctx := context.Background()

	// 1. Build a chat.CallableTool — same shape an action body would
	// register and the same shape RegisterTools accepts.
	echo, err := chat.NewTool(
		chat.ToolDefinition{
			Name:        "echo",
			Description: "echo the input text",
			InputSchema: `{"type":"object","properties":{"text":{"type":"string"}},"required":["text"]}`,
		},
		chat.ToolMetadata{},
		func(_ context.Context, arguments string) (string, error) {
			var p struct {
				Text string `json:"text"`
			}
			if err := json.Unmarshal([]byte(arguments), &p); err != nil {
				return "", err
			}
			return p.Text, nil
		},
	)
	if err != nil {
		log.Fatal(err)
	}

	// 2. Stand up an MCP server and expose the tool.
	server := sdkmcp.NewServer(
		&sdkmcp.Implementation{Name: "lynx-bridge", Version: "v0.1.0"},
		nil,
	)
	if err := lynxmcp.RegisterTools(server, echo); err != nil {
		log.Fatal(err)
	}

	// 3. Drive the server end-to-end with a tiny in-memory client so the
	// example works offline. In production you'd swap this for:
	//
	//	server.Run(ctx, &sdkmcp.StdioTransport{})
	//
	// and let the MCP host (Claude Desktop, Cursor, …) spawn this binary.
	srvT, cliT := sdkmcp.NewInMemoryTransports()
	srvSession, err := server.Connect(ctx, srvT, nil)
	if err != nil {
		log.Fatal(err)
	}
	defer srvSession.Close()

	cli := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "demo-host", Version: "v0.1.0"}, nil)
	cliSession, err := cli.Connect(ctx, cliT, nil)
	if err != nil {
		log.Fatal(err)
	}
	defer cliSession.Close()

	// 4. Confirm the tool is visible and callable from the host's POV.
	for descriptor, err := range cliSession.Tools(ctx, nil) {
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("[host] discovered tool: %s — %s\n", descriptor.Name, descriptor.Description)
	}

	out, err := cliSession.CallTool(ctx, &sdkmcp.CallToolParams{
		Name:      "echo",
		Arguments: json.RawMessage(`{"text":"hello from the host"}`),
	})
	if err != nil {
		log.Fatal(err)
	}
	if text, ok := out.Content[0].(*sdkmcp.TextContent); ok {
		fmt.Printf("[host] tool result: %s\n", text.Text)
	}
}

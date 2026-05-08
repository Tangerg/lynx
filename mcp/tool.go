package mcp

import (
	"context"
	"errors"
	"fmt"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Tangerg/lynx/core/model/chat"
)

// Tool wraps a single remote MCP tool as a chat.CallableTool. Each Call
// dials the bound *sdkmcp.ClientSession's tools/call RPC, translates the
// result, and returns a (string, error) pair compatible with
// chat.ToolMiddleware. A remote IsError result is mapped to
// *ToolCallError; use errors.As to distinguish it from transport or
// protocol failures.
//
// The wrapper is immutable after construction.
type Tool struct {
	session    *sdkmcp.ClientSession
	descriptor *sdkmcp.Tool
	definition chat.ToolDefinition
	metadata   chat.ToolMetadata
	metaFunc   MetaFunc
}

// ToolConfig configures a [Tool]. Session and Descriptor are required;
// everything else has a zero-value default.
type ToolConfig struct {
	// Session is the live, initialized client session. The Tool does not
	// own the session; callers are responsible for closing it.
	Session *sdkmcp.ClientSession

	// Descriptor is the remote tool descriptor advertised by the server.
	// Its Name must be non-empty.
	Descriptor *sdkmcp.Tool

	// PrefixedName overrides the public name reported into the lynx tool
	// registry. Empty defaults to Descriptor.Name.
	PrefixedName string

	// Metadata is the chat.ToolMetadata reported by the wrapper. The
	// zero value is fine for most cases.
	Metadata chat.ToolMetadata

	// MetaFunc produces the _meta map carried on each CallTool RPC. Nil
	// forwards no metadata.
	MetaFunc MetaFunc
}

func (c *ToolConfig) validate() error {
	if c == nil {
		return errors.New("tool config must not be nil")
	}
	if c.Session == nil {
		return errors.New("tool config: session must not be nil")
	}
	if c.Descriptor == nil {
		return errors.New("tool config: descriptor must not be nil")
	}
	if c.Descriptor.Name == "" {
		return errors.New("tool config: descriptor has empty name")
	}
	if c.PrefixedName == "" {
		c.PrefixedName = c.Descriptor.Name
	}
	return nil
}

// NewTool builds a chat.CallableTool from cfg. cfg.Session must be
// initialized (returned from (*sdkmcp.Client).Connect) and must outlive
// the returned Tool.
func NewTool(cfg ToolConfig) (*Tool, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}

	schema, err := schemaToString(cfg.Descriptor.InputSchema)
	if err != nil {
		return nil, fmt.Errorf("convert input schema for tool %q: %w", cfg.Descriptor.Name, err)
	}

	return &Tool{
		session:    cfg.Session,
		descriptor: cfg.Descriptor,
		definition: chat.ToolDefinition{
			Name:        cfg.PrefixedName,
			Description: cfg.Descriptor.Description,
			InputSchema: schema,
		},
		metadata: cfg.Metadata,
		metaFunc: cfg.MetaFunc,
	}, nil
}

func (t *Tool) Definition() chat.ToolDefinition { return t.definition }
func (t *Tool) Metadata() chat.ToolMetadata     { return t.metadata }

// Descriptor returns the underlying MCP tool descriptor for callers
// that need fields beyond name/description/schema (annotations, output
// schema, icons, …).
func (t *Tool) Descriptor() *sdkmcp.Tool { return t.descriptor }

// Call implements chat.CallableTool. IsError=true on the remote result
// is mapped to *ToolCallError so a tool failure is not silently fed
// back to the model as a successful result.
func (t *Tool) Call(ctx context.Context, arguments string) (string, error) {
	args, err := decodeArguments(arguments)
	if err != nil {
		return "", fmt.Errorf("decode arguments for tool %q: %w", t.descriptor.Name, err)
	}

	params := &sdkmcp.CallToolParams{
		Name:      t.descriptor.Name,
		Arguments: args,
	}
	if t.metaFunc != nil {
		if meta := t.metaFunc(ctx); len(meta) > 0 {
			params.Meta = meta
		}
	}

	result, err := t.session.CallTool(ctx, params)
	if err != nil {
		return "", fmt.Errorf("call tool %q: %w", t.descriptor.Name, err)
	}
	if result.IsError {
		return "", &ToolCallError{
			ToolName: t.descriptor.Name,
			Message:  firstTextOrFallback(result.Content, "tool returned isError=true with no text content"),
		}
	}
	return flattenContent(result.Content)
}

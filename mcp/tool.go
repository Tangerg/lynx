package mcp

import (
	"context"
	"errors"
	"fmt"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/Tangerg/lynx/core/model/chat"
)

// Tool wraps a single remote MCP tool as a chat.Tool. Each Call
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

func (c *ToolConfig) Validate() error {
	if c.Session == nil {
		return ErrNilSession
	}
	if c.Descriptor == nil {
		return ErrNilDescriptor
	}
	if c.Descriptor.Name == "" {
		return errors.New("mcp.ToolConfig: descriptor has empty name")
	}
	return nil
}

// ApplyDefaults fills zero fields. PrefixedName defaults to the
// descriptor's name. Nil Descriptor is left alone — Validate surfaces
// it as an error.
func (c *ToolConfig) ApplyDefaults() {
	if c.PrefixedName == "" && c.Descriptor != nil {
		c.PrefixedName = c.Descriptor.Name
	}
}

// NewTool builds a [chat.Tool] from cfg. cfg.Session must be
// initialized (returned from (*sdkmcp.Client).Connect) and must outlive
// the returned Tool.
func NewTool(cfg ToolConfig) (*Tool, error) {
	cfg.ApplyDefaults()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	schema, err := schemaToString(cfg.Descriptor.InputSchema)
	if err != nil {
		return nil, fmt.Errorf("mcp.NewTool: convert input schema for tool %q: %w", cfg.Descriptor.Name, err)
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

// Call implements [chat.Tool]. IsError=true on the remote
// result is mapped to [*ToolCallError] so a tool failure is not
// silently fed back to the model as a successful result.
//
// One `mcp.tool.call <name>` span per call (kind=Client), carrying
// `lynx.tool.name` and (on failure) `lynx.mcp.tool.is_error=true`.
// No-op overhead when no TracerProvider is configured.
func (t *Tool) Call(ctx context.Context, arguments string) (out string, err error) {
	ctx, span := mcpTracer.Start(ctx, "mcp.tool.call "+t.descriptor.Name,
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(attribute.String(attrLynxMCPTool, t.descriptor.Name)),
	)
	defer func() {
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
		span.End()
	}()

	args, err := decodeArguments(arguments)
	if err != nil {
		return "", fmt.Errorf("mcp.Tool.Call: decode arguments for %q: %w", t.descriptor.Name, err)
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

	res, err := t.session.CallTool(ctx, params)
	if err != nil {
		return "", fmt.Errorf("mcp.Tool.Call: %q: %w", t.descriptor.Name, err)
	}
	if res.IsError {
		err = &ToolCallError{
			ToolName: t.descriptor.Name,
			Message:  firstTextOrFallback(res.Content, "tool returned isError=true with no text content"),
		}
		return "", err
	}
	return flattenContent(res.Content)
}

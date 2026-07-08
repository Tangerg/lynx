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

// tool wraps a single remote MCP tool as a chat.Tool. Each Call
// dials the bound *sdkmcp.ClientSession's tools/call RPC, translates the
// result, and returns a (string, error) pair compatible with chat.Tool. A
// remote IsError result is mapped to *ToolCallError; use errors.As to
// distinguish it from transport or protocol failures.
//
// The wrapper is immutable after construction.
type tool struct {
	session    *sdkmcp.ClientSession
	descriptor *sdkmcp.Tool
	definition chat.ToolDefinition
	metaFunc   MetaFunc
}

// toolConfig configures a [tool]. Session and Descriptor are required;
// everything else has a zero-value default.
type toolConfig struct {
	// Session is the live, initialized client session. The tool does not
	// own the session; callers are responsible for closing it.
	Session *sdkmcp.ClientSession

	// Descriptor is the remote tool descriptor advertised by the server.
	// Its Name must be non-empty.
	Descriptor *sdkmcp.Tool

	// PrefixedName overrides the public name reported into the tool
	// registry. Empty defaults to Descriptor.Name.
	PrefixedName string

	// MetaFunc produces the _meta map carried on each CallTool RPC. Nil
	// forwards no metadata.
	MetaFunc MetaFunc
}

func (c *toolConfig) validate() error {
	if c.Session == nil {
		return ErrNilSession
	}
	if c.Descriptor == nil {
		return errNilDescriptor
	}
	if c.Descriptor.Name == "" {
		return errors.New("mcp.toolConfig: descriptor has empty name")
	}
	if c.PrefixedName == "" {
		return errors.New("mcp.toolConfig: public name must not be empty")
	}
	return nil
}

func (c *toolConfig) applyDefaults() {
	if c.PrefixedName == "" && c.Descriptor != nil {
		c.PrefixedName = c.Descriptor.Name
	}
}

// newTool builds a [chat.Tool] from cfg. cfg.Session must be
// initialized (returned from (*sdkmcp.Client).Connect) and must outlive
// the returned tool.
func newTool(cfg toolConfig) (*tool, error) {
	cfg.applyDefaults()
	if err := cfg.validate(); err != nil {
		return nil, err
	}

	schema, err := schemaToString(cfg.Descriptor.InputSchema)
	if err != nil {
		return nil, fmt.Errorf("mcp.newTool: convert input schema for tool %q: %w", cfg.Descriptor.Name, err)
	}

	return &tool{
		session:    cfg.Session,
		descriptor: cfg.Descriptor,
		definition: chat.ToolDefinition{
			Name:        cfg.PrefixedName,
			Description: cfg.Descriptor.Description,
			InputSchema: schema,
		},
		metaFunc: cfg.MetaFunc,
	}, nil
}

func (t *tool) Definition() chat.ToolDefinition { return t.definition }

// Call implements [chat.Tool]. IsError=true on the remote
// result is mapped to [*ToolCallError] so a tool failure is not
// silently fed back to the model as a successful result.
//
// One `mcp.tool.call <name>` span per call (kind=Client), carrying
// `gen_ai.tool.name`; a failed call records the error and sets the span
// status to Error (no separate bool attribute). No-op overhead when no
// TracerProvider is configured.
func (t *tool) Call(ctx context.Context, arguments string) (out string, err error) {
	ctx, span := mcpTracer.Start(ctx, "mcp.tool.call "+t.descriptor.Name,
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(attribute.String(attrToolName, t.descriptor.Name)),
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
		return "", fmt.Errorf("mcp.tool.Call: decode arguments for %q: %w", t.descriptor.Name, err)
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
		return "", fmt.Errorf("mcp.tool.Call: %q: %w", t.descriptor.Name, err)
	}
	if res == nil {
		return "", fmt.Errorf("mcp.tool.Call: %q: nil CallToolResult", t.descriptor.Name)
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

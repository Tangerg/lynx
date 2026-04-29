package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Tangerg/lynx/core/model/chat"
)

// emptyObjectSchema is the canonical "accepts an empty JSON object" schema,
// used as a fallback when a tool advertises no input schema.
const emptyObjectSchema = `{"type":"object"}`

// ===== Naming =====

// NamingFunc maps a remote MCP tool to the public name reported into the lynx
// tool registry.
//
// It must be deterministic: the same input pair must always yield the same
// output, otherwise cache invalidation may produce mismatched names.
type NamingFunc func(sourceName string, tool *sdkmcp.Tool) string

// DefaultNaming returns "<sourceName>_<toolName>", or the bare tool name when
// the source name is empty. This is the strategy used when ProviderConfig.Naming
// is left nil.
var DefaultNaming NamingFunc = func(sourceName string, tool *sdkmcp.Tool) string {
	if sourceName == "" {
		return tool.Name
	}
	return sourceName + "_" + tool.Name
}

// ===== Meta =====

// MetaFunc produces the _meta map carried on each MCP CallToolParams. It is
// the seam through which a caller forwards ambient identifiers (userId,
// traceId, sessionId, ...) from the lynx-side context to the remote MCP
// server.
//
// A nil MetaFunc, or a MetaFunc that returns an empty map, results in no
// _meta being sent. Tool.Call and Provider treat a nil MetaFunc as
// "do not transmit metadata" — the safe default.
type MetaFunc func(ctx context.Context) sdkmcp.Meta

// metaContextKey is the unexported context key under which WithMeta stashes
// metadata. Using an unexported type prevents collisions with other packages.
type metaContextKey struct{}

// WithMeta returns a copy of ctx that carries the supplied metadata. Empty
// metadata is returned unchanged. Combine with MetaFromContext to forward
// per-request metadata across the tool subsystem.
func WithMeta(ctx context.Context, meta sdkmcp.Meta) context.Context {
	if len(meta) == 0 {
		return ctx
	}
	return context.WithValue(ctx, metaContextKey{}, meta)
}

// MetaFromContext returns metadata previously attached via WithMeta, or nil.
//
// Its signature matches MetaFunc, so it can be assigned directly:
//
//	cfg := &ProviderConfig{MetaFunc: lynxmcp.MetaFromContext}
func MetaFromContext(ctx context.Context) sdkmcp.Meta {
	meta, _ := ctx.Value(metaContextKey{}).(sdkmcp.Meta)
	return meta
}

// ===== Content =====

// flattenContent reduces a CallToolResult.Content slice into the single
// string shape that chat.CallableTool.Call must return.
//
// Rules:
//   - empty slice -> empty string
//   - exactly one TextContent -> its Text verbatim
//   - everything else -> JSON serialization of the entire slice, preserving
//     each element's "type" discriminator so the LLM still sees structure.
func flattenContent(items []sdkmcp.Content) (string, error) {
	if len(items) == 0 {
		return "", nil
	}

	if len(items) == 1 {
		if text, ok := items[0].(*sdkmcp.TextContent); ok {
			return text.Text, nil
		}
	}

	encoded, err := json.Marshal(items)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

// firstTextOrFallback returns the first non-empty TextContent.Text in items,
// or fallback if none is present. Used to produce a human-readable message
// for an MCP tool error.
func firstTextOrFallback(items []sdkmcp.Content, fallback string) string {
	for _, item := range items {
		if text, ok := item.(*sdkmcp.TextContent); ok && text.Text != "" {
			return text.Text
		}
	}
	return fallback
}

// ===== Schema =====

// schemaToString converts the heterogeneous sdkmcp.Tool.InputSchema (declared
// `any`) into the JSON string form lynx requires. Pre-encoded forms
// (string / json.RawMessage / []byte) are passed through; everything else
// is JSON-marshaled. A missing schema falls back to emptyObjectSchema.
func schemaToString(schema any) (string, error) {
	switch v := schema.(type) {
	case nil:
		return emptyObjectSchema, nil
	case string:
		if v == "" {
			return emptyObjectSchema, nil
		}
		return v, nil
	case json.RawMessage:
		if len(v) == 0 {
			return emptyObjectSchema, nil
		}
		return string(v), nil
	case []byte:
		if len(v) == 0 {
			return emptyObjectSchema, nil
		}
		return string(v), nil
	default:
		encoded, err := json.Marshal(v)
		if err != nil {
			return "", err
		}
		return string(encoded), nil
	}
}

// ===== Tool =====

// Tool wraps a single remote MCP tool as a chat.CallableTool.
//
// The wrapper is immutable after construction. Each Call dials the underlying
// *sdkmcp.ClientSession's CallTool RPC, translates the result, and returns a
// (string, error) pair compatible with chat.ToolMiddleware. A remote IsError
// result is mapped to *ToolCallError; use errors.As to distinguish it from
// transport or protocol failures.
type Tool struct {
	session    *sdkmcp.ClientSession
	descriptor *sdkmcp.Tool
	definition chat.ToolDefinition
	metadata   chat.ToolMetadata
	metaFunc   MetaFunc
}

// ToolConfig configures a Tool. Session and Descriptor are required;
// everything else has a zero-value default. Call Validate before use to
// surface missing fields and apply defaults in place.
type ToolConfig struct {
	// Session is the live, initialized client session. Required. The Tool
	// does not own the session: callers are responsible for closing it.
	Session *sdkmcp.ClientSession

	// Descriptor is the remote tool descriptor advertised by the server.
	// Required. Its Name must be non-empty.
	Descriptor *sdkmcp.Tool

	// PrefixedName overrides the tool's public name. Empty means use
	// Descriptor.Name; Validate fills it in.
	PrefixedName string

	// Metadata is the chat.ToolMetadata reported by the wrapper. The zero
	// value is fine for most cases.
	Metadata chat.ToolMetadata

	// MetaFunc produces the _meta map carried on each CallTool RPC. Nil
	// means no metadata is forwarded.
	MetaFunc MetaFunc
}

// Validate checks required fields and applies defaults in place. It is
// safe to call multiple times.
func (c *ToolConfig) Validate() error {
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

// NewTool builds a chat.CallableTool from the supplied configuration.
//
// cfg.Session must be initialized (returned from (*sdkmcp.Client).Connect)
// and must outlive the returned Tool.
func NewTool(cfg ToolConfig) (*Tool, error) {
	if err := cfg.Validate(); err != nil {
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

// Definition implements chat.Tool.
func (t *Tool) Definition() chat.ToolDefinition { return t.definition }

// Metadata implements chat.Tool.
func (t *Tool) Metadata() chat.ToolMetadata { return t.metadata }

// Descriptor returns the underlying MCP tool descriptor for callers that
// need fields beyond name/description/schema (annotations, output schema,
// icons, etc.).
func (t *Tool) Descriptor() *sdkmcp.Tool { return t.descriptor }

// Call implements chat.CallableTool. It marshals arguments, invokes
// tools/call on the remote session, and converts the response into the
// (string, error) shape expected by chat.ToolMiddleware.
//
// IsError=true on the result is mapped to *ToolCallError so that lynx tool
// middleware does not silently feed a tool failure back to the model as a
// successful result. Use errors.As(err, &tcErr) to detect this case.
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

// decodeArguments parses a JSON string argument blob into the typeless form
// accepted by sdkmcp.CallToolParams.Arguments. An empty input becomes an
// empty JSON object.
func decodeArguments(arguments string) (any, error) {
	if arguments == "" {
		return map[string]any{}, nil
	}

	var decoded any
	if err := json.Unmarshal([]byte(arguments), &decoded); err != nil {
		return nil, err
	}
	return decoded, nil
}

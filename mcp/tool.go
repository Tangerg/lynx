package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	corechat "github.com/Tangerg/lynx/core/chat"
	toolcontract "github.com/Tangerg/lynx/tools"
)

// tool wraps a single remote MCP tool as a chat.Tool. Each Call
// dials the bound *sdkmcp.ClientSession's tools/call RPC, translates the
// result, and returns a (string, error) pair compatible with chat.Tool. A
// remote IsError result is mapped to *ToolCallError; use errors.AsType to
// distinguish it from transport or protocol failures.
//
// The wrapper is immutable after construction.
type tool struct {
	session     *sdkmcp.ClientSession
	remoteName  string
	descriptor  *sdkmcp.Tool
	definition  corechat.ToolDefinition
	metaFunc    MetaFunc
	sourceName  string
	concurrency ConcurrencyFunc
}

var _ toolcontract.Tool = (*tool)(nil)

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

	// SourceName and Concurrency carry the optional caller-owned scheduling
	// policy. Nil Concurrency keeps the remote tool exclusive.
	SourceName  string
	Concurrency ConcurrencyFunc
}

// newTool builds a [tools.Tool] from cfg. cfg.Session must be
// initialized (returned from (*sdkmcp.Client).Connect) and must outlive
// the returned tool.
func newTool(cfg toolConfig) (*tool, error) {
	if cfg.Session == nil {
		return nil, ErrNilSession
	}
	if cfg.Descriptor == nil {
		return nil, errNilDescriptor
	}
	if cfg.Descriptor.Name == "" {
		return nil, errors.New("mcp: descriptor name must not be empty")
	}
	if cfg.PrefixedName == "" {
		cfg.PrefixedName = cfg.Descriptor.Name
	}

	descriptor, err := snapshotDescriptor(cfg.Descriptor)
	if err != nil {
		return nil, fmt.Errorf("mcp.newTool: snapshot descriptor for tool %q: %w", cfg.Descriptor.Name, err)
	}
	schema, err := schemaToJSON(descriptor.InputSchema)
	if err != nil {
		return nil, fmt.Errorf("mcp.newTool: convert input schema for tool %q: %w", descriptor.Name, err)
	}
	definition := corechat.ToolDefinition{
		Name:        cfg.PrefixedName,
		Description: descriptor.Description,
		InputSchema: schema,
	}
	if err := definition.Validate(); err != nil {
		return nil, fmt.Errorf("mcp.newTool: definition for remote tool %q: %w", descriptor.Name, err)
	}

	return &tool{
		session:     cfg.Session,
		remoteName:  descriptor.Name,
		descriptor:  descriptor,
		definition:  definition,
		metaFunc:    cfg.MetaFunc,
		sourceName:  cfg.SourceName,
		concurrency: cfg.Concurrency,
	}, nil
}

func snapshotDescriptor(descriptor *sdkmcp.Tool) (*sdkmcp.Tool, error) {
	data, err := json.Marshal(descriptor)
	if err != nil {
		return nil, err
	}
	var snapshot sdkmcp.Tool
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return nil, err
	}
	return &snapshot, nil
}

func (t *tool) Definition() corechat.ToolDefinition { return t.definition.Clone() }

// MCPToolIdentity returns the unsanitized source and remote tool names bound to
// this wrapper. Consumers use the pair for policy decisions; Definition.Name is
// a provider-constrained presentation label and is not an injective identity.
func (t *tool) MCPToolIdentity() (sourceName, remoteName string) {
	return t.sourceName, t.remoteName
}

// ConcurrencyKey structurally satisfies schedulers that support conflict-aware
// parallel calls without coupling this protocol adapter to a particular agent
// runtime. Unknown remote tools remain exclusive unless the caller supplied a
// policy through [ToolOptions.Concurrency].
func (t *tool) ConcurrencyKey(arguments string) (key string, concurrent bool) {
	if t.concurrency == nil {
		return "", false
	}
	return t.concurrency(t.sourceName, t.descriptor, arguments)
}

// Call implements [tools.Tool]. IsError=true on the remote
// result is mapped to [*ToolCallError] so a tool failure is not
// silently fed back to the model as a successful result.
//
// One `mcp.tool.call <name>` span per call (kind=Client), carrying
// `gen_ai.tool.name`; a failed call records the error and sets the span
// status to Error (no separate bool attribute). No-op overhead when no
// TracerProvider is configured.
func (t *tool) Call(ctx context.Context, arguments string) (out string, err error) {
	ctx, span := mcpTracer.Start(ctx, "mcp.tool.call "+t.remoteName,
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(attribute.String(attrToolName, t.remoteName)),
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
		return "", fmt.Errorf("mcp.tool.Call: decode arguments for %q: %w", t.remoteName, err)
	}

	params := &sdkmcp.CallToolParams{
		Name:      t.remoteName,
		Arguments: args,
	}
	if t.metaFunc != nil {
		if meta := t.metaFunc(ctx); len(meta) > 0 {
			params.Meta = maps.Clone(meta)
		}
	}

	res, err := t.session.CallTool(ctx, params)
	if err != nil {
		return "", fmt.Errorf("mcp.tool.Call: %q: %w", t.remoteName, err)
	}
	if res == nil {
		return "", fmt.Errorf("mcp.tool.Call: %q: nil CallToolResult", t.remoteName)
	}
	if res.IsError {
		err = &ToolCallError{
			ToolName: t.remoteName,
			Message:  firstTextOrFallback(res.Content, "tool returned isError=true with no text content"),
		}
		return "", err
	}
	return flattenContent(res.Content)
}

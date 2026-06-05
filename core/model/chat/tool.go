package chat

import (
	"context"
	"errors"
)

// ToolDefinition is the static description of a tool that LLMs see when
// deciding whether and how to call it. The InputSchema is a JSON Schema
// the model uses to format its arguments.
type ToolDefinition struct {
	// Name uniquely identifies the tool. Required.
	Name string

	// Description is a human-readable hint shown to the LLM.
	Description string

	// InputSchema is a JSON Schema describing the argument shape.
	// Required so the LLM can format arguments correctly.
	InputSchema string
}

// ToolMetadata controls how the framework treats a tool's result after
// execution.
type ToolMetadata struct {
	// ReturnDirect routes the tool result straight back to the caller
	// without re-prompting the LLM. Useful for UI affordances and
	// notifications. False (the default) sends the result back to the
	// LLM for integration into the next reply.
	ReturnDirect bool
}

// Tool is the executable contract every tool exposes — describable to
// the LLM (Definition / Metadata) and runnable by the framework (Call).
//
// Tools that cannot run in-process — human approval gates, frontend
// delegation, async dispatch — are not modeled as a separate type.
// Instead, layers above (agent middleware, tool decorators) wrap a Tool
// and surface control-flow signals via sentinel errors. See
// agent/hitl and agent/toolpolicy for production examples.
type Tool interface {
	// Definition returns the static description shown to the LLM.
	Definition() ToolDefinition

	// Metadata returns the post-execution behavior (return-direct, ...).
	Metadata() ToolMetadata

	// Call runs the tool's body. arguments is the JSON-encoded payload
	// the LLM produced. The string result is fed back to the LLM (or
	// returned to the caller when ReturnDirect is true).
	Call(ctx context.Context, arguments string) (string, error)
}

// ToolResult is the richer return of an [ArtifactTool]: the Content string
// is fed to the LLM exactly like [Tool.Call]'s result, while Artifact is an
// optional typed value carried out-of-band for non-LLM consumers (sinked
// onto the tool message, never shown to the model). Use it for tools that
// produce a binary or structured artifact — an image, a parsed document, a
// domain object — alongside a textual summary.
type ToolResult struct {
	// Content is the LLM-visible text (same role as [Tool.Call]'s string).
	Content string

	// Artifact is the out-of-band typed value, or nil. It lands on the
	// resulting [ToolReturn.Artifact].
	Artifact any
}

// ArtifactTool is an optional capability a [Tool] may also implement to
// return a typed artifact alongside its text. The tool-calling machinery
// detects it via type assertion (no change to the base [Tool] contract):
// when present, [ArtifactTool.CallArtifact] is used instead of
// [Tool.Call], and the artifact is attached to [ToolReturn.Artifact].
type ArtifactTool interface {
	Tool

	// CallArtifact runs the tool, returning the LLM-visible content plus an
	// optional artifact.
	CallArtifact(ctx context.Context, arguments string) (ToolResult, error)
}

// tool is the concrete backing for tools built via [NewTool].
type tool struct {
	definition ToolDefinition
	metadata   ToolMetadata
	execFunc   func(ctx context.Context, arguments string) (string, error)
}

func (t *tool) Definition() ToolDefinition { return t.definition }
func (t *tool) Metadata() ToolMetadata     { return t.metadata }

// Call runs the tool's exec function.
func (t *tool) Call(ctx context.Context, arguments string) (string, error) {
	return t.execFunc(ctx, arguments)
}

// NewTool builds a [Tool] backed by execFunc. All three components are
// required: an empty name, an empty input schema, or a nil exec function
// will return an error.
//
// To gate execution on human approval or to delegate execution to an
// external system, wrap the result with a decorator that returns a
// sentinel error (e.g., agent/hitl.RequireAwait) — the chat layer
// always treats a registered tool as runnable.
//
// Example:
//
//	tool, err := chat.NewTool(
//	    chat.ToolDefinition{Name: "add", InputSchema: addSchema},
//	    chat.ToolMetadata{},
//	    func(ctx context.Context, args string) (string, error) { ... },
//	)
func NewTool(definition ToolDefinition, metadata ToolMetadata, execFunc func(ctx context.Context, arguments string) (string, error)) (Tool, error) {
	if definition.Name == "" {
		return nil, errors.New("chat.NewTool: definition.Name must not be empty")
	}
	if definition.InputSchema == "" {
		return nil, errors.New("chat.NewTool: definition.InputSchema must not be empty")
	}
	if execFunc == nil {
		return nil, errors.New("chat.NewTool: execFunc must not be nil")
	}

	return &tool{
		definition: definition,
		metadata:   metadata,
		execFunc:   execFunc,
	}, nil
}

// artifactTool is the concrete backing for [NewArtifactTool].
type artifactTool struct {
	definition ToolDefinition
	metadata   ToolMetadata
	execFunc   func(ctx context.Context, arguments string) (ToolResult, error)
}

func (t *artifactTool) Definition() ToolDefinition { return t.definition }
func (t *artifactTool) Metadata() ToolMetadata     { return t.metadata }

// Call satisfies [Tool] by returning only the text content, so an
// artifactTool works anywhere a plain Tool is expected.
func (t *artifactTool) Call(ctx context.Context, arguments string) (string, error) {
	res, err := t.execFunc(ctx, arguments)
	return res.Content, err
}

// CallArtifact satisfies [ArtifactTool], returning content + artifact.
func (t *artifactTool) CallArtifact(ctx context.Context, arguments string) (ToolResult, error) {
	return t.execFunc(ctx, arguments)
}

// NewArtifactTool builds an [ArtifactTool] — a tool that returns a typed
// artifact alongside its LLM-visible text. Same required components as
// [NewTool]. The artifact lands on [ToolReturn.Artifact] and is never sent
// to the model.
func NewArtifactTool(definition ToolDefinition, metadata ToolMetadata, execFunc func(ctx context.Context, arguments string) (ToolResult, error)) (ArtifactTool, error) {
	if definition.Name == "" {
		return nil, errors.New("chat.NewArtifactTool: definition.Name must not be empty")
	}
	if definition.InputSchema == "" {
		return nil, errors.New("chat.NewArtifactTool: definition.InputSchema must not be empty")
	}
	if execFunc == nil {
		return nil, errors.New("chat.NewArtifactTool: execFunc must not be nil")
	}

	return &artifactTool{
		definition: definition,
		metadata:   metadata,
		execFunc:   execFunc,
	}, nil
}

package fs

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Tangerg/lynx/core/model/chat"
	pkgjson "github.com/Tangerg/lynx/pkg/json"
)

// WriteRequest is the LLM-facing argument shape for the write tool.
type WriteRequest struct {
	Path    string `json:"path" jsonschema:"required" jsonschema_description:"Absolute file path. Parent directories are created automatically."`
	Content string `json:"content" jsonschema:"required" jsonschema_description:"Full text content. Overwrites the file unless append=true. Must not contain NUL bytes."`
	Append  bool   `json:"append,omitempty" jsonschema_description:"Append to the file instead of overwriting. Default false."`
}

// WriteResponse is the LLM-facing return shape.
type WriteResponse struct {
	BytesWritten int `json:"bytes_written"`
}

var writeToolSchema, _ = pkgjson.StringDefSchemaOf(WriteRequest{})

var _ chat.Tool = (*WriteTool)(nil)

// WriteTool is the thin LLM-facing adapter for [Executor.Write].
type WriteTool struct {
	executor Executor
}

// NewWriteTool builds a [WriteTool] backed by executor. Passing nil
// wires up an unconfined [LocalExecutor] (workspace root "").
func NewWriteTool(executor Executor) *WriteTool {
	if executor == nil {
		executor = NewLocalExecutor("")
	}
	return &WriteTool{executor: executor}
}

func (t *WriteTool) Definition() chat.ToolDefinition {
	return chat.ToolDefinition{
		Name: "write",
		Description: "Create a new text file, overwrite an existing one, or append. " +
			"Prefer the `edit` tool when changing part of an existing file — it sends only the diff and is cheaper. " +
			"Parent directories are created automatically.",
		InputSchema: writeToolSchema,
	}
}

func (t *WriteTool) Metadata() chat.ToolMetadata { return chat.ToolMetadata{} }

func (t *WriteTool) Call(ctx context.Context, arguments string) (string, error) {
	var req WriteRequest
	if err := json.Unmarshal([]byte(arguments), &req); err != nil {
		return "", fmt.Errorf("fs.write: parse arguments: %w", err)
	}
	if req.Path == "" {
		return "", fmt.Errorf("fs.write: %w", ErrEmptyPath)
	}
	res, err := t.executor.Write(ctx, WriteInput(req))
	if err != nil {
		return "", fmt.Errorf("fs.write: %w", err)
	}
	body, err := json.Marshal(WriteResponse(res))
	if err != nil {
		return "", fmt.Errorf("fs.write: marshal: %w", err)
	}
	return string(body), nil
}

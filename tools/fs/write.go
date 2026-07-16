package fs

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Tangerg/lynx/core/chat"
	pkgjson "github.com/Tangerg/lynx/pkg/json"
	toolcontract "github.com/Tangerg/lynx/tools"
)

// WriteRequest is the LLM-facing argument shape for the write tool.
type WriteRequest struct {
	FilePath string `json:"file_path" jsonschema:"required" jsonschema_description:"Path to the file — absolute, or relative to the workspace root. Parent directories are created automatically."`
	Content  string `json:"content" jsonschema:"required" jsonschema_description:"Full text content. Overwrites the file unless append=true. Must not contain NUL bytes."`
	Append   bool   `json:"append,omitempty" jsonschema_description:"Append to the end of the file instead of overwriting. Default false."`
}

// WriteResponse is the LLM-facing return shape.
type WriteResponse struct {
	BytesWritten int `json:"bytes_written"`
}

var writeToolSchema, _ = pkgjson.StringDefSchemaOf(WriteRequest{})

var (
	_ toolcontract.Tool                 = (*WriteTool)(nil)
	_ toolcontract.FileMutationReporter = (*WriteTool)(nil)
)

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
		Description: "Create a new file, or overwrite / append to an existing one. " +
			"Before overwriting a file that already exists you must `read` it first — a blind overwrite is refused. " +
			"Prefer the `edit` tool when changing part of an existing file: it sends only the diff, and is cheaper and safer. " +
			"Parent directories are created automatically.",
		InputSchema: json.RawMessage(writeToolSchema),
	}
}

// ConcurrencyKey opts write into concurrent execution keyed on its target file
// (the tool loop's optional concurrency contract): distinct-file writes run in
// parallel, same-file writes serialize. An unparseable / empty path yields no
// key (no known conflict).
func (t *WriteTool) ConcurrencyKey(arguments string) (key string, concurrent bool) {
	var req WriteRequest
	_ = json.Unmarshal([]byte(arguments), &req)
	return req.FilePath, true
}

// MutationPaths reports the file targeted by this call.
func (*WriteTool) MutationPaths(arguments string) ([]string, error) {
	var req WriteRequest
	if err := json.Unmarshal([]byte(arguments), &req); err != nil {
		return nil, err
	}
	if req.FilePath == "" {
		return nil, nil
	}
	return []string{req.FilePath}, nil
}

func (t *WriteTool) Call(ctx context.Context, arguments string) (string, error) {
	_ = ctx
	var req WriteRequest
	if err := json.Unmarshal([]byte(arguments), &req); err != nil {
		return "", fmt.Errorf("fs.write: parse arguments: %w", err)
	}
	if req.FilePath == "" {
		return "", fmt.Errorf("fs.write: %w", ErrEmptyPath)
	}
	res, err := t.executor.Write(ctx, WriteInput{Path: req.FilePath, Content: req.Content, Append: req.Append})
	if err != nil {
		return "", fmt.Errorf("fs.write: %w", err)
	}
	body, err := json.Marshal(WriteResponse(res))
	if err != nil {
		return "", fmt.Errorf("fs.write: marshal: %w", err)
	}
	return string(body), nil
}

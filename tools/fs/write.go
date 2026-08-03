package fs

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Tangerg/lynx/core/chat"
	toolcontract "github.com/Tangerg/lynx/tool"
)

// WriteRequest is the LLM-facing argument shape for the write tool.
type WriteRequest struct {
	Path    string `json:"path" jsonschema:"minLength=1" jsonschema_description:"File path, absolute or relative to the workspace root. Parent directories are created automatically."`
	Content string `json:"content" jsonschema_description:"Complete text content for the file. Existing content is replaced. Must not contain NUL bytes."`
}

// WriteResponse is the LLM-facing return shape.
type WriteResponse struct {
	BytesWritten int `json:"bytes_written"`
}

var _ toolcontract.Tool = (*WriteTool)(nil)

// WriteTool is the thin LLM-facing adapter for [Executor.Write].
type WriteTool struct {
	executor Executor
	typed    *toolcontract.Func[WriteRequest, WriteResponse]
}

// NewWriteTool builds a [WriteTool] backed by executor. Passing nil
// wires up an unconfined [LocalExecutor] (workspace root "").
func NewWriteTool(executor Executor) *WriteTool {
	if executor == nil {
		executor = NewLocalExecutor("")
	}
	t := &WriteTool{executor: executor}
	t.typed = mustTypedTool(
		toolcontract.FuncConfig{
			Name: "write",
			Description: "Create a file or replace the complete contents of an existing file. " +
				"Before replacing an existing file, read the whole file; a blind overwrite is refused. " +
				"Use edit for a targeted change to an existing file. Parent directories are created automatically.",
		},
		t.write,
	)
	return t
}

func (t *WriteTool) Definition() chat.ToolDefinition {
	return t.typed.Definition()
}

// ConcurrencyKey opts write into concurrent execution keyed on its target file
// (the tool loop's optional concurrency contract): distinct-file writes run in
// parallel, same-file writes serialize. An unparseable / empty path yields no
// key (no known conflict).
func (t *WriteTool) ConcurrencyKey(arguments string) (key string, concurrent bool) {
	var req WriteRequest
	_ = json.Unmarshal([]byte(arguments), &req)
	return req.Path, true
}

// MutationPaths reports the file targeted by this call.
func (*WriteTool) MutationPaths(arguments string) ([]string, error) {
	var req WriteRequest
	if err := json.Unmarshal([]byte(arguments), &req); err != nil {
		return nil, err
	}
	if req.Path == "" {
		return nil, nil
	}
	return []string{req.Path}, nil
}

func (t *WriteTool) Call(ctx context.Context, arguments string) (string, error) {
	return t.typed.Call(ctx, arguments)
}

func (t *WriteTool) write(ctx context.Context, req WriteRequest) (WriteResponse, error) {
	res, err := t.executor.Write(ctx, WriteInput{Path: req.Path, Content: req.Content})
	if err != nil {
		return WriteResponse{}, fmt.Errorf("fs.write: %w", err)
	}
	return WriteResponse(res), nil
}

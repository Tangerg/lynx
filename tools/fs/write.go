package fs

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Tangerg/scope/core/chat"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

type WriteRequest struct {
	Path    string `json:"path" jsonschema:"minLength=1" jsonschema_description:"File path, absolute or relative to the workspace root. Parent directories are created automatically."`
	Content string `json:"content" jsonschema_description:"Complete text content for the file. Existing content is replaced. Must not contain NUL bytes."`
}

type WriteResponse struct {
	BytesWritten int `json:"bytes_written"`
}

var _ toolcontract.Tool = (*WriteTool)(nil)

type WriteTool struct {
	executor Writer
	typed    toolcontract.Func[WriteRequest, WriteResponse]
}

func NewWriteTool(executor Writer) *WriteTool {
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

func (w *WriteTool) Definition() chat.ToolDefinition {
	return w.typed.Definition()
}

// ConcurrencyKey opts write into concurrent execution keyed on its target file
// (the tool loop's optional concurrency contract): distinct-file writes run in
// parallel, same-file writes serialize. An unparseable / empty path yields no
// key (no known conflict).
func (w *WriteTool) ConcurrencyKey(arguments string) (key string, concurrent bool) {
	var req WriteRequest
	_ = json.Unmarshal([]byte(arguments), &req)
	return req.Path, true
}

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

func (w *WriteTool) Call(ctx context.Context, arguments string) (string, error) {
	return w.typed.Call(ctx, arguments)
}

func (w *WriteTool) write(ctx context.Context, req WriteRequest) (WriteResponse, error) {
	res, err := w.executor.Write(ctx, WriteInput{Path: req.Path, Content: req.Content})
	if err != nil {
		return WriteResponse{}, fmt.Errorf("fs.write: %w", err)
	}
	return res, nil
}

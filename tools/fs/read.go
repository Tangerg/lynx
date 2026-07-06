package fs

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Tangerg/lynx/core/model/chat"
	pkgjson "github.com/Tangerg/lynx/pkg/json"
)

// ReadRequest is the LLM-facing argument shape for the read tool.
// Offset is 1-based to match editor / grep / IDE conventions; pass
// 0 or omit to start at the first line.
type ReadRequest struct {
	FilePath string `json:"file_path" jsonschema:"required" jsonschema_description:"Path to the file to read — absolute, or relative to the workspace root."`
	Offset   int    `json:"offset,omitempty" jsonschema_description:"1-based line number to start reading from. 0 or omitted = start at line 1. Pair with limit to page through a large file."`
	Limit    int    `json:"limit,omitempty" jsonschema_description:"Maximum number of lines to return. 0 = read to the end of the file."`
}

// ReadResponse is the LLM-facing return shape. StartLine / EndLine
// are 1-based inclusive.
type ReadResponse struct {
	Content    string `json:"content"`
	StartLine  int    `json:"start_line"`
	EndLine    int    `json:"end_line"`
	TotalLines int    `json:"total_lines"`
	Truncated  bool   `json:"truncated,omitempty"`
}

var readToolSchema, _ = pkgjson.StringDefSchemaOf(ReadRequest{})

var _ chat.Tool = (*ReadTool)(nil)

// ReadTool is the thin LLM-facing adapter for [Executor.Read].
type ReadTool struct {
	executor Executor
}

// NewReadTool builds a [ReadTool] backed by executor. Passing nil
// wires up an unconfined [LocalExecutor] (workspace root "").
func NewReadTool(executor Executor) *ReadTool {
	if executor == nil {
		executor = NewLocalExecutor("")
	}
	return &ReadTool{executor: executor}
}

func (t *ReadTool) Definition() chat.ToolDefinition {
	return chat.ToolDefinition{
		Name: "read",
		Description: "Read a text file from the filesystem. Returns the requested line range with the total line count and a truncation flag. " +
			"By default returns the whole file; for a large file pass `offset` (1-based line) and `limit` to page through it. " +
			"Call this in parallel when you need several files at once. " +
			"Binary files are rejected — use the shell tool for non-text data, and use glob/grep to locate files or content rather than guessing paths.",
		InputSchema: readToolSchema,
	}
}

// ConcurrencyKey opts read into parallel execution — a pure read has no
// resource conflict (the tool loop's optional concurrency contract), so the
// loop runs several reads (and reads alongside other parallel tools) at once.
func (t *ReadTool) ConcurrencyKey(string) (key string, concurrent bool) { return "", true }

func (t *ReadTool) Call(ctx context.Context, arguments string) (string, error) {
	_ = ctx
	var req ReadRequest
	if err := json.Unmarshal([]byte(arguments), &req); err != nil {
		return "", fmt.Errorf("fs.read: parse arguments: %w", err)
	}
	if req.FilePath == "" {
		return "", fmt.Errorf("fs.read: %w", ErrEmptyPath)
	}

	// LLM speaks 1-based offset; the executor SPI is 0-based.
	spiOffset := 0
	if req.Offset > 0 {
		spiOffset = req.Offset - 1
	}

	res, err := t.executor.Read(ctx, ReadInput{
		Path:   req.FilePath,
		Offset: spiOffset,
		Limit:  req.Limit,
	})
	if err != nil {
		return "", fmt.Errorf("fs.read: %w", err)
	}

	body, err := json.Marshal(ReadResponse{
		Content:    res.Content,
		StartLine:  res.StartLine + 1, // 0-based exclusive start → 1-based inclusive
		EndLine:    res.EndLine,       // 0-based exclusive end → 1-based inclusive
		TotalLines: res.TotalLines,
		Truncated:  res.Truncated,
	})
	if err != nil {
		return "", fmt.Errorf("fs.read: marshal: %w", err)
	}
	return string(body), nil
}

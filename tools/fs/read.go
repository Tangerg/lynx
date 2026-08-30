package fs

import (
	"context"
	"fmt"

	"github.com/samber/lo"

	"github.com/Tangerg/scope/core/chat"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

// ReadRequest is the LLM-facing argument shape for the read tool. StartLine is
// 1-based to match editor, grep, and language-server conventions.
type ReadRequest struct {
	Path      string `json:"path" jsonschema:"minLength=1" jsonschema_description:"File path, absolute or relative to the workspace root."`
	StartLine int    `json:"start_line,omitempty" jsonschema:"minimum=1" jsonschema_description:"1-based line at which to start. Omit to start at line 1."`
	MaxLines  int    `json:"max_lines,omitempty" jsonschema:"minimum=1" jsonschema_description:"Maximum lines to return. Omit to read through the end of the file."`
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

var _ toolcontract.Tool = (*ReadTool)(nil)

type ReadTool struct {
	executor Reader
	typed    toolcontract.Func[ReadRequest, ReadResponse]
}

func NewReadTool(executor Reader) *ReadTool {
	if lo.IsNil(executor) {
		executor = NewLocalExecutor("")
	}
	t := &ReadTool{executor: executor}
	t.typed = mustTypedTool(
		toolcontract.FuncConfig{
			Name: "read",
			Description: "Read a text file from the filesystem. Returns the requested line range with the total line count and a truncation flag. " +
				"By default returns the admitted file up to the backend's input, line, and output bounds; pass start_line and max_lines to page by 1-based line number. " +
				"Call this in parallel when you need several files at once. " +
				"Binary files are rejected; use shell for non-text data, and use glob or grep to locate files or content rather than guessing paths.",
		},
		t.read,
	)
	return t
}

func (r *ReadTool) Definition() chat.ToolDefinition {
	return r.typed.Definition()
}

// ConcurrencyKey opts read into parallel execution — a pure read has no
// resource conflict (the tool loop's optional concurrency contract), so the
// loop runs several reads (and reads alongside other parallel tools) at once.
func (r *ReadTool) ConcurrencyKey(toolcontract.Invocation) (key string, concurrent bool) {
	return "", true
}

func (r *ReadTool) Call(ctx context.Context, invocation toolcontract.Invocation) (chat.ToolOutput, error) {
	return r.typed.Call(ctx, invocation)
}

func (r *ReadTool) read(ctx context.Context, req ReadRequest) (ReadResponse, error) {
	// The model-facing start line is 1-based; the executor SPI is 0-based.
	spiOffset := 0
	if req.StartLine > 0 {
		spiOffset = req.StartLine - 1
	}

	res, err := r.executor.Read(ctx, ReadInput{
		Path:   req.Path,
		Offset: spiOffset,
		Limit:  req.MaxLines,
	})
	if err != nil {
		return ReadResponse{}, fmt.Errorf("fs.read: %w", err)
	}

	return ReadResponse{
		Content:    res.Content,
		StartLine:  res.StartLine + 1, // 0-based exclusive start → 1-based inclusive
		EndLine:    res.EndLine,       // 0-based exclusive end → 1-based inclusive
		TotalLines: res.TotalLines,
		Truncated:  res.Truncated,
	}, nil
}

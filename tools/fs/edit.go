package fs

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Tangerg/lynx/core/model/chat"
	pkgjson "github.com/Tangerg/lynx/pkg/json"
)

// EditRequest is the LLM-facing argument shape for the edit tool.
type EditRequest struct {
	FilePath   string `json:"file_path" jsonschema:"required" jsonschema_description:"Path to the file to edit — absolute, or relative to the workspace root."`
	OldString  string `json:"old_string" jsonschema:"required" jsonschema_description:"Exact text to find, copied verbatim from the file (the read tool returns raw text — there is no line-number prefix to strip). Keep it to the few unique lines needed; fails when the match is not unique unless replace_all=true."`
	NewString  string `json:"new_string" jsonschema:"required" jsonschema_description:"Replacement text. Preserve the surrounding indentation exactly. Must differ from old_string."`
	ReplaceAll bool   `json:"replace_all,omitempty" jsonschema_description:"Replace every occurrence. Default false. Use this for renaming a symbol across the file."`
}

// EditResponse is the LLM-facing return shape.
type EditResponse struct {
	Replacements int `json:"replacements"`
}

var editToolSchema, _ = pkgjson.StringDefSchemaOf(EditRequest{})

var _ chat.Tool = (*EditTool)(nil)

// EditTool is the thin LLM-facing adapter for [Executor.Edit]. The
// match-and-replace logic lives in the executor so a backend upgrade
// can swap match policy without changing the tool.
type EditTool struct {
	executor Executor
}

// NewEditTool builds an [EditTool] backed by executor. Passing nil
// wires up an unconfined [LocalExecutor] (workspace root "").
func NewEditTool(executor Executor) *EditTool {
	if executor == nil {
		executor = NewLocalExecutor("")
	}
	return &EditTool{executor: executor}
}

func (t *EditTool) Definition() chat.ToolDefinition {
	return chat.ToolDefinition{
		Name: "edit",
		Description: "Replace exact text in a file. You must `read` the file before editing it — an edit without a prior read is refused. " +
			"Copy `old_string` verbatim from the file: `read` returns raw text, so there is no line-number prefix to strip. " +
			"Keep `old_string` to the few unique lines needed — larger snippets drift on whitespace. " +
			"Pass `replace_all=true` to change every occurrence (use this when renaming a symbol).",
		InputSchema: editToolSchema,
	}
}

func (t *EditTool) Metadata() chat.ToolMetadata { return chat.ToolMetadata{} }

// ConcurrencyKey opts edit into concurrent execution keyed on its target file
// — the tool loop's optional concurrency contract (a tool reports per call
// whether it may overlap others and the resource it conflicts on). The loop
// parallelizes edits to DISTINCT files and serializes edits to the SAME file.
// An unparseable / empty path yields no key (no known conflict); the call still
// fails its own validation in Call.
func (t *EditTool) ConcurrencyKey(arguments string) (key string, concurrent bool) {
	var req EditRequest
	_ = json.Unmarshal([]byte(arguments), &req)
	return req.FilePath, true
}

func (t *EditTool) Call(ctx context.Context, arguments string) (string, error) {
	_ = ctx
	var req EditRequest
	if err := json.Unmarshal([]byte(arguments), &req); err != nil {
		return "", fmt.Errorf("fs.edit: parse arguments: %w", err)
	}
	if req.FilePath == "" {
		return "", fmt.Errorf("fs.edit: %w", ErrEmptyPath)
	}
	res, err := t.executor.Edit(ctx, EditInput{Path: req.FilePath, OldString: req.OldString, NewString: req.NewString, ReplaceAll: req.ReplaceAll})
	if err != nil {
		return "", fmt.Errorf("fs.edit: %w", err)
	}
	body, err := json.Marshal(EditResponse(res))
	if err != nil {
		return "", fmt.Errorf("fs.edit: marshal: %w", err)
	}
	return string(body), nil
}

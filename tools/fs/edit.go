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
	Path       string `json:"path" jsonschema:"required" jsonschema_description:"Absolute path to a text file."`
	OldString  string `json:"old_string" jsonschema:"required" jsonschema_description:"Exact text to find. Keep it small — usually 2-4 lines of unique context is enough. Fails when the match is not unique unless replace_all=true."`
	NewString  string `json:"new_string" jsonschema:"required" jsonschema_description:"Replacement text. Preserve the surrounding indentation exactly."`
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
		Description: "Replace exact text in a text file. Keep `old_string` as small as possible — usually 2-4 lines of unique context is enough; larger snippets are more likely to drift on whitespace. " +
			"Pass `replace_all=true` to change every occurrence (use this when renaming a symbol).",
		InputSchema: editToolSchema,
	}
}

func (t *EditTool) Metadata() chat.ToolMetadata { return chat.ToolMetadata{} }

func (t *EditTool) Call(ctx context.Context, arguments string) (string, error) {
	_ = ctx
	var req EditRequest
	if err := json.Unmarshal([]byte(arguments), &req); err != nil {
		return "", fmt.Errorf("fs.edit: parse arguments: %w", err)
	}
	if req.Path == "" {
		return "", fmt.Errorf("fs.edit: %w", ErrEmptyPath)
	}
	res, err := t.executor.Edit(ctx, EditInput(req))
	if err != nil {
		return "", fmt.Errorf("fs.edit: %w", err)
	}
	body, err := json.Marshal(EditResponse(res))
	if err != nil {
		return "", fmt.Errorf("fs.edit: marshal: %w", err)
	}
	return string(body), nil
}

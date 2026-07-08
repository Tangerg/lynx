package fs

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Tangerg/lynx/core/model/chat"
	pkgjson "github.com/Tangerg/lynx/pkg/json"
)

// MultiEditRequest is the LLM-facing argument shape for the multiedit tool.
type MultiEditRequest struct {
	FilePath string            `json:"file_path" jsonschema:"required" jsonschema_description:"Path to the file to edit — absolute, or relative to the workspace root."`
	Changes  []EditRequestItem `json:"edits" jsonschema:"required" jsonschema_description:"Ordered replacements to apply. All edits are validated before the file is written; if any edit fails, no change is saved."`
}

// EditRequestItem is one replacement inside a MultiEditRequest.
type EditRequestItem struct {
	OldString  string `json:"old_string" jsonschema:"required" jsonschema_description:"Exact text to find, copied verbatim from the file. Each edit is applied after the previous edit."`
	NewString  string `json:"new_string" jsonschema:"required" jsonschema_description:"Replacement text."`
	ReplaceAll bool   `json:"replace_all,omitempty" jsonschema_description:"Replace every occurrence for this edit. Default false."`
}

// MultiEditResponse is the LLM-facing return shape.
type MultiEditResponse struct {
	Edits        int `json:"edits"`
	Replacements int `json:"replacements"`
}

var multiEditToolSchema, _ = pkgjson.StringDefSchemaOf(MultiEditRequest{})

var _ chat.Tool = (*MultiEditTool)(nil)

// MultiEditTool is the thin LLM-facing adapter for [Executor.MultiEdit].
type MultiEditTool struct {
	executor Executor
}

// NewMultiEditTool builds a [MultiEditTool] backed by executor. Passing nil
// wires up an unconfined [LocalExecutor] (workspace root "").
func NewMultiEditTool(executor Executor) *MultiEditTool {
	if executor == nil {
		executor = NewLocalExecutor("")
	}
	return &MultiEditTool{executor: executor}
}

func (t *MultiEditTool) Definition() chat.ToolDefinition {
	return chat.ToolDefinition{
		Name: "multiedit",
		Description: "Apply several exact text replacements to one file in order. You must `read` the file first. " +
			"The file is read once and written once; if any edit fails, no changes are saved. Use this when a single file needs several coordinated replacements.",
		InputSchema: multiEditToolSchema,
	}
}

func (t *MultiEditTool) ConcurrencyKey(arguments string) (key string, concurrent bool) {
	var req MultiEditRequest
	_ = json.Unmarshal([]byte(arguments), &req)
	return req.FilePath, true
}

func (t *MultiEditTool) MutatedPaths(arguments string) ([]string, error) {
	var req MultiEditRequest
	if err := json.Unmarshal([]byte(arguments), &req); err != nil {
		return nil, err
	}
	if req.FilePath == "" {
		return nil, nil
	}
	return []string{req.FilePath}, nil
}

func (t *MultiEditTool) Call(ctx context.Context, arguments string) (string, error) {
	var req MultiEditRequest
	if err := json.Unmarshal([]byte(arguments), &req); err != nil {
		return "", fmt.Errorf("fs.multiedit: parse arguments: %w", err)
	}
	if req.FilePath == "" {
		return "", fmt.Errorf("fs.multiedit: %w", ErrEmptyPath)
	}
	if len(req.Changes) == 0 {
		return "", fmt.Errorf("fs.multiedit: edits must not be empty")
	}
	ops := make([]EditOperation, len(req.Changes))
	for i, edit := range req.Changes {
		ops[i] = EditOperation{
			OldString:  edit.OldString,
			NewString:  edit.NewString,
			ReplaceAll: edit.ReplaceAll,
		}
	}
	res, err := t.executor.MultiEdit(ctx, MultiEditInput{Path: req.FilePath, Edits: ops})
	if err != nil {
		return "", fmt.Errorf("fs.multiedit: %w", err)
	}
	body, err := json.Marshal(MultiEditResponse(res))
	if err != nil {
		return "", fmt.Errorf("fs.multiedit: marshal: %w", err)
	}
	return string(body), nil
}

package fs

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Tangerg/lynx/core/chat"
	pkgjson "github.com/Tangerg/lynx/pkg/json"
	toolcontract "github.com/Tangerg/lynx/tools"
)

// ApplyPatchRequest is the LLM-facing argument shape for the apply_patch tool.
type ApplyPatchRequest struct {
	Patch string `json:"patch" jsonschema:"required" jsonschema_description:"A standard unified diff. Supports create, modify, and delete hunks. Renames are rejected."`
}

// ApplyPatchResponse is the LLM-facing return shape.
type ApplyPatchResponse struct {
	Files []PatchFileResponse `json:"files"`
	Hunks int                 `json:"hunks"`
}

// PatchFileResponse reports one patched file.
type PatchFileResponse struct {
	FilePath string `json:"file_path"`
	Hunks    int    `json:"hunks"`
	Created  bool   `json:"created,omitempty"`
	Deleted  bool   `json:"deleted,omitempty"`
}

var applyPatchToolSchema, _ = pkgjson.StringDefSchemaOf(ApplyPatchRequest{})

var (
	_ toolcontract.Tool                 = (*ApplyPatchTool)(nil)
	_ toolcontract.FileMutationReporter = (*ApplyPatchTool)(nil)
)

// ApplyPatchTool is the thin LLM-facing adapter for [Executor.ApplyPatch].
type ApplyPatchTool struct {
	executor Executor
}

// NewApplyPatchTool builds an [ApplyPatchTool] backed by executor. Passing nil
// wires up an unconfined [LocalExecutor] (workspace root "").
func NewApplyPatchTool(executor Executor) *ApplyPatchTool {
	if executor == nil {
		executor = NewLocalExecutor("")
	}
	return &ApplyPatchTool{executor: executor}
}

func (t *ApplyPatchTool) Definition() chat.ToolDefinition {
	return chat.ToolDefinition{
		Name: "apply_patch",
		Description: "Apply a standard unified diff to one or more files. You must `read` existing files before patching them. " +
			"Use this for coordinated multi-file changes. The patch must match exactly; unsupported renames are rejected instead of guessed.",
		InputSchema: json.RawMessage(applyPatchToolSchema),
	}
}

func (*ApplyPatchTool) MutationPaths(arguments string) ([]string, error) {
	var req ApplyPatchRequest
	if err := json.Unmarshal([]byte(arguments), &req); err != nil {
		return nil, err
	}
	return patchPaths(req.Patch)
}

func (t *ApplyPatchTool) Call(ctx context.Context, arguments string) (string, error) {
	var req ApplyPatchRequest
	if err := json.Unmarshal([]byte(arguments), &req); err != nil {
		return "", fmt.Errorf("fs.apply_patch: parse arguments: %w", err)
	}
	res, err := t.executor.ApplyPatch(ctx, ApplyPatchInput(req))
	if err != nil {
		return "", fmt.Errorf("fs.apply_patch: %w", err)
	}
	files := make([]PatchFileResponse, len(res.Files))
	for i, file := range res.Files {
		files[i] = PatchFileResponse{
			FilePath: file.Path,
			Hunks:    file.Hunks,
			Created:  file.Created,
			Deleted:  file.Deleted,
		}
	}
	body, err := json.Marshal(ApplyPatchResponse{Files: files, Hunks: res.Hunks})
	if err != nil {
		return "", fmt.Errorf("fs.apply_patch: marshal: %w", err)
	}
	return string(body), nil
}

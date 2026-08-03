package fs

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Tangerg/lynx/core/chat"
	toolcontract "github.com/Tangerg/lynx/tool"
)

// ApplyPatchRequest is the LLM-facing argument shape for the apply_patch tool.
type ApplyPatchRequest struct {
	Patch string `json:"patch" jsonschema:"required" jsonschema_description:"A standard unified diff. Supports create (--- /dev/null), modify, delete (+++ /dev/null), and move (the two headers naming different paths; a pure rename carries no hunks)."`
}

// ApplyPatchResponse is the LLM-facing return shape.
type ApplyPatchResponse struct {
	Files []PatchFileResponse `json:"files"`
	Hunks int                 `json:"hunks"`
}

// PatchFileResponse reports one patched file.
type PatchFileResponse struct {
	FilePath  string `json:"file_path"`
	Hunks     int    `json:"hunks"`
	Created   bool   `json:"created,omitempty"`
	Deleted   bool   `json:"deleted,omitempty"`
	MovedFrom string `json:"moved_from,omitempty"`
}

var applyPatchToolSchema, _ = toolcontract.Schema[ApplyPatchRequest]()

var _ toolcontract.Tool = (*ApplyPatchTool)(nil)

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
		Description: "Apply a standard unified diff to one or more files, including creating, deleting and moving them. " +
			"You must `read` existing files before patching them. Use this for coordinated multi-file changes — a rename plus the " +
			"edits that follow from it is one call. The patch must match exactly; a destination that already exists is refused " +
			"rather than overwritten.",
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
			FilePath:  file.Path,
			Hunks:     file.Hunks,
			Created:   file.Created,
			Deleted:   file.Deleted,
			MovedFrom: file.MovedFrom,
		}
	}
	body, err := json.Marshal(ApplyPatchResponse{Files: files, Hunks: res.Hunks})
	if err != nil {
		return "", fmt.Errorf("fs.apply_patch: marshal: %w", err)
	}
	return string(body), nil
}

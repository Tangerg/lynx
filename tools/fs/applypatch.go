package fs

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Tangerg/lynx/core/chat"
	toolcontract "github.com/Tangerg/lynx/core/tool"
)

// ApplyPatchRequest applies a standard unified diff. The local executor
// supports create, modify, delete and move (headers naming two different
// paths), which makes a coordinated refactor one call.
type ApplyPatchRequest struct {
	Patch string `json:"patch" jsonschema:"minLength=1" jsonschema_description:"Standard unified diff. Supports create, modify, delete, and move operations."`
}

// ApplyPatchResponse is the LLM-facing return shape.
type ApplyPatchResponse struct {
	Files []PatchFileResponse `json:"files"`
	Hunks int                 `json:"hunks"`
}

// PatchFileResponse reports one patched file.
type PatchFileResponse struct {
	// Path is where the file ended up.
	Path    string `json:"path"`
	Hunks   int    `json:"hunks"`
	Created bool   `json:"created,omitempty"`
	Deleted bool   `json:"deleted,omitempty"`
	// MovedFrom is the path the file left, set only for a move. Path alone would
	// say a file exists somewhere new without saying which one stopped existing.
	MovedFrom string `json:"moved_from,omitempty"`
}

var _ toolcontract.Tool = (*ApplyPatchTool)(nil)

// ApplyPatchTool is the thin LLM-facing adapter for [Executor.ApplyPatch].
type ApplyPatchTool struct {
	executor Executor
	typed    toolcontract.Func[ApplyPatchRequest, ApplyPatchResponse]
}

// NewApplyPatchTool builds an [ApplyPatchTool] backed by executor. Passing nil
// wires up an unconfined [LocalExecutor] (workspace root "").
func NewApplyPatchTool(executor Executor) *ApplyPatchTool {
	if executor == nil {
		executor = NewLocalExecutor("")
	}
	t := &ApplyPatchTool{executor: executor}
	t.typed = mustTypedTool(
		toolcontract.FuncConfig{
			Name: "apply_patch",
			Description: "Apply one standard unified diff across one or more files, including create, modify, delete, and move operations. " +
				"Read existing files before patching them. Group coordinated multi-file changes in one patch. " +
				"The patch must match exactly, and an existing move destination is never overwritten.",
		},
		t.apply,
	)
	return t
}

func (t *ApplyPatchTool) Definition() chat.ToolDefinition {
	return t.typed.Definition()
}

func (*ApplyPatchTool) MutationPaths(arguments string) ([]string, error) {
	var req ApplyPatchRequest
	if err := json.Unmarshal([]byte(arguments), &req); err != nil {
		return nil, err
	}
	return patchPaths(req.Patch)
}

func (t *ApplyPatchTool) Call(ctx context.Context, arguments string) (string, error) {
	return t.typed.Call(ctx, arguments)
}

func (t *ApplyPatchTool) apply(ctx context.Context, req ApplyPatchRequest) (ApplyPatchResponse, error) {
	res, err := t.executor.ApplyPatch(ctx, req)
	if err != nil {
		return ApplyPatchResponse{}, fmt.Errorf("fs.apply_patch: %w", err)
	}
	return res, nil
}

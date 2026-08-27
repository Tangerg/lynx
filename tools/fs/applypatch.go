package fs

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Tangerg/scope/core/chat"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

// ApplyPatchRequest applies a Git-compatible unified diff. The local executor
// supports create, modify, delete, and Git rename patches, which makes a
// coordinated refactor one call.
type ApplyPatchRequest struct {
	Patch string `json:"patch" jsonschema:"minLength=1" jsonschema_description:"Git-compatible unified diff. Supports create, modify, delete, and rename operations; express moves with Git rename metadata."`
}

type ApplyPatchResponse struct {
	Files []PatchFileResponse `json:"files"`
	Hunks int                 `json:"hunks"`
}

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

type ApplyPatchTool struct {
	executor Executor
	typed    toolcontract.Func[ApplyPatchRequest, ApplyPatchResponse]
}

func NewApplyPatchTool(executor Executor) *ApplyPatchTool {
	if executor == nil {
		executor = NewLocalExecutor("")
	}
	t := &ApplyPatchTool{executor: executor}
	t.typed = mustTypedTool(
		toolcontract.FuncConfig{
			Name: "apply_patch",
			Description: "Apply one Git-compatible unified diff across one or more files, including create, modify, delete, and rename operations. " +
				"Read existing files before patching them. Group coordinated multi-file changes in one patch. " +
				"Express moves with Git rename metadata. The patch must match exactly, and an existing rename destination is never overwritten.",
		},
		t.apply,
	)
	return t
}

func (a *ApplyPatchTool) Definition() chat.ToolDefinition {
	return a.typed.Definition()
}

func (*ApplyPatchTool) MutationPaths(arguments string) ([]string, error) {
	var req ApplyPatchRequest
	if err := json.Unmarshal([]byte(arguments), &req); err != nil {
		return nil, err
	}
	return patchPaths(req.Patch)
}

func (a *ApplyPatchTool) Call(ctx context.Context, arguments string) (string, error) {
	return a.typed.Call(ctx, arguments)
}

func (a *ApplyPatchTool) apply(ctx context.Context, req ApplyPatchRequest) (ApplyPatchResponse, error) {
	res, err := a.executor.ApplyPatch(ctx, req)
	if err != nil {
		return ApplyPatchResponse{}, fmt.Errorf("fs.apply_patch: %w", err)
	}
	return res, nil
}

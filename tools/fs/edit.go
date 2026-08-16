package fs

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Tangerg/lynx/core/chat"
	toolcontract "github.com/Tangerg/lynx/tool"
)

// EditRequest is the LLM-facing argument shape for the edit tool.
type EditRequest struct {
	Path       string `json:"path" jsonschema:"minLength=1" jsonschema_description:"File path, absolute or relative to the workspace root."`
	OldString  string `json:"old_string" jsonschema:"required" jsonschema_description:"Exact text to find, copied verbatim from the file (the read tool returns raw text — there is no line-number prefix to strip). Keep it to the few unique lines needed; fails when the match is not unique unless replace_all=true."`
	NewString  string `json:"new_string" jsonschema:"required" jsonschema_description:"Replacement text. Preserve the surrounding indentation exactly. Must differ from old_string."`
	ReplaceAll bool   `json:"replace_all,omitempty" jsonschema_description:"Replace every occurrence. Default false. Use this for renaming a symbol across the file."`
}

// EditResponse is the LLM-facing return shape.
type EditResponse struct {
	Replacements int `json:"replacements"`
}

var _ toolcontract.Tool = (*EditTool)(nil)

// EditTool is the thin LLM-facing adapter for [Executor.Edit]. The
// match-and-replace logic lives in the executor so a backend upgrade
// can swap match policy without changing the tool.
type EditTool struct {
	executor Executor
	typed    *toolcontract.Func[EditRequest, EditResponse]
}

// NewEditTool builds an [EditTool] backed by executor. Passing nil
// wires up an unconfined [LocalExecutor] (workspace root "").
func NewEditTool(executor Executor) *EditTool {
	if executor == nil {
		executor = NewLocalExecutor("")
	}
	t := &EditTool{executor: executor}
	t.typed = mustTypedTool(
		toolcontract.FuncConfig{
			Name: "edit",
			Description: "Replace exact text in one file. Read the file before editing; an edit without a current read is refused. " +
				"Copy old_string verbatim from read output and keep it to the few unique lines needed. " +
				"Set replace_all=true only when every occurrence in this file should change.",
		},
		t.edit,
	)
	return t
}

func (t *EditTool) Definition() chat.ToolDefinition {
	return t.typed.Definition()
}

// ConcurrencyKey opts edit into concurrent execution keyed on its target file
// — the tool loop's optional concurrency contract (a tool reports per call
// whether it may overlap others and the resource it conflicts on). The loop
// parallelizes edits to DISTINCT files and serializes edits to the SAME file.
// An unparseable / empty path yields no key (no known conflict); the call still
// fails its own validation in Call.
func (t *EditTool) ConcurrencyKey(arguments string) (key string, concurrent bool) {
	var req EditRequest
	_ = json.Unmarshal([]byte(arguments), &req)
	return req.Path, true
}

// MutationPaths reports the file targeted by this call.
func (*EditTool) MutationPaths(arguments string) ([]string, error) {
	var req EditRequest
	if err := json.Unmarshal([]byte(arguments), &req); err != nil {
		return nil, err
	}
	if req.Path == "" {
		return nil, nil
	}
	return []string{req.Path}, nil
}

func (t *EditTool) Call(ctx context.Context, arguments string) (string, error) {
	return t.typed.Call(ctx, arguments)
}

func (t *EditTool) edit(ctx context.Context, req EditRequest) (EditResponse, error) {
	res, err := t.executor.Edit(ctx, EditInput(req))
	if err != nil {
		return EditResponse{}, fmt.Errorf("fs.edit: %w", err)
	}
	return EditResponse(res), nil
}

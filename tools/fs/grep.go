package fs

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Tangerg/lynx/core/chat"
	pkgjson "github.com/Tangerg/lynx/pkg/json"
	toolcontract "github.com/Tangerg/lynx/tools"
)

// GrepRequest is the LLM-facing argument shape for the grep tool.
//
// Notes on pattern syntax: the underlying engine is ripgrep (when
// available, falling back to GNU grep). That means literal braces /
// brackets need escaping (`interface\{\}` to find `interface{}`).
// By default patterns match within a single line; set `multiline=true`
// for patterns that span newlines.
type GrepRequest struct {
	Pattern string `json:"pattern" jsonschema:"required" jsonschema_description:"Regular expression pattern (ripgrep syntax — escape literal braces / parens)."`
	Path    string `json:"path,omitempty" jsonschema_description:"File or directory to search. Defaults to the workspace root."`
	Glob    string `json:"glob,omitempty" jsonschema_description:"Optional file filter glob, e.g. \"**/*.go\"."`
	Type    string `json:"type,omitempty" jsonschema_description:"Optional ripgrep file-type filter, e.g. \"go\", \"ts\", \"rust\". Ignored when only GNU grep is available."`

	IgnoreCase bool `json:"ignore_case,omitempty" jsonschema_description:"Case-insensitive search. Default false."`
	Multiline  bool `json:"multiline,omitempty" jsonschema_description:"Allow patterns to span line breaks. Default false. Requires ripgrep."`

	Context       int `json:"context,omitempty" jsonschema_description:"Lines of context before AND after each match (symmetric shortcut)."`
	BeforeContext int `json:"before_context,omitempty" jsonschema_description:"Lines of context before each match. Overrides 'context' for before-side."`
	AfterContext  int `json:"after_context,omitempty" jsonschema_description:"Lines of context after each match. Overrides 'context' for after-side."`

	OutputMode string `json:"output_mode,omitempty" jsonschema_description:"\"content\" (default; matching lines), \"files_with_matches\" (only paths — saves a lot of context), or \"count\" (per-file match counts)."`

	HeadLimit int `json:"head_limit,omitempty" jsonschema_description:"Cap on the first N result entries (like head). 0 = use default cap (250)."`
}

// GrepResponse is the LLM-facing return shape. Exactly one of
// matches / files / counts is populated based on the request's
// output_mode.
type GrepResponse struct {
	Matches   []GrepMatch     `json:"matches,omitempty"`
	Files     []string        `json:"files,omitempty"`
	Counts    []GrepFileCount `json:"counts,omitempty"`
	Truncated bool            `json:"truncated,omitempty"`
}

var grepToolSchema, _ = pkgjson.StringDefSchemaOf(GrepRequest{})

var _ toolcontract.Tool = (*GrepTool)(nil)

// GrepTool is the thin LLM-facing adapter for [Executor.Grep].
type GrepTool struct {
	executor Executor
}

// NewGrepTool builds a [GrepTool] backed by executor. Passing nil
// wires up an unconfined [LocalExecutor] (workspace root "").
func NewGrepTool(executor Executor) *GrepTool {
	if executor == nil {
		executor = NewLocalExecutor("")
	}
	return &GrepTool{executor: executor}
}

func (t *GrepTool) Definition() chat.ToolDefinition {
	return chat.ToolDefinition{
		Name: "grep",
		Description: "Regex search across files. Always use this tool for content search; never invoke grep or ripgrep via the shell tool. " +
			"Pattern syntax follows ripgrep — escape literal braces (use `interface\\{\\}` to find `interface{}`). " +
			"By default patterns match within a single line; pass `multiline=true` for cross-line patterns like `struct \\{[\\s\\S]*?field`. " +
			"Use `output_mode=files_with_matches` when you only need the list of files containing the pattern — it returns far less data.",
		InputSchema: json.RawMessage(grepToolSchema),
	}
}

// ConcurrencyKey opts grep into parallel execution — a read-only content
// search has no conflict (the tool loop's optional concurrency contract).
func (t *GrepTool) ConcurrencyKey(string) (key string, concurrent bool) { return "", true }

func (t *GrepTool) Call(ctx context.Context, arguments string) (string, error) {
	_ = ctx
	var req GrepRequest
	if err := json.Unmarshal([]byte(arguments), &req); err != nil {
		return "", fmt.Errorf("fs.grep: parse arguments: %w", err)
	}
	if req.Pattern == "" {
		return "", fmt.Errorf("fs.grep: %w", ErrEmptyPattern)
	}
	res, err := t.executor.Grep(ctx, GrepInput{
		Pattern:       req.Pattern,
		Root:          req.Path,
		Glob:          req.Glob,
		FileType:      req.Type,
		IgnoreCase:    req.IgnoreCase,
		Multiline:     req.Multiline,
		Context:       req.Context,
		BeforeContext: req.BeforeContext,
		AfterContext:  req.AfterContext,
		OutputMode:    GrepOutputMode(req.OutputMode),
		MaxResults:    req.HeadLimit,
	})
	if err != nil {
		return "", fmt.Errorf("fs.grep: %w", err)
	}
	body, err := json.Marshal(GrepResponse(res))
	if err != nil {
		return "", fmt.Errorf("fs.grep: marshal: %w", err)
	}
	return string(body), nil
}

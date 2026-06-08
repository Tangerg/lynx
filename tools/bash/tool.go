package bash

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Tangerg/lynx/core/model/chat"
	pkgjson "github.com/Tangerg/lynx/pkg/json"
)

// Request is the LLM-facing argument shape. It is a strict subset of
// [Input] — environment, working directory, and streaming are
// executor-side concerns, not LLM knobs.
type Request struct {
	Command string `json:"command" jsonschema:"required" jsonschema_description:"Shell command line. Run by /bin/sh -c."`
	Timeout int    `json:"timeout,omitempty" jsonschema_description:"Optional timeout in milliseconds. Typical: 5000-60000. Hard cap recommended: 600000 (10 min). 0 = no timeout (subject to outer ctx)."`
}

// Response is the LLM-facing return shape. Stdout/stderr are strings
// (not []byte) because every consumer is a chat model.
type Response struct {
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	ExitCode int    `json:"exit_code"`
	Killed   bool   `json:"killed,omitempty"`
	Duration string `json:"duration"`
}

var toolSchema, _ = pkgjson.StringDefSchemaOf(Request{})

var _ chat.Tool = (*Tool)(nil)

// Tool runs a shell command via the supplied [Executor].
type Tool struct {
	executor Executor
}

// NewTool builds a [Tool] backed by executor. Passing nil wires up
// a default [LocalExecutor] so callers who just want "run on this
// host" don't have to construct one explicitly.
func NewTool(executor Executor) *Tool {
	if executor == nil {
		executor = NewLocalExecutor()
	}
	return &Tool{executor: executor}
}

func (t *Tool) Definition() chat.ToolDefinition {
	return chat.ToolDefinition{
		Name: "bash",
		Description: "Execute a shell command via /bin/sh -c. Returns stdout, stderr, exit code, and duration. " +
			"Avoid using `find`, `grep`, `cat`, `head`, `tail`, `sed`, `awk` here — use the dedicated `glob`, `grep`, `read`, `edit` tools instead. Reserve bash for shell-only operations (build commands, git, package managers, etc.). " +
			"Each invocation starts a fresh shell — `cd`, exported variables, and shell options do not persist between calls.",
		InputSchema: toolSchema,
	}
}

func (t *Tool) Metadata() chat.ToolMetadata { return chat.ToolMetadata{} }

func (t *Tool) Call(ctx context.Context, arguments string) (string, error) {
	_ = ctx
	var req Request
	if err := json.Unmarshal([]byte(arguments), &req); err != nil {
		return "", fmt.Errorf("bash.tool: parse arguments: %w", err)
	}
	res, err := t.executor.Run(ctx, Input{
		Cmd:     req.Command,
		Timeout: time.Duration(req.Timeout) * time.Millisecond,
	})
	if err != nil {
		return "", fmt.Errorf("bash.tool: run: %w", err)
	}
	body, err := json.Marshal(Response{
		Stdout:   string(res.Stdout),
		Stderr:   string(res.Stderr),
		ExitCode: res.ExitCode,
		Killed:   res.Killed,
		Duration: res.Duration.String(),
	})
	if err != nil {
		return "", fmt.Errorf("bash.tool: marshal: %w", err)
	}
	return string(body), nil
}

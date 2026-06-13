package lsptools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/lyra/internal/domain/codeintel"
	"github.com/Tangerg/lynx/lyra/internal/kernel/toolset/turnctx"
)

// Build exposes the code-intelligence service as the agent's
// language tools (definition / references / hover / symbols / diagnostics).
// The service is working-directory independent — it keys servers by
// workspace root internally — so these tools are built ONCE and each reads
// the turn's cwd off the process blackboard at call time (the same
// per-session-cwd seam as fs / bash). Positions are 1-based at the tool
// boundary (what a human/LLM reads off a file); the service converts to the
// LSP 0-based wire form and folds unsupported file types into a plain reply.
func Build(ci *codeintel.Service, defaultWorkdir string) []chat.Tool {
	return []chat.Tool{
		newLSPPositionTool(
			"lsp_definition",
			"Find where the symbol at a file position is declared/defined. Returns the declaration location(s) as file:line:col.",
			ci.Definition,
			defaultWorkdir,
		),
		newLSPPositionTool(
			"lsp_references",
			"Find all references to the symbol at a file position (including its declaration). Returns locations as file:line:col.",
			ci.References,
			defaultWorkdir,
		),
		newLSPPositionTool(
			"lsp_hover",
			"Get hover information (type signature, documentation) for the symbol at a file position.",
			ci.Hover,
			defaultWorkdir,
		),
		newLSPFileTool(
			"lsp_document_symbols",
			"List the symbols (functions, types, methods, variables) declared in a file.",
			ci.DocumentSymbols,
			defaultWorkdir,
		),
		newLSPFileTool(
			"lsp_diagnostics",
			"Get the language server's current problems (compile errors, warnings) for a file.",
			ci.Diagnostics,
			defaultWorkdir,
		),
		newLSPQueryTool(
			"lsp_workspace_symbols",
			"Search the whole workspace for symbols (functions, types) matching a query string.",
			ci.WorkspaceSymbols,
			defaultWorkdir,
		),
	}
}

// --- input shapes + schemas ---

type lspPositionInput struct {
	File   string `json:"file"`
	Line   int    `json:"line"`
	Column int    `json:"column"`
}

type lspFileInput struct {
	File string `json:"file"`
}

type lspQueryInput struct {
	Query string `json:"query"`
}

const lspPositionSchema = `{"type":"object","properties":{` +
	`"file":{"type":"string","description":"Path to the file, relative to the workspace root (or absolute)."},` +
	`"line":{"type":"integer","description":"1-based line number of the symbol."},` +
	`"column":{"type":"integer","description":"1-based column number of the symbol on that line."}` +
	`},"required":["file","line","column"]}`

const lspFileSchema = `{"type":"object","properties":{` +
	`"file":{"type":"string","description":"Path to the file, relative to the workspace root (or absolute)."}` +
	`},"required":["file"]}`

const lspQuerySchema = `{"type":"object","properties":{` +
	`"query":{"type":"string","description":"Symbol name or substring to search for across the workspace."}` +
	`},"required":["query"]}`

// newLSPPositionTool builds a tool that takes a (file, line, column) position.
// run receives the resolved workspace root and the 1-based position and
// returns model-facing text (it folds an unsupported file into a plain reply).
func newLSPPositionTool(name, desc string, run func(ctx context.Context, root, file string, line, column int) (string, error), defaultWorkdir string) chat.Tool {
	t, _ := chat.NewTool(
		chat.ToolDefinition{Name: name, Description: desc, InputSchema: lspPositionSchema},
		chat.ToolMetadata{},
		func(ctx context.Context, arguments string) (string, error) {
			var in lspPositionInput
			if err := json.Unmarshal([]byte(arguments), &in); err != nil {
				return "", fmt.Errorf("%s: invalid arguments: %w", name, err)
			}
			if in.File == "" {
				return "", fmt.Errorf("%s: file is required", name)
			}
			return run(ctx, turnctx.TurnCwd(ctx, defaultWorkdir), in.File, in.Line, in.Column)
		},
	)
	return t
}

// newLSPFileTool builds a tool that takes just a file.
func newLSPFileTool(name, desc string, run func(ctx context.Context, root, file string) (string, error), defaultWorkdir string) chat.Tool {
	t, _ := chat.NewTool(
		chat.ToolDefinition{Name: name, Description: desc, InputSchema: lspFileSchema},
		chat.ToolMetadata{},
		func(ctx context.Context, arguments string) (string, error) {
			var in lspFileInput
			if err := json.Unmarshal([]byte(arguments), &in); err != nil {
				return "", fmt.Errorf("%s: invalid arguments: %w", name, err)
			}
			if in.File == "" {
				return "", fmt.Errorf("%s: file is required", name)
			}
			return run(ctx, turnctx.TurnCwd(ctx, defaultWorkdir), in.File)
		},
	)
	return t
}

// newLSPQueryTool builds a tool that takes a workspace-wide query string.
func newLSPQueryTool(name, desc string, run func(ctx context.Context, root, query string) (string, error), defaultWorkdir string) chat.Tool {
	t, _ := chat.NewTool(
		chat.ToolDefinition{Name: name, Description: desc, InputSchema: lspQuerySchema},
		chat.ToolMetadata{},
		func(ctx context.Context, arguments string) (string, error) {
			var in lspQueryInput
			if err := json.Unmarshal([]byte(arguments), &in); err != nil {
				return "", fmt.Errorf("%s: invalid arguments: %w", name, err)
			}
			if in.Query == "" {
				return "", fmt.Errorf("%s: query is required", name)
			}
			return run(ctx, turnctx.TurnCwd(ctx, defaultWorkdir), in.Query)
		},
	)
	return t
}

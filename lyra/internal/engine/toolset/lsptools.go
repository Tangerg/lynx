package toolset

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/lyra/internal/service/codeintel"
)

// BuildLSPTools exposes the code-intelligence service as the agent's
// language tools (definition / references / hover / symbols / diagnostics).
// The service is working-directory independent — it keys servers by
// workspace root internally — so these tools are built ONCE and each reads
// the turn's cwd off the process blackboard at call time (the same
// per-session-cwd seam as fs / bash). Positions are 1-based at the tool
// boundary (what a human/LLM reads off a file); the service converts to the
// LSP 0-based wire form and folds unsupported file types into a plain reply.
func BuildLSPTools(ci *codeintel.Service, defaultWorkdir string) []chat.Tool {
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

// withEditDiagnostics wraps a file-mutating tool (write / edit) so a
// successful edit is immediately type-checked: the code-intelligence service
// re-analyzes the file and appends any problems the edit INTRODUCED to the
// tool result, so the model sees the breakage it just caused without a
// separate lsp_diagnostics call. The baseline-diff, staleness guard, and
// best-effort semantics live in [codeintel.Service.DiagnoseEdit]; here we
// only feed it the edit closure. root is the resolved workspace directory
// for this resolution; the wrapped tool's path argument is relative to it.
func withEditDiagnostics(inner chat.Tool, ci *codeintel.Service, root string) chat.Tool {
	if ci == nil {
		return inner
	}
	return wrapTool(inner, func(ctx context.Context, arguments string) (string, error) {
		var a struct {
			Path string `json:"path"`
		}
		_ = json.Unmarshal([]byte(arguments), &a)
		return ci.DiagnoseEdit(ctx, root, a.Path, func() (string, error) {
			return inner.Call(ctx, arguments)
		})
	})
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
			return run(ctx, TurnCwd(ctx, defaultWorkdir), in.File, in.Line, in.Column)
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
			return run(ctx, TurnCwd(ctx, defaultWorkdir), in.File)
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
			return run(ctx, TurnCwd(ctx, defaultWorkdir), in.Query)
		},
	)
	return t
}

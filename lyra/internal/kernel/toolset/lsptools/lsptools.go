package lsptools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/core/model/chat"

	"github.com/Tangerg/lynx/lyra/internal/domain/codeintel"
	"github.com/Tangerg/lynx/lyra/internal/kernel/toolset/turnctx"
)

// Build exposes the code-intelligence service as the agent's language tools: a
// single `lsp` tool whose `operation` selects the query (the shape opencode and
// claude_code converged on), plus a separate `lsp_diagnostics` (a whole-file
// problem list — a different interaction both peers also keep apart).
//
// The service is working-directory independent — it keys servers by workspace
// root internally — so these tools are built ONCE and read the turn's cwd off
// the process blackboard at call time (the per-session-cwd seam shared with
// fs / bash). Positions are 1-based at the tool boundary (what a human/LLM reads
// off a file); the service converts to the LSP 0-based wire form and folds an
// unsupported file type into a plain reply.
func Build(ci *codeintel.Service, defaultWorkdir string) []chat.Tool {
	return []chat.Tool{
		newLSPTool(ci, defaultWorkdir),
		newDiagnosticsTool(ci, defaultWorkdir),
	}
}

type lspInput struct {
	Operation string `json:"operation"`
	FilePath  string `json:"file_path"`
	Line      int    `json:"line"`
	Character int    `json:"character"`
	Query     string `json:"query"`
}

const lspSchema = `{"type":"object","properties":{` +
	`"operation":{"type":"string","enum":["definition","references","implementation","hover","incoming_calls","outgoing_calls","document_symbols","workspace_symbols"],"description":"Which language-server query to run."},` +
	`"file_path":{"type":"string","description":"File path, relative to the workspace root (or absolute). Required for every operation except workspace_symbols."},` +
	`"line":{"type":"integer","description":"1-based line of the symbol. Required for definition/references/implementation/hover/incoming_calls/outgoing_calls."},` +
	`"character":{"type":"integer","description":"1-based character (column) of the symbol on that line. Required with line."},` +
	`"query":{"type":"string","description":"Symbol name or substring to search for. Required for workspace_symbols."}` +
	`},"required":["operation"]}`

const lspDesc = "Query the language server (LSP) about code at a position or across the workspace. " +
	"operation selects: definition (where a symbol is declared) · references (all use sites) · " +
	"implementation (concrete implementations of an interface / abstract method) · hover (type signature + docs) · " +
	"incoming_calls (callers of the function at the position) · outgoing_calls (functions the one at the position calls) · " +
	"document_symbols (symbols declared in a file) · workspace_symbols (search symbols across the workspace by name). " +
	"Position operations need file_path + line + character (1-based); document_symbols needs file_path; workspace_symbols needs query. " +
	"(For a file's compile errors / warnings use lsp_diagnostics.)"

func newLSPTool(ci *codeintel.Service, defaultWorkdir string) chat.Tool {
	t, _ := chat.NewTool(
		chat.ToolDefinition{Name: "lsp", Description: lspDesc, InputSchema: lspSchema},
		chat.ToolMetadata{},
		func(ctx context.Context, arguments string) (string, error) {
			var in lspInput
			if err := json.Unmarshal([]byte(arguments), &in); err != nil {
				return "", fmt.Errorf("lsp: invalid arguments: %w", err)
			}

			// Validate the operand each operation needs before dispatch.
			switch in.Operation {
			case "definition", "references", "implementation", "hover",
				"incoming_calls", "outgoing_calls", "document_symbols":
				if in.FilePath == "" {
					return "", fmt.Errorf("lsp %s: file_path is required", in.Operation)
				}
			case "workspace_symbols":
				if in.Query == "" {
					return "", errors.New("lsp workspace_symbols: query is required")
				}
			default:
				return "", fmt.Errorf("lsp: unknown operation %q", in.Operation)
			}

			root := turnctx.TurnCwd(ctx, defaultWorkdir)
			switch in.Operation {
			case "definition":
				return ci.Definition(ctx, root, in.FilePath, in.Line, in.Character)
			case "references":
				return ci.References(ctx, root, in.FilePath, in.Line, in.Character)
			case "implementation":
				return ci.Implementation(ctx, root, in.FilePath, in.Line, in.Character)
			case "hover":
				return ci.Hover(ctx, root, in.FilePath, in.Line, in.Character)
			case "incoming_calls":
				return ci.IncomingCalls(ctx, root, in.FilePath, in.Line, in.Character)
			case "outgoing_calls":
				return ci.OutgoingCalls(ctx, root, in.FilePath, in.Line, in.Character)
			case "document_symbols":
				return ci.DocumentSymbols(ctx, root, in.FilePath)
			default: // workspace_symbols (validated above)
				return ci.WorkspaceSymbols(ctx, root, in.Query)
			}
		},
	)
	return t
}

// newDiagnosticsTool exposes lsp_diagnostics — a file's current problems. Kept
// separate from the `lsp` query tool (as opencode / claude_code do): it's a
// whole-file problem list, not a position/symbol query, and the same engine
// auto-appends post-edit diagnostics on writes.
func newDiagnosticsTool(ci *codeintel.Service, defaultWorkdir string) chat.Tool {
	const schema = `{"type":"object","properties":{` +
		`"file_path":{"type":"string","description":"Path to the file, relative to the workspace root (or absolute)."}` +
		`},"required":["file_path"]}`
	t, _ := chat.NewTool(
		chat.ToolDefinition{
			Name:        "lsp_diagnostics",
			Description: "Get the language server's current problems (compile errors, warnings) for a file.",
			InputSchema: schema,
		},
		chat.ToolMetadata{},
		func(ctx context.Context, arguments string) (string, error) {
			var in struct {
				FilePath string `json:"file_path"`
			}
			if err := json.Unmarshal([]byte(arguments), &in); err != nil {
				return "", fmt.Errorf("lsp_diagnostics: invalid arguments: %w", err)
			}
			if in.FilePath == "" {
				return "", errors.New("lsp_diagnostics: file_path is required")
			}
			return ci.Diagnostics(ctx, turnctx.TurnCwd(ctx, defaultWorkdir), in.FilePath)
		},
	)
	return t
}

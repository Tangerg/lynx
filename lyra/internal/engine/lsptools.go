package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/lyra/internal/lsp"
)

// buildLSPTools wraps the shared LSP manager as the agent's code-intelligence
// tools (definition / references / hover / symbols / diagnostics). The manager
// is working-directory independent — it keys servers by workspace root
// internally — so these tools are built ONCE and each reads the turn's cwd off
// the process blackboard at call time (the same per-session-cwd seam as fs /
// bash). Positions are 1-based at the tool boundary (what a human/LLM reads
// off a file) and converted to the LSP 0-based wire form internally.
func buildLSPTools(mgr *lsp.Manager, defaultWorkdir string) []chat.Tool {
	return []chat.Tool{
		newLSPPositionTool(
			"lsp_definition",
			"Find where the symbol at a file position is declared/defined. Returns the declaration location(s) as file:line:col.",
			func(ctx context.Context, root, file string, pos lsp.Position) (string, error) {
				locs, err := mgr.Definition(ctx, root, file, pos)
				if err != nil {
					return "", err
				}
				return formatLocations(root, locs, "definition"), nil
			},
			defaultWorkdir,
		),
		newLSPPositionTool(
			"lsp_references",
			"Find all references to the symbol at a file position (including its declaration). Returns locations as file:line:col.",
			func(ctx context.Context, root, file string, pos lsp.Position) (string, error) {
				locs, err := mgr.References(ctx, root, file, pos)
				if err != nil {
					return "", err
				}
				return formatLocations(root, locs, "reference"), nil
			},
			defaultWorkdir,
		),
		newLSPPositionTool(
			"lsp_hover",
			"Get hover information (type signature, documentation) for the symbol at a file position.",
			func(ctx context.Context, root, file string, pos lsp.Position) (string, error) {
				text, err := mgr.Hover(ctx, root, file, pos)
				if err != nil {
					return "", err
				}
				if text == "" {
					return "No hover information available at that position.", nil
				}
				return text, nil
			},
			defaultWorkdir,
		),
		newLSPFileTool(
			"lsp_document_symbols",
			"List the symbols (functions, types, methods, variables) declared in a file.",
			func(ctx context.Context, root, file string) (string, error) {
				syms, err := mgr.DocumentSymbols(ctx, root, file)
				if err != nil {
					return "", err
				}
				return formatSymbols(root, syms), nil
			},
			defaultWorkdir,
		),
		newLSPFileTool(
			"lsp_diagnostics",
			"Get the language server's current problems (compile errors, warnings) for a file.",
			func(ctx context.Context, root, file string) (string, error) {
				diags, err := mgr.Diagnostics(ctx, root, file)
				if err != nil {
					return "", err
				}
				return formatDiagnostics(root, file, diags), nil
			},
			defaultWorkdir,
		),
		newLSPQueryTool(
			"lsp_workspace_symbols",
			"Search the whole workspace for symbols (functions, types) matching a query string.",
			func(ctx context.Context, root, query string) (string, error) {
				syms, err := mgr.WorkspaceSymbols(ctx, root, query)
				if err != nil {
					return "", err
				}
				return formatSymbols(root, syms), nil
			},
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
// run receives the resolved workspace root and the 0-based LSP position.
func newLSPPositionTool(name, desc string, run func(ctx context.Context, root, file string, pos lsp.Position) (string, error), defaultWorkdir string) chat.Tool {
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
			out, err := run(ctx, turnCwd(ctx, defaultWorkdir), in.File, toPosition(in.Line, in.Column))
			return lspResult(out, err)
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
			out, err := run(ctx, turnCwd(ctx, defaultWorkdir), in.File)
			return lspResult(out, err)
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
			out, err := run(ctx, turnCwd(ctx, defaultWorkdir), in.Query)
			return lspResult(out, err)
		},
	)
	return t
}

// lspResult folds an expected "no server for this language" outcome into a
// plain result string (the model adapts) and passes any other error through.
func lspResult(out string, err error) (string, error) {
	if errors.Is(err, lsp.ErrNoServer) {
		return "No language server is available for that file type.", nil
	}
	if err != nil {
		return "", err
	}
	return out, nil
}

// toPosition converts a 1-based (line, column) — what the model reads off a
// file — to the 0-based LSP wire position, clamping at the document origin.
func toPosition(line, column int) lsp.Position {
	if line < 1 {
		line = 1
	}
	if column < 1 {
		column = 1
	}
	return lsp.Position{Line: line - 1, Character: column - 1}
}

// --- result formatting (1-based, workspace-relative paths) ---

func relPath(root, abs string) string {
	if rel, err := filepath.Rel(root, abs); err == nil && !strings.HasPrefix(rel, "..") {
		return rel
	}
	return abs
}

func formatLocations(root string, locs []lsp.Location, kind string) string {
	if len(locs) == 0 {
		return fmt.Sprintf("No %s found.", kind)
	}
	var b strings.Builder
	for _, l := range locs {
		// LSP ranges are 0-based; present 1-based.
		fmt.Fprintf(&b, "%s:%d:%d\n", relPath(root, l.Path()), l.Range.Start.Line+1, l.Range.Start.Character+1)
	}
	return strings.TrimRight(b.String(), "\n")
}

func formatSymbols(root string, syms []lsp.Symbol) string {
	if len(syms) == 0 {
		return "No symbols found."
	}
	var b strings.Builder
	for _, s := range syms {
		fmt.Fprintf(&b, "%s %s", symbolKindName(s.Kind), s.Name)
		if s.Container != "" {
			fmt.Fprintf(&b, " (in %s)", s.Container)
		}
		fmt.Fprintf(&b, " — %s:%d:%d\n", relPath(root, s.Location.Path()), s.Location.Range.Start.Line+1, s.Location.Range.Start.Character+1)
	}
	return strings.TrimRight(b.String(), "\n")
}

// formatDiagnostics echoes the caller's file path (relative or absolute, as
// the model passed it) in each line, so root is unused here.
func formatDiagnostics(_, file string, diags []lsp.Diagnostic) string {
	if len(diags) == 0 {
		return fmt.Sprintf("No diagnostics for %s.", file)
	}
	var b strings.Builder
	for _, d := range diags {
		sev := d.SeverityName()
		if sev == "" {
			sev = "note"
		}
		fmt.Fprintf(&b, "%s %s:%d:%d: %s", sev, file, d.Range.Start.Line+1, d.Range.Start.Character+1, d.Message)
		if d.Source != "" {
			fmt.Fprintf(&b, " [%s]", d.Source)
		}
		b.WriteByte('\n')
	}
	return strings.TrimRight(b.String(), "\n")
}

// symbolKindName maps the LSP SymbolKind enum (1..26) to a readable label.
func symbolKindName(kind int) string {
	switch kind {
	case 1:
		return "file"
	case 2:
		return "module"
	case 3:
		return "namespace"
	case 4:
		return "package"
	case 5:
		return "class"
	case 6:
		return "method"
	case 7:
		return "property"
	case 8:
		return "field"
	case 9:
		return "constructor"
	case 10:
		return "enum"
	case 11:
		return "interface"
	case 12:
		return "function"
	case 13:
		return "variable"
	case 14:
		return "constant"
	case 15:
		return "string"
	case 16:
		return "number"
	case 17:
		return "boolean"
	case 18:
		return "array"
	case 19:
		return "object"
	case 20:
		return "key"
	case 21:
		return "null"
	case 22:
		return "enum-member"
	case 23:
		return "struct"
	case 24:
		return "event"
	case 25:
		return "operator"
	case 26:
		return "type-parameter"
	default:
		return "symbol"
	}
}

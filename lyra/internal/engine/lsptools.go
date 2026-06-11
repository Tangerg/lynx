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

// withEditDiagnostics wraps a file-mutating tool (write / edit) so a
// successful edit is immediately type-checked: the language server re-analyzes
// the file and any problems the edit INTRODUCED are appended to the tool
// result, so the model sees the breakage it just caused without a separate
// lsp_diagnostics call (the highest-value LSP integration in mature agents).
//
// It reports only NEW problems — a baseline of the file's diagnostics is taken
// BEFORE the edit and subtracted from the post-edit set. This is the guard
// against the language server's caching/staleness causing false positives: if
// the server returns a cached (not-yet-reanalyzed) result, after == before and
// the diff is empty, so nothing is reported. Pre-existing problems the edit
// didn't touch are likewise filtered out. The diff is position-independent
// (keyed by severity/source/code/message) so a pre-existing problem that
// merely shifted lines isn't mistaken for a new one. The bias is deliberately
// toward under-reporting: never blame an edit for a problem it didn't cause.
//
// Best-effort throughout: a failed edit is passed through untouched, an
// unsupported file type is skipped (no server spawned), and any language-server
// trouble leaves the original result intact — an edit never fails because of
// LSP. root is the resolved workspace directory for this resolution; the
// wrapped tool's path argument is relative to it.
func withEditDiagnostics(inner chat.Tool, mgr *lsp.Manager, root string) chat.Tool {
	if mgr == nil {
		return inner
	}
	return wrapTool(inner, func(ctx context.Context, arguments string) (string, error) {
		var a struct {
			Path string `json:"path"`
		}
		_ = json.Unmarshal([]byte(arguments), &a)
		check := a.Path != "" && mgr.Supported(a.Path)

		// Baseline BEFORE the edit (best effort: a brand-new file has none).
		var baseline []lsp.Diagnostic
		if check {
			baseline, _ = mgr.Diagnostics(ctx, root, a.Path)
		}

		out, err := inner.Call(ctx, arguments)
		if err != nil || !check {
			return out, err // edit failed (nothing to diagnose) or unsupported
		}

		after, derr := mgr.Diagnostics(ctx, root, a.Path)
		if derr != nil {
			return out, nil // never fail an edit on language-server trouble
		}
		section := diagnosticsSection(a.Path, newProblems(baseline, after))
		if section == "" {
			return out, nil
		}
		return out + "\n\n" + section, nil
	})
}

// newProblems returns the diagnostics in after that aren't in before, matched
// as a multiset on a position-independent key — so a pre-existing problem
// whose line merely shifted (or that the server re-reported from cache) is not
// counted as introduced.
func newProblems(before, after []lsp.Diagnostic) []lsp.Diagnostic {
	seen := make(map[string]int, len(before))
	for _, d := range before {
		seen[diagKey(d)]++
	}
	var out []lsp.Diagnostic
	for _, d := range after {
		k := diagKey(d)
		if seen[k] > 0 {
			seen[k]-- // consume one baseline occurrence
			continue
		}
		out = append(out, d)
	}
	return out
}

// diagKey identifies a diagnostic independently of its position, so line shifts
// from the edit don't turn a pre-existing problem into a "new" one.
func diagKey(d lsp.Diagnostic) string {
	return fmt.Sprintf("%d\x00%s\x00%v\x00%s", d.Severity, d.Source, d.Code, d.Message)
}

// diagnosticsSection renders the errors and warnings (info/hint are dropped as
// noise) for a just-edited file, or "" when there are none.
func diagnosticsSection(file string, diags []lsp.Diagnostic) string {
	var lines []string
	for _, d := range diags {
		if d.Severity > 2 { // 1=error 2=warning; skip 3=info 4=hint
			continue
		}
		sev := d.SeverityName()
		if sev == "" {
			sev = "error" // unset severity → treat as error per the LSP default
		}
		line := fmt.Sprintf("%s %s:%d:%d: %s", sev, file, d.Range.Start.Line+1, d.Range.Start.Character+1, d.Message)
		if d.Source != "" {
			line += fmt.Sprintf(" [%s]", d.Source)
		}
		lines = append(lines, line)
	}
	if len(lines) == 0 {
		return ""
	}
	return fmt.Sprintf("Language server flagged %d new problem(s) in %s after this edit:\n%s", len(lines), file, strings.Join(lines, "\n"))
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
		if s.Detail != "" {
			fmt.Fprintf(&b, " %s", s.Detail) // signature / type the server attached
		}
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

// symbolKindNames maps the LSP SymbolKind enum (1..26) to a readable label.
var symbolKindNames = map[int]string{
	1: "file", 2: "module", 3: "namespace", 4: "package", 5: "class",
	6: "method", 7: "property", 8: "field", 9: "constructor", 10: "enum",
	11: "interface", 12: "function", 13: "variable", 14: "constant", 15: "string",
	16: "number", 17: "boolean", 18: "array", 19: "object", 20: "key",
	21: "null", 22: "enum-member", 23: "struct", 24: "event", 25: "operator",
	26: "type-parameter",
}

func symbolKindName(kind int) string {
	if name, ok := symbolKindNames[kind]; ok {
		return name
	}
	return "symbol"
}

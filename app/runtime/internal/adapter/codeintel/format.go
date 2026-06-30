package codeintel

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Tangerg/lynx/app/runtime/internal/infra/lsp"
)

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

// formatCalls renders call-hierarchy results (the callers/callees, each a
// symbol) like [formatSymbols], with a call-direction-specific empty message.
func formatCalls(root string, syms []lsp.Symbol, kind string) string {
	if len(syms) == 0 {
		return fmt.Sprintf("No %s found.", kind)
	}
	return formatSymbols(root, syms)
}

// formatDiagnostics echoes the caller's file path (relative or absolute, as
// the model passed it) in each line.
func formatDiagnostics(file string, diags []lsp.Diagnostic) string {
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

// LSP exposes language-server capabilities as one model tool.
package builtin

import (
	"context"
	"errors"
	"fmt"
	"strings"

	toolcontract "github.com/Tangerg/lynx/core/tool"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/codeintel"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/executionctx"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
)

// BuildLSP exposes the code-intelligence analyzer as one `lsp` tool whose
// operation selects the query. Keeping diagnostics in the same operation
// vocabulary avoids two model-visible names for one language-server capability.
//
// The analyzer is working-directory independent — it keys servers by workspace
// root internally — so these tools are built ONCE and read the Run's cwd off
// application context at call time (the per-session-cwd seam shared with fs /
// shell). Positions are 1-based at the tool boundary (what a human/LLM reads
// off a file); the analyzer converts to the LSP 0-based wire form and folds an
// unsupported file type into a plain reply.
func BuildLSP(ci *codeintel.Analyzer, defaultCWD string) ([]toolcontract.Tool, error) {
	if ci == nil {
		return nil, errors.New("lsp: analyzer is nil")
	}
	lsp, err := newQuery(ci, defaultCWD)
	if err != nil {
		return nil, err
	}
	return []toolcontract.Tool{lsp}, nil
}

// lspInput is the model-facing argument shape; [toolcontract.NewFunc] derives the
// JSON schema from it and decodes calls back into it, so the advertised schema
// and parsed value cannot drift. Only `operation` is structurally required —
// which operand each operation needs is validated per-operation in the handler.
type lspInput struct {
	Operation string `json:"operation" jsonschema:"enum=definition,enum=references,enum=implementation,enum=hover,enum=incoming_calls,enum=outgoing_calls,enum=document_symbols,enum=workspace_symbols,enum=diagnostics" jsonschema_description:"Language-server query to run."`
	Path      string `json:"path,omitempty" jsonschema_description:"File path, absolute or relative to the workspace root. Required except for workspace_symbols."`
	Line      int    `json:"line,omitempty" jsonschema:"minimum=1" jsonschema_description:"1-based line of the symbol. Required for position operations."`
	Character int    `json:"character,omitempty" jsonschema:"minimum=1" jsonschema_description:"1-based character (column) of the symbol. Required for position operations."`
	Query     string `json:"query,omitempty" jsonschema_description:"Symbol name or substring to search for. Required for workspace_symbols."`
}

func (l lspInput) validate() error {
	switch l.Operation {
	case "definition", "references", "implementation", "hover",
		"incoming_calls", "outgoing_calls":
		if strings.TrimSpace(l.Path) == "" {
			return fmt.Errorf("lsp %s: path is required", l.Operation)
		}
		if l.Line < 1 || l.Character < 1 {
			return fmt.Errorf("lsp %s: line and character must both be at least 1", l.Operation)
		}
		if strings.TrimSpace(l.Query) != "" {
			return fmt.Errorf("lsp %s: query is not used for position operations", l.Operation)
		}
	case "document_symbols", "diagnostics":
		if strings.TrimSpace(l.Path) == "" {
			return fmt.Errorf("lsp %s: path is required", l.Operation)
		}
		if l.Line != 0 || l.Character != 0 || strings.TrimSpace(l.Query) != "" {
			return fmt.Errorf("lsp %s: only path is accepted", l.Operation)
		}
	case "workspace_symbols":
		if strings.TrimSpace(l.Query) == "" {
			return errors.New("lsp workspace_symbols: query is required")
		}
		if strings.TrimSpace(l.Path) != "" || l.Line != 0 || l.Character != 0 {
			return errors.New("lsp workspace_symbols: only query is accepted")
		}
	default:
		return fmt.Errorf("lsp: unknown operation %q", l.Operation)
	}
	return nil
}

const lspDesc = "Query the language server (LSP) about code at a position or across the workspace. " +
	"Use definition, references, implementation, hover, incoming_calls, or outgoing_calls with path + 1-based line + character; " +
	"document_symbols or diagnostics with path; workspace_symbols with query. " +
	"diagnostics returns the current compile errors and warnings for one file."

type lspRunner struct {
	analyzer   *codeintel.Analyzer
	defaultCWD string
}

func newQuery(ci *codeintel.Analyzer, defaultCWD string) (toolcontract.Tool, error) {
	t := &lspRunner{analyzer: ci, defaultCWD: defaultCWD}
	return toolcontract.NewFunc[lspInput, string](
		toolcontract.FuncConfig{Name: tool.LSP, Description: lspDesc},
		t.query,
	)
}

func (l *lspRunner) query(ctx context.Context, in lspInput) (string, error) {
	if err := in.validate(); err != nil {
		return "", err
	}
	root := executionctx.CWD(ctx, l.defaultCWD)
	switch in.Operation {
	case "definition":
		return l.analyzer.Definition(ctx, root, in.Path, in.Line, in.Character)
	case "references":
		return l.analyzer.References(ctx, root, in.Path, in.Line, in.Character)
	case "implementation":
		return l.analyzer.Implementation(ctx, root, in.Path, in.Line, in.Character)
	case "hover":
		return l.analyzer.Hover(ctx, root, in.Path, in.Line, in.Character)
	case "incoming_calls":
		return l.analyzer.IncomingCalls(ctx, root, in.Path, in.Line, in.Character)
	case "outgoing_calls":
		return l.analyzer.OutgoingCalls(ctx, root, in.Path, in.Line, in.Character)
	case "document_symbols":
		return l.analyzer.DocumentSymbols(ctx, root, in.Path)
	case "diagnostics":
		return l.analyzer.Diagnostics(ctx, root, in.Path)
	default:
		return l.analyzer.WorkspaceSymbols(ctx, root, in.Query)
	}
}

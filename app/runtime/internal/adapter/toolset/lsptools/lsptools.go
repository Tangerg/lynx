package lsptools

import (
	"context"
	"errors"
	"fmt"
	"strings"

	toolcontract "github.com/Tangerg/lynx/tool"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/codeintel"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/executionctx"
)

// Build exposes the code-intelligence analyzer as one `lsp` tool whose
// operation selects the query. Keeping diagnostics in the same operation
// vocabulary avoids two model-visible names for one language-server capability.
//
// The analyzer is working-directory independent — it keys servers by workspace
// root internally — so these tools are built ONCE and read the turn's cwd off
// application context at call time (the per-session-cwd seam shared with fs /
// shell). Positions are 1-based at the tool boundary (what a human/LLM reads
// off a file); the analyzer converts to the LSP 0-based wire form and folds an
// unsupported file type into a plain reply.
func Build(ci *codeintel.Analyzer, defaultWorkdir string) ([]toolcontract.Tool, error) {
	if ci == nil {
		return nil, errors.New("lsptools: analyzer is nil")
	}
	lsp, err := newLSPTool(ci, defaultWorkdir)
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

func (in lspInput) validate() error {
	switch in.Operation {
	case "definition", "references", "implementation", "hover",
		"incoming_calls", "outgoing_calls":
		if strings.TrimSpace(in.Path) == "" {
			return fmt.Errorf("lsp %s: path is required", in.Operation)
		}
		if in.Line < 1 || in.Character < 1 {
			return fmt.Errorf("lsp %s: line and character must both be at least 1", in.Operation)
		}
		if strings.TrimSpace(in.Query) != "" {
			return fmt.Errorf("lsp %s: query is not used for position operations", in.Operation)
		}
	case "document_symbols", "diagnostics":
		if strings.TrimSpace(in.Path) == "" {
			return fmt.Errorf("lsp %s: path is required", in.Operation)
		}
		if in.Line != 0 || in.Character != 0 || strings.TrimSpace(in.Query) != "" {
			return fmt.Errorf("lsp %s: only path is accepted", in.Operation)
		}
	case "workspace_symbols":
		if strings.TrimSpace(in.Query) == "" {
			return errors.New("lsp workspace_symbols: query is required")
		}
		if strings.TrimSpace(in.Path) != "" || in.Line != 0 || in.Character != 0 {
			return errors.New("lsp workspace_symbols: only query is accepted")
		}
	default:
		return fmt.Errorf("lsp: unknown operation %q", in.Operation)
	}
	return nil
}

const lspDesc = "Query the language server (LSP) about code at a position or across the workspace. " +
	"Use definition, references, implementation, hover, incoming_calls, or outgoing_calls with path + 1-based line + character; " +
	"document_symbols or diagnostics with path; workspace_symbols with query. " +
	"diagnostics returns the current compile errors and warnings for one file."

type lspRunner struct {
	analyzer       *codeintel.Analyzer
	defaultWorkdir string
}

func newLSPTool(ci *codeintel.Analyzer, defaultWorkdir string) (toolcontract.Tool, error) {
	t := &lspRunner{analyzer: ci, defaultWorkdir: defaultWorkdir}
	return toolcontract.NewFunc[lspInput, string](
		toolcontract.FuncConfig{Name: "lsp", Description: lspDesc},
		t.query,
	)
}

func (t *lspRunner) query(ctx context.Context, in lspInput) (string, error) {
	if err := in.validate(); err != nil {
		return "", err
	}
	root := executionctx.CWD(ctx, t.defaultWorkdir)
	switch in.Operation {
	case "definition":
		return t.analyzer.Definition(ctx, root, in.Path, in.Line, in.Character)
	case "references":
		return t.analyzer.References(ctx, root, in.Path, in.Line, in.Character)
	case "implementation":
		return t.analyzer.Implementation(ctx, root, in.Path, in.Line, in.Character)
	case "hover":
		return t.analyzer.Hover(ctx, root, in.Path, in.Line, in.Character)
	case "incoming_calls":
		return t.analyzer.IncomingCalls(ctx, root, in.Path, in.Line, in.Character)
	case "outgoing_calls":
		return t.analyzer.OutgoingCalls(ctx, root, in.Path, in.Line, in.Character)
	case "document_symbols":
		return t.analyzer.DocumentSymbols(ctx, root, in.Path)
	case "diagnostics":
		return t.analyzer.Diagnostics(ctx, root, in.Path)
	default:
		return t.analyzer.WorkspaceSymbols(ctx, root, in.Query)
	}
}

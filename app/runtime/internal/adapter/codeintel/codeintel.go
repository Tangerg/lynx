// Package codeintel adapts the LSP manager (infra/lsp) into model-facing code
// intelligence: root-relative, 1-based locations; plain messages for unsupported
// file types; and new-problems-after-edit diagnostics.
//
// It is the single owner of the LSP protocol types at the tool adapter boundary:
// tool assembly builds lsp_* tools and edit diagnostics over this service rather
// than importing infra/lsp directly.
package codeintel

import (
	"context"
	"errors"

	"github.com/Tangerg/lynx/app/runtime/internal/infra/lsp"
)

// ServerSpec is the language-server table entry, re-exported so callers
// configure code intelligence without importing the infra package. The
// yaml/config layer unmarshals into the identical underlying type.
type ServerSpec = lsp.ServerSpec

// noServerMsg is returned (with a nil error) when no language server
// handles the target file type, so an unsupported file informs the model
// instead of halting the tool loop.
const noServerMsg = "No language server is available for that file type."

// Service wraps the LSP manager as the code-intelligence adapter surface.
// Servers launch lazily per (workspace root, language) inside the manager;
// Close shuts them all down.
type Service struct {
	mgr *lsp.Manager
}

// New builds a Service over the given language-server table. An empty table
// falls back to the built-in LSP server table.
func New(servers []ServerSpec) *Service {
	if len(servers) == 0 {
		servers = lsp.DefaultServers()
	}
	return &Service{mgr: lsp.NewManager(servers)}
}

// Close shuts down every launched language server. Safe on a nil receiver
// / nil manager (a no-op).
func (s *Service) Close() error {
	if s == nil || s.mgr == nil {
		return nil
	}
	return s.mgr.Close()
}

// Supported reports whether a configured language server handles file's
// type. False on a nil receiver.
func (s *Service) Supported(file string) bool {
	if s == nil || s.mgr == nil {
		return false
	}
	return s.mgr.Supported(file)
}

// Definition returns the declaration location(s) of the symbol at the
// 1-based (line, column) in file, as root-relative file:line:col lines.
func (s *Service) Definition(ctx context.Context, root, file string, line, column int) (string, error) {
	if s == nil {
		return noServerMsg, nil
	}
	locs, err := s.mgr.Definition(ctx, root, file, toPosition(line, column))
	if msg, handled := foldNoServer(err); handled {
		return msg, nil
	}
	if err != nil {
		return "", err
	}
	return formatLocations(root, locs, "definition"), nil
}

// References returns every reference (including the declaration) to the
// symbol at the 1-based (line, column) in file, as root-relative locations.
func (s *Service) References(ctx context.Context, root, file string, line, column int) (string, error) {
	if s == nil {
		return noServerMsg, nil
	}
	locs, err := s.mgr.References(ctx, root, file, toPosition(line, column))
	if msg, handled := foldNoServer(err); handled {
		return msg, nil
	}
	if err != nil {
		return "", err
	}
	return formatLocations(root, locs, "reference"), nil
}

// Implementation returns the concrete implementation site(s) of the interface
// or abstract method at the 1-based (line, column) in file — e.g. every type
// that implements the interface method under the cursor.
func (s *Service) Implementation(ctx context.Context, root, file string, line, column int) (string, error) {
	if s == nil {
		return noServerMsg, nil
	}
	locs, err := s.mgr.Implementation(ctx, root, file, toPosition(line, column))
	if msg, handled := foldNoServer(err); handled {
		return msg, nil
	}
	if err != nil {
		return "", err
	}
	return formatLocations(root, locs, "implementation"), nil
}

// IncomingCalls lists the callers of the function/method at the 1-based
// (line, column) in file (who calls it). OutgoingCalls lists its callees.
func (s *Service) IncomingCalls(ctx context.Context, root, file string, line, column int) (string, error) {
	if s == nil {
		return noServerMsg, nil
	}
	syms, err := s.mgr.IncomingCalls(ctx, root, file, toPosition(line, column))
	if msg, handled := foldNoServer(err); handled {
		return msg, nil
	}
	if err != nil {
		return "", err
	}
	return formatCalls(root, syms, "incoming calls"), nil
}

func (s *Service) OutgoingCalls(ctx context.Context, root, file string, line, column int) (string, error) {
	if s == nil {
		return noServerMsg, nil
	}
	syms, err := s.mgr.OutgoingCalls(ctx, root, file, toPosition(line, column))
	if msg, handled := foldNoServer(err); handled {
		return msg, nil
	}
	if err != nil {
		return "", err
	}
	return formatCalls(root, syms, "outgoing calls"), nil
}

// Hover returns the hover text (type signature, documentation) for the
// symbol at the 1-based (line, column) in file.
func (s *Service) Hover(ctx context.Context, root, file string, line, column int) (string, error) {
	if s == nil {
		return noServerMsg, nil
	}
	text, err := s.mgr.Hover(ctx, root, file, toPosition(line, column))
	if msg, handled := foldNoServer(err); handled {
		return msg, nil
	}
	if err != nil {
		return "", err
	}
	if text == "" {
		return "No hover information available at that position.", nil
	}
	return text, nil
}

// DocumentSymbols lists the symbols declared in file (root-relative).
func (s *Service) DocumentSymbols(ctx context.Context, root, file string) (string, error) {
	if s == nil {
		return noServerMsg, nil
	}
	syms, err := s.mgr.DocumentSymbols(ctx, root, file)
	if msg, handled := foldNoServer(err); handled {
		return msg, nil
	}
	if err != nil {
		return "", err
	}
	return formatSymbols(root, syms), nil
}

// WorkspaceSymbols searches the whole workspace for symbols matching query.
func (s *Service) WorkspaceSymbols(ctx context.Context, root, query string) (string, error) {
	if s == nil {
		return noServerMsg, nil
	}
	syms, err := s.mgr.WorkspaceSymbols(ctx, root, query)
	if msg, handled := foldNoServer(err); handled {
		return msg, nil
	}
	if err != nil {
		return "", err
	}
	return formatSymbols(root, syms), nil
}

// Diagnostics returns the language server's current problems for file.
func (s *Service) Diagnostics(ctx context.Context, root, file string) (string, error) {
	if s == nil {
		return noServerMsg, nil
	}
	diags, err := s.mgr.Diagnostics(ctx, root, file)
	if msg, handled := foldNoServer(err); handled {
		return msg, nil
	}
	if err != nil {
		return "", err
	}
	return formatDiagnostics(file, diags), nil
}

// DiagnoseEdit runs apply (a file mutation on file, relative to root)
// between a before/after diagnostics snapshot and appends any problems the
// edit INTRODUCED to apply's output — the highest-value LSP integration:
// the model sees the breakage it just caused without a separate lookup.
//
// Only NEW problems surface: a baseline taken BEFORE the edit is subtracted
// from the post-edit set, keyed position-independently so a pre-existing
// problem that merely shifted lines (or that the server re-reported from
// cache) is never blamed on the edit. The bias is deliberately toward
// under-reporting.
//
// Best-effort throughout: an empty / unsupported path, an apply error, or
// any language-server trouble passes apply's result through untouched — an
// edit never fails because of code intelligence. Nil receiver → apply only.
func (s *Service) DiagnoseEdit(ctx context.Context, root, file string, apply func() (string, error)) (string, error) {
	check := s != nil && file != "" && s.Supported(file)

	// Baseline BEFORE the edit (best effort: a brand-new file has none).
	var baseline []lsp.Diagnostic
	if check {
		baseline, _ = s.mgr.Diagnostics(ctx, root, file)
	}

	out, err := apply()
	if err != nil || !check {
		return out, err // edit failed (nothing to diagnose) or unsupported
	}

	after, derr := s.mgr.Diagnostics(ctx, root, file)
	if derr != nil {
		return out, nil // never fail an edit on language-server trouble
	}
	section := diagnosticsSection(file, newProblems(baseline, after))
	if section == "" {
		return out, nil
	}
	return out + "\n\n" + section, nil
}

// foldNoServer maps the "no language server for this file type" sentinel to
// a plain model-facing message; handled is true only for that case.
func foldNoServer(err error) (msg string, handled bool) {
	if errors.Is(err, lsp.ErrNoServer) {
		return noServerMsg, true
	}
	return "", false
}

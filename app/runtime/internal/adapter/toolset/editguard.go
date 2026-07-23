package toolset

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec/turnctx"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/codeintel"
	"github.com/Tangerg/lynx/app/runtime/internal/component/pathidentity"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/editguard"
	"github.com/Tangerg/lynx/tools"
)

// The read/edit/write guards: the LLM-facing presentation of the
// [editguard.Tracker] invariant (read-before-edit + staleness). The invariant
// itself lives in domain/editguard; these wrappers parse the tool's arguments,
// resolve the path against the turn's working directory, read the session id off
// the blackboard, and turn a refused check into a model-facing message. They are
// to the editguard domain what the wire translator is to a domain event —
// presentation, kept out of the domain.

// withReadTracking wraps the read tool to stamp every successfully read file,
// marking it partial when only a line range was requested.
func withReadTracking(inner tools.Tool, tr *editguard.Tracker, workdir string) tools.Tool {
	if tr == nil {
		return inner
	}
	return wrapTool(inner, func(ctx context.Context, arguments string) (string, error) {
		out, err := inner.Call(ctx, arguments)
		if err != nil {
			return out, err
		}
		var a struct {
			Path   string `json:"file_path"`
			Offset int    `json:"offset"`
			Limit  int    `json:"limit"`
		}
		_ = json.Unmarshal([]byte(arguments), &a)
		if a.Path != "" {
			abs := pathidentity.Canonical(workdir, a.Path)
			if fingerprint, err := fingerprintFile(abs); err == nil {
				tr.Record(turnctx.TurnSession(ctx), abs, fingerprint, a.Offset > 0 || a.Limit > 0)
			}
		}
		return out, nil
	})
}

// withEditGuard wraps the edit tool: it requires the file to have been read and
// unchanged since, then refreshes the stamp after a successful edit.
func withEditGuard(inner tools.Tool, tr *editguard.Tracker, workdir string) tools.Tool {
	if tr == nil {
		return inner
	}
	return wrapTool(inner, func(ctx context.Context, arguments string) (string, error) {
		paths := mutationPaths(inner, arguments)
		for _, path := range paths {
			abs := pathidentity.Canonical(workdir, path)
			if !isExistingFile(abs) {
				continue
			}
			fingerprint, err := fingerprintFile(abs)
			if err != nil {
				continue
			}
			if verdict := tr.Check(turnctx.TurnSession(ctx), abs, fingerprint, false); !verdict.Allowed() {
				return editGuardMessage(verdict, path, "editing"), nil
			}
		}
		out, err := inner.Call(ctx, arguments)
		if err != nil {
			return out, err
		}
		for _, path := range paths {
			abs := pathidentity.Canonical(workdir, path)
			if fingerprint, err := fingerprintFile(abs); err == nil {
				tr.Refresh(turnctx.TurnSession(ctx), abs, fingerprint)
			}
		}
		return out, nil
	})
}

// withWriteGuard wraps the write tool: overwriting an EXISTING file requires a
// full, current read (a new file or an append is exempt — there's nothing to
// clobber). The stamp is refreshed after a successful write.
func withWriteGuard(inner tools.Tool, tr *editguard.Tracker, workdir string) tools.Tool {
	if tr == nil {
		return inner
	}
	return wrapTool(inner, func(ctx context.Context, arguments string) (string, error) {
		var a struct {
			Path   string `json:"file_path"`
			Append bool   `json:"append"`
		}
		_ = json.Unmarshal([]byte(arguments), &a)
		if a.Path != "" && !a.Append {
			abs := pathidentity.Canonical(workdir, a.Path)
			if isExistingFile(abs) {
				fingerprint, err := fingerprintFile(abs)
				if err == nil {
					if verdict := tr.Check(turnctx.TurnSession(ctx), abs, fingerprint, true); !verdict.Allowed() {
						return editGuardMessage(verdict, a.Path, "overwriting"), nil
					}
				}
			}
		}
		out, err := inner.Call(ctx, arguments)
		if err != nil {
			return out, err
		}
		if a.Path != "" {
			abs := pathidentity.Canonical(workdir, a.Path)
			if fingerprint, err := fingerprintFile(abs); err == nil {
				tr.Refresh(turnctx.TurnSession(ctx), abs, fingerprint)
			}
		}
		return out, nil
	})
}

func editGuardMessage(verdict editguard.Result, path, verb string) string {
	switch verdict {
	case editguard.ResultReadRequired:
		return fmt.Sprintf("You must read %s before %s it. Use the read tool first.", path, verb)
	case editguard.ResultChanged:
		return fmt.Sprintf("%s changed since you last read it (edited by the user or a tool). Read it again before %s it.", path, verb)
	case editguard.ResultFullReadRequired:
		return fmt.Sprintf("You only read part of %s. Read the whole file before %s it.", path, verb)
	default:
		return fmt.Sprintf("Cannot %s %s until its current contents have been read.", verb, path)
	}
}

func fingerprintFile(path string) (editguard.Fingerprint, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return editguard.Fingerprint{}, err
	}
	return editguard.FingerprintOf(content), nil
}

func isExistingFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// withEditDiagnostics wraps a file-mutating tool (write / edit) so a successful
// edit is immediately type-checked: the code-intelligence analyzer re-analyzes the
// file and appends any problems the edit INTRODUCED to the tool result, so the
// model sees the breakage it just caused without a separate lsp_diagnostics call.
// The baseline-diff, staleness guard, and best-effort semantics live in
// [codeintel.Analyzer.DiagnoseEdit]; here we only feed it the edit closure. root is
// the resolved workspace directory for this resolution; the wrapped tool's path
// argument is relative to it. A fs-edit decorator (sibling to the read/edit/write
// guards), not an lsp query tool — hence it lives here, not in the lsptools package.
func withEditDiagnostics(inner tools.Tool, ci *codeintel.Analyzer, root string) tools.Tool {
	if ci == nil {
		return inner
	}
	return wrapTool(inner, func(ctx context.Context, arguments string) (string, error) {
		paths := mutationPaths(inner, arguments)
		path := ""
		if len(paths) == 1 {
			path = paths[0]
		}
		return ci.DiagnoseEdit(ctx, root, path, func() (string, error) {
			return inner.Call(ctx, arguments)
		})
	})
}

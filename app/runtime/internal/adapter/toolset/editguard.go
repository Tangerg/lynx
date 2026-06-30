package toolset

import (
	"context"
	"encoding/json"

	"github.com/Tangerg/lynx/core/model/chat"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/codeintel"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/editguard"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel/turnctx"
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
func withReadTracking(inner chat.Tool, tr *editguard.Tracker, workdir string) chat.Tool {
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
			tr.Record(turnctx.TurnSession(ctx), resolveAbs(workdir, a.Path), a.Offset > 0 || a.Limit > 0)
		}
		return out, nil
	})
}

// withEditGuard wraps the edit tool: it requires the file to have been read and
// unchanged since, then refreshes the stamp after a successful edit.
func withEditGuard(inner chat.Tool, tr *editguard.Tracker, workdir string) chat.Tool {
	if tr == nil {
		return inner
	}
	return wrapTool(inner, func(ctx context.Context, arguments string) (string, error) {
		var a struct {
			Path string `json:"file_path"`
		}
		_ = json.Unmarshal([]byte(arguments), &a)
		if a.Path != "" {
			if msg := tr.Check(turnctx.TurnSession(ctx), resolveAbs(workdir, a.Path), false).Message(a.Path, "editing"); msg != "" {
				return msg, nil // recoverable: the model reads, then retries
			}
		}
		out, err := inner.Call(ctx, arguments)
		if err != nil {
			return out, err
		}
		if a.Path != "" {
			tr.Refresh(turnctx.TurnSession(ctx), resolveAbs(workdir, a.Path))
		}
		return out, nil
	})
}

// withWriteGuard wraps the write tool: overwriting an EXISTING file requires a
// full, current read (a new file or an append is exempt — there's nothing to
// clobber). The stamp is refreshed after a successful write.
func withWriteGuard(inner chat.Tool, tr *editguard.Tracker, workdir string) chat.Tool {
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
			abs := resolveAbs(workdir, a.Path)
			if isExistingFile(abs) {
				if msg := tr.Check(turnctx.TurnSession(ctx), abs, true).Message(a.Path, "overwriting"); msg != "" {
					return msg, nil
				}
			}
		}
		out, err := inner.Call(ctx, arguments)
		if err != nil {
			return out, err
		}
		if a.Path != "" {
			tr.Refresh(turnctx.TurnSession(ctx), resolveAbs(workdir, a.Path))
		}
		return out, nil
	})
}

// withEditDiagnostics wraps a file-mutating tool (write / edit) so a successful
// edit is immediately type-checked: the code-intelligence service re-analyzes the
// file and appends any problems the edit INTRODUCED to the tool result, so the
// model sees the breakage it just caused without a separate lsp_diagnostics call.
// The baseline-diff, staleness guard, and best-effort semantics live in
// [codeintel.Service.DiagnoseEdit]; here we only feed it the edit closure. root is
// the resolved workspace directory for this resolution; the wrapped tool's path
// argument is relative to it. A fs-edit decorator (sibling to the read/edit/write
// guards), not an lsp query tool — hence it lives here, not in the lsptools package.
func withEditDiagnostics(inner chat.Tool, ci *codeintel.Service, root string) chat.Tool {
	if ci == nil {
		return inner
	}
	return wrapTool(inner, func(ctx context.Context, arguments string) (string, error) {
		var a struct {
			Path string `json:"file_path"`
		}
		_ = json.Unmarshal([]byte(arguments), &a)
		return ci.DiagnoseEdit(ctx, root, a.Path, func() (string, error) {
			return inner.Call(ctx, arguments)
		})
	})
}

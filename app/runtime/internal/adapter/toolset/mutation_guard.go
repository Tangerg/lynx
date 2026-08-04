package toolset

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	toolcontract "github.com/Tangerg/lynx/tool"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/codeintel"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/executionctx"
	"github.com/Tangerg/lynx/app/runtime/internal/component/pathidentity"
)

// These decorators enforce the readTracker invariant at the model boundary.
// They resolve tool paths against the Run working directory, scope stamps to
// the active session, and return an actionable result when a mutation is stale.

// withReadTracking stamps the current full-file fingerprint after a successful
// read. The read may return a range, but staleness is checked against the whole
// file so any concurrent change invalidates the stamp.
func withReadTracking(inner toolcontract.Tool, tr *readTracker, workdir string) toolcontract.Tool {
	if tr == nil {
		return inner
	}
	return decorate(inner, func(ctx context.Context, arguments string) (string, error) {
		out, err := inner.Call(ctx, arguments)
		if err != nil {
			return out, err
		}
		var a struct {
			Path string `json:"path"`
		}
		_ = json.Unmarshal([]byte(arguments), &a)
		if a.Path != "" {
			abs, pathErr := pathidentity.Canonical(workdir, a.Path)
			if pathErr != nil {
				return out, fmt.Errorf("track read path: %w", pathErr)
			}
			if fingerprint, err := fingerprintFile(abs); err == nil {
				tr.record(executionctx.SessionID(ctx), abs, fingerprint)
			}
		}
		return out, nil
	})
}

// withMutationGuard requires every existing target to have been read and to
// remain unchanged, then refreshes stamps after a successful mutation.
func withMutationGuard(inner toolcontract.Tool, tr *readTracker, workdir string) toolcontract.Tool {
	if tr == nil {
		return inner
	}
	return decorate(inner, func(ctx context.Context, arguments string) (string, error) {
		paths, err := mutationPaths(inner, arguments)
		if err != nil {
			return "", fmt.Errorf("inspect mutation paths before applying patch: %w", err)
		}
		for _, path := range paths {
			abs, err := pathidentity.Canonical(workdir, path)
			if err != nil {
				return "", fmt.Errorf("resolve mutation path: %w", err)
			}
			if !isExistingFile(abs) {
				continue
			}
			fingerprint, err := fingerprintFile(abs)
			if err != nil {
				continue
			}
			if verdict := tr.check(executionctx.SessionID(ctx), abs, fingerprint); !verdict.allowed() {
				return mutationGuardMessage(verdict, path), nil
			}
		}
		out, err := inner.Call(ctx, arguments)
		if err != nil {
			return out, err
		}
		for _, path := range paths {
			abs, err := pathidentity.Canonical(workdir, path)
			if err != nil {
				return out, fmt.Errorf("refresh mutation path: %w", err)
			}
			if fingerprint, err := fingerprintFile(abs); err == nil {
				tr.refresh(executionctx.SessionID(ctx), abs, fingerprint)
			}
		}
		return out, nil
	})
}

func mutationGuardMessage(verdict guardVerdict, path string) string {
	switch verdict {
	case readRequired:
		return fmt.Sprintf("You must read %s before modifying it. Use the read tool first.", path)
	case contentChanged:
		return fmt.Sprintf("%s changed since you last read it (modified by the user or a tool). Read it again before modifying it.", path)
	default:
		return fmt.Sprintf("Cannot modify %s until its current contents have been read.", path)
	}
}

func fingerprintFile(path string) (contentFingerprint, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return contentFingerprint{}, err
	}
	return fingerprintOf(content), nil
}

func isExistingFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// withMutationDiagnostics wraps a file-mutating tool so a successful change is
// immediately type-checked. The baseline diff and best-effort semantics live in
// [codeintel.Analyzer.DiagnoseMutation]; this adapter supplies the mutation
// closure and resolved workspace root. It is a filesystem decorator, not an LSP
// query tool, so it lives here rather than in package lsp.
func withMutationDiagnostics(inner toolcontract.Tool, ci *codeintel.Analyzer, root string) toolcontract.Tool {
	if ci == nil {
		return inner
	}
	return decorate(inner, func(ctx context.Context, arguments string) (string, error) {
		paths, err := mutationPaths(inner, arguments)
		if err != nil {
			return "", fmt.Errorf("inspect mutation paths before diagnostics: %w", err)
		}
		path := ""
		if len(paths) == 1 {
			path = paths[0]
		}
		return ci.DiagnoseMutation(ctx, root, path, func() (string, error) {
			return inner.Call(ctx, arguments)
		})
	})
}

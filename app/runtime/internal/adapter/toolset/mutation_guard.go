package toolset

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	toolcontract "github.com/Tangerg/lynx/tool"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/codeintel"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/executionctx"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/pathidentity"
)

// These decorators enforce the readTracker invariant at the model boundary.
// They resolve tool paths against the Run working directory, scope stamps to
// the active session, and return an actionable result when a mutation is stale.

// withReadTracking stamps the current full-file fingerprint after a successful
// read. The read may return a range, but staleness is checked against the whole
// file so any concurrent change invalidates the stamp.
func withReadTracking(inner toolcontract.Tool, tr *readTracker, cwd string) toolcontract.Tool {
	if tr == nil {
		return inner
	}
	return decorateCall(inner, func(ctx context.Context, arguments string) (string, error) {
		out, err := inner.Call(ctx, arguments)
		if err != nil {
			return out, err
		}
		var a struct {
			Path string `json:"path"`
		}
		_ = json.Unmarshal([]byte(arguments), &a)
		if a.Path != "" {
			abs, pathErr := pathidentity.Canonical(cwd, a.Path)
			if pathErr != nil {
				return out, fmt.Errorf("track read path: %w", pathErr)
			}
			fingerprint, exists, err := fingerprintExistingFile(ctx, abs)
			if err != nil {
				return out, fmt.Errorf("track read %s: %w", a.Path, err)
			}
			if !exists {
				tr.forget(executionctx.SessionID(ctx), abs)
				return out, fmt.Errorf("track read %s: file no longer exists", a.Path)
			}
			tr.record(executionctx.SessionID(ctx), abs, fingerprint)
		}
		return out, nil
	})
}

// withMutationGuard requires every existing target to have been read and to
// remain unchanged, then refreshes stamps after a successful mutation.
func withMutationGuard(inner toolcontract.Tool, tr *readTracker, cwd string) toolcontract.Tool {
	if tr == nil {
		return inner
	}
	return decorateCall(inner, func(ctx context.Context, arguments string) (string, error) {
		paths, err := mutationPaths(inner, arguments)
		if err != nil {
			return "", fmt.Errorf("inspect mutation paths before applying patch: %w", err)
		}
		for _, path := range paths {
			abs, err := pathidentity.Canonical(cwd, path)
			if err != nil {
				return "", fmt.Errorf("resolve mutation path: %w", err)
			}
			fingerprint, exists, err := fingerprintExistingFile(ctx, abs)
			if err != nil {
				return "", fmt.Errorf("fingerprint mutation path %s: %w", path, err)
			}
			if !exists {
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
			abs, err := pathidentity.Canonical(cwd, path)
			if err != nil {
				return out, fmt.Errorf("refresh mutation path: %w", err)
			}
			fingerprint, exists, fingerprintErr := fingerprintExistingFile(ctx, abs)
			if fingerprintErr != nil {
				tr.forget(executionctx.SessionID(ctx), abs)
				return out, fmt.Errorf("refresh mutation path %s: %w", path, fingerprintErr)
			}
			if !exists {
				tr.forget(executionctx.SessionID(ctx), abs)
				continue
			}
			tr.refresh(executionctx.SessionID(ctx), abs, fingerprint)
		}
		return out, nil
	})
}

func mutationGuardMessage(verdict guardVerdict, path string) string {
	switch verdict {
	case readRequired:
		return fmt.Sprintf("You must read %s before modifying it. Use the read tool first.", path)
	case contentChanged:
		return path + " changed since you last read it (modified by the user or a tool). Read it again before modifying it."
	default:
		return fmt.Sprintf("Cannot modify %s until its current contents have been read.", path)
	}
}

func fingerprintExistingFile(ctx context.Context, path string) (contentFingerprint, bool, error) {
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return contentFingerprint{}, false, nil
	}
	if err != nil {
		return contentFingerprint{}, false, err
	}
	if !info.Mode().IsRegular() {
		return contentFingerprint{}, false, fmt.Errorf("unsupported file mode %s", info.Mode().Type())
	}
	fingerprint, err := fingerprintFile(ctx, path)
	return fingerprint, true, err
}

func fingerprintFile(ctx context.Context, path string) (_ contentFingerprint, err error) {
	if cause := context.Cause(ctx); cause != nil {
		return contentFingerprint{}, cause
	}
	file, err := os.Open(path)
	if err != nil {
		return contentFingerprint{}, err
	}
	defer func() {
		err = errors.Join(err, file.Close())
	}()
	hash := sha256.New()
	buffer := make([]byte, 64<<10)
	if _, err := io.CopyBuffer(hash, fingerprintContextReader{ctx: ctx, reader: file}, buffer); err != nil {
		return contentFingerprint{}, err
	}
	var fingerprint contentFingerprint
	copy(fingerprint[:], hash.Sum(nil))
	return fingerprint, nil
}

type fingerprintContextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (reader fingerprintContextReader) Read(buffer []byte) (int, error) {
	if cause := context.Cause(reader.ctx); cause != nil {
		return 0, cause
	}
	read, err := reader.reader.Read(buffer)
	if cause := context.Cause(reader.ctx); cause != nil {
		return read, cause
	}
	return read, err
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
	return decorateCall(inner, func(ctx context.Context, arguments string) (string, error) {
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

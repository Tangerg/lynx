package toolset

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"

	toolcontract "github.com/Tangerg/lynx/core/tool"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/codeintel"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/executionctx"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/pathidentity"
	"github.com/Tangerg/lynx/tools/fs"
)

// These decorators enforce the readTracker invariant at the model boundary.
// They resolve tool paths against the Run working directory, scope stamps to
// the active session, and return an actionable result when a mutation is stale.

// withReadTracking stamps a full-file fingerprint only when the same digest
// brackets the successful read. The read may return a range, but any concurrent
// whole-file change invalidates the call rather than authorizing unseen bytes.
func withReadTracking(inner toolcontract.Tool, tr *readTracker, cwd string) toolcontract.Tool {
	if tr == nil {
		return inner
	}
	return decorateCall(inner, func(ctx context.Context, arguments string) (string, error) {
		request, decodeErr := decodeToolArguments[fs.ReadRequest](arguments)
		if decodeErr != nil || request.Path == "" {
			return inner.Call(ctx, arguments)
		}
		abs, pathErr := pathidentity.Canonical(cwd, request.Path)
		if pathErr != nil {
			return "", fmt.Errorf("track read path: %w", pathErr)
		}
		before, existedBefore, err := observeFingerprintExistingFile(ctx, abs, maxRuntimeReadFileBytes)
		if err != nil {
			return "", fmt.Errorf("track read %s: %w", request.Path, err)
		}
		out, err := inner.Call(ctx, arguments)
		if err != nil {
			return out, err
		}
		after, existsAfter, err := observeFingerprintExistingFile(ctx, abs, maxRuntimeReadFileBytes)
		if err != nil {
			tr.forget(executionctx.SessionID(ctx), abs)
			return out, fmt.Errorf("track read %s: %w", request.Path, err)
		}
		if !existedBefore || !existsAfter || !sameFingerprintObservation(before, after) {
			tr.forget(executionctx.SessionID(ctx), abs)
			return out, fmt.Errorf("track read %s: file changed while reading; read it again", request.Path)
		}
		tr.record(executionctx.SessionID(ctx), abs, after.fingerprint)
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
			fingerprint, exists, err := fingerprintExistingFile(ctx, abs, 0)
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
			fingerprint, exists, fingerprintErr := fingerprintExistingFile(ctx, abs, 0)
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

func fingerprintExistingFile(ctx context.Context, path string, maxBytes int64) (contentFingerprint, bool, error) {
	observation, exists, err := observeFingerprintExistingFile(ctx, path, maxBytes)
	return observation.fingerprint, exists, err
}

type fingerprintObservation struct {
	fingerprint contentFingerprint
	info        os.FileInfo
}

func observeFingerprintExistingFile(ctx context.Context, path string, maxBytes int64) (_ fingerprintObservation, exists bool, err error) {
	preflight, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return fingerprintObservation{}, false, nil
	}
	if err != nil {
		return fingerprintObservation{}, false, err
	}
	if !preflight.Mode().IsRegular() {
		return fingerprintObservation{}, false, fmt.Errorf("unsupported file mode %s", preflight.Mode().Type())
	}
	if maxBytes > 0 && preflight.Size() > maxBytes {
		return fingerprintObservation{}, false, fmt.Errorf("%w: file uses %d bytes", errRuntimeReadFileTooLarge, preflight.Size())
	}
	if cause := context.Cause(ctx); cause != nil {
		return fingerprintObservation{}, false, cause
	}
	file, err := os.Open(path)
	if err != nil {
		return fingerprintObservation{}, false, err
	}
	defer func() {
		err = errors.Join(err, file.Close())
	}()
	before, err := file.Stat()
	if err != nil {
		return fingerprintObservation{}, false, err
	}
	if !before.Mode().IsRegular() {
		return fingerprintObservation{}, false, fmt.Errorf("unsupported file mode %s", before.Mode().Type())
	}
	if maxBytes > 0 && before.Size() > maxBytes {
		return fingerprintObservation{}, false, fmt.Errorf("%w: file uses %d bytes", errRuntimeReadFileTooLarge, before.Size())
	}
	hash := sha256.New()
	buffer := make([]byte, 64<<10)
	var source io.Reader = fingerprintContextReader{ctx: ctx, reader: file}
	if maxBytes > 0 {
		source = io.LimitReader(source, maxBytes+1)
	}
	written, err := io.CopyBuffer(hash, source, buffer)
	if err != nil {
		return fingerprintObservation{}, false, err
	}
	if maxBytes > 0 && written > maxBytes {
		return fingerprintObservation{}, false, fmt.Errorf("%w: file grew while fingerprinting", errRuntimeReadFileTooLarge)
	}
	after, err := file.Stat()
	if err != nil {
		return fingerprintObservation{}, false, err
	}
	current, err := os.Stat(path)
	if err != nil {
		return fingerprintObservation{}, false, fmt.Errorf("file changed while fingerprinting: %w", err)
	}
	if !sameFileVersion(before, after) || !sameFileVersion(after, current) {
		return fingerprintObservation{}, false, errors.New("file changed while fingerprinting")
	}
	var fingerprint contentFingerprint
	copy(fingerprint[:], hash.Sum(nil))
	return fingerprintObservation{fingerprint: fingerprint, info: current}, true, nil
}

func fingerprintFile(ctx context.Context, path string, maxBytes int64) (contentFingerprint, error) {
	observation, exists, err := observeFingerprintExistingFile(ctx, path, maxBytes)
	if err != nil {
		return contentFingerprint{}, err
	}
	if !exists {
		return contentFingerprint{}, os.ErrNotExist
	}
	return observation.fingerprint, nil
}

func sameFingerprintObservation(left, right fingerprintObservation) bool {
	return left.fingerprint == right.fingerprint && sameFileVersion(left.info, right.info)
}

func sameFileVersion(left, right os.FileInfo) bool {
	return left != nil && right != nil &&
		os.SameFile(left, right) &&
		left.Size() == right.Size() &&
		left.Mode() == right.Mode() &&
		left.ModTime().Equal(right.ModTime())
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

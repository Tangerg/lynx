package fs

import (
	"cmp"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// LocalExecutor is the reference [Executor] running against the host
// filesystem.
//
//   - Glob shells out to find (portable across BSD and GNU; bash 3.2
//     on macOS doesn't honor globstar, so a bash-based impl wouldn't
//     work everywhere).
//   - Grep prefers ripgrep when it's on PATH; falls back to GNU grep
//     otherwise (FileType / Multiline only work on the ripgrep path).
//   - Write and Edit serialize per file via [LocalExecutor.lockPath]
//     so concurrent tool calls on the same path can't tear.
//   - Read normalises CRLF→LF and strips UTF-8 BOM; Write and Edit
//     restore both when the existing file uses them.
type LocalExecutor struct {
	// Root, if set, anchors relative paths. "" = no confinement.
	// This is not a security jail; callers that need confinement validate
	// paths before invoking the executor.
	Root string

	pathLocksMu sync.Mutex
	pathLocks   map[string]*sync.Mutex

	rgOnce sync.Once
	rgPath string // "" after rgOnce runs means rg is not on PATH
}

// NewLocalExecutor returns a [LocalExecutor] anchored at root. Pass
// "" for an unrestricted executor (typical for trusted local dev).
func NewLocalExecutor(root string) *LocalExecutor {
	return &LocalExecutor{Root: root}
}

// resolve combines the executor's Root with a relative path. A leading ~ is
// expanded to the home dir first; absolute paths pass through. Empty path is
// rejected.
func (l *LocalExecutor) resolve(path string) (string, error) {
	if path == "" {
		return "", ErrEmptyPath
	}
	path = expandHome(path)
	if l.Root == "" || filepath.IsAbs(path) {
		return path, nil
	}
	return filepath.Join(l.Root, path), nil
}

// rootDir returns the directory bulk queries (Glob/Grep) should start
// from. Precedence: caller-supplied Root → executor's Root → CWD. A leading
// ~ in the chosen dir is expanded to the home dir.
func (l *LocalExecutor) rootDir(callerRoot string) string {
	return expandHome(cmp.Or(callerRoot, l.Root, "."))
}

// expandHome expands a leading ~ — the shell convention an LLM routinely emits
// — to the current user's home dir: "~" or "~/" → home, "~/x" → home/x. Any
// other form (a plain relative path, an absolute path, or "~user") is returned
// unchanged. Best-effort: if the home dir can't be resolved the path is left
// as-is. Without this, "~/x" anchors literally under Root as ".../~/x" and the
// open fails with "no such file or directory".
func expandHome(path string) string {
	if path != "~" && !strings.HasPrefix(path, "~/") {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return path
	}
	if path == "~" {
		return home
	}
	return filepath.Join(home, path[len("~/"):])
}

// lockPath returns a per-path mutex unlock func. Used to serialize
// Write and Edit on the same file. The map of locks grows monotonically
// — bounded by the set of paths the agent touches — which is acceptable
// for typical workspace sizes (a few thousand entries × 16 bytes).
func (l *LocalExecutor) lockPath(path string) func() {
	l.pathLocksMu.Lock()
	if l.pathLocks == nil {
		l.pathLocks = map[string]*sync.Mutex{}
	}
	m, ok := l.pathLocks[path]
	if !ok {
		m = &sync.Mutex{}
		l.pathLocks[path] = m
	}
	l.pathLocksMu.Unlock()
	m.Lock()
	return m.Unlock
}

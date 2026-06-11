package engine

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/Tangerg/lynx/core/model/chat"
)

// readTracker records which files an agent has read, and the content hash at
// read time, so the edit / write guards can enforce two reliability rules that
// the mature Claude-optimized agent (Claude Code) relies on instead of a patch
// format: a file must be READ before it is edited, and it must not have CHANGED
// since (a user or a tool — e.g. a formatter — may have rewritten it). The
// guard makes the agent re-read rather than blindly clobbering stale content.
//
// Keyed by session so one session reading a file doesn't license another to
// edit it. In-memory and per-engine: lost on restart (the agent just re-reads,
// exactly as Claude Code's per-session cache behaves). Content hash, not mtime
// — mtime has coarse granularity and is unreliable across filesystems, and we
// read the file content anyway.
type readTracker struct {
	mu   sync.Mutex
	seen map[string]map[string]fileStamp // sessionID → absPath → stamp
}

type fileStamp struct {
	hash    [32]byte
	partial bool // only a line range was read → not safe to overwrite wholesale
}

func newReadTracker() *readTracker {
	return &readTracker{seen: map[string]map[string]fileStamp{}}
}

func (t *readTracker) record(session, abs string, st fileStamp) {
	t.mu.Lock()
	defer t.mu.Unlock()
	m := t.seen[session]
	if m == nil {
		m = map[string]fileStamp{}
		t.seen[session] = m
	}
	m[abs] = st
}

// refresh re-stamps abs from its current content (a full view), called after a
// successful edit / write so consecutive edits to the same file in a turn
// don't trip the guard.
func (t *readTracker) refresh(session, abs string) {
	if h, err := hashFile(abs); err == nil {
		t.record(session, abs, fileStamp{hash: h})
	}
}

type readCheck int

const (
	readOK      readCheck = iota
	readMissing           // never read in this session
	readStale             // changed on disk since it was read
	readPartial           // only a range was read; full overwrite unsafe (write only)
)

// check reports whether abs may be modified by this session. requireFull adds
// the partial-view rule (a whole-file overwrite needs a whole-file read). A
// file that can't be hashed now (missing / unreadable) returns readOK so the
// underlying tool surfaces its own, more specific error.
func (t *readTracker) check(session, abs string, requireFull bool) readCheck {
	st, ok := t.get(session, abs)
	if !ok {
		return readMissing
	}
	cur, err := hashFile(abs)
	if err != nil {
		return readOK
	}
	if cur != st.hash {
		return readStale
	}
	if requireFull && st.partial {
		return readPartial
	}
	return readOK
}

func (t *readTracker) get(session, abs string) (fileStamp, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	st, ok := t.seen[session][abs]
	return st, ok
}

// withReadTracking wraps the read tool to stamp every successfully read file,
// marking it partial when only a line range was requested.
func withReadTracking(inner chat.Tool, tr *readTracker, workdir string) chat.Tool {
	if tr == nil {
		return inner
	}
	t, _ := chat.NewTool(inner.Definition(), inner.Metadata(),
		func(ctx context.Context, arguments string) (string, error) {
			out, err := inner.Call(ctx, arguments)
			if err != nil {
				return out, err
			}
			var a struct {
				Path   string `json:"path"`
				Offset int    `json:"offset"`
				Limit  int    `json:"limit"`
			}
			_ = json.Unmarshal([]byte(arguments), &a)
			if a.Path != "" {
				abs := resolveAbs(workdir, a.Path)
				if h, herr := hashFile(abs); herr == nil {
					tr.record(turnSession(ctx), abs, fileStamp{hash: h, partial: a.Offset > 0 || a.Limit > 0})
				}
			}
			return out, nil
		})
	return t
}

// withEditGuard wraps the edit tool: it requires the file to have been read and
// unchanged since, then refreshes the stamp after a successful edit.
func withEditGuard(inner chat.Tool, tr *readTracker, workdir string) chat.Tool {
	if tr == nil {
		return inner
	}
	t, _ := chat.NewTool(inner.Definition(), inner.Metadata(),
		func(ctx context.Context, arguments string) (string, error) {
			var a struct {
				Path string `json:"path"`
			}
			_ = json.Unmarshal([]byte(arguments), &a)
			if a.Path != "" {
				if msg := guardMessage(tr.check(turnSession(ctx), resolveAbs(workdir, a.Path), false), a.Path, "editing"); msg != "" {
					return msg, nil // recoverable: the model reads, then retries
				}
			}
			out, err := inner.Call(ctx, arguments)
			if err != nil {
				return out, err
			}
			if a.Path != "" {
				tr.refresh(turnSession(ctx), resolveAbs(workdir, a.Path))
			}
			return out, nil
		})
	return t
}

// withWriteGuard wraps the write tool: overwriting an EXISTING file requires a
// full, current read (a new file or an append is exempt — there's nothing to
// clobber). The stamp is refreshed after a successful write.
func withWriteGuard(inner chat.Tool, tr *readTracker, workdir string) chat.Tool {
	if tr == nil {
		return inner
	}
	t, _ := chat.NewTool(inner.Definition(), inner.Metadata(),
		func(ctx context.Context, arguments string) (string, error) {
			var a struct {
				Path   string `json:"path"`
				Append bool   `json:"append"`
			}
			_ = json.Unmarshal([]byte(arguments), &a)
			if a.Path != "" && !a.Append {
				abs := resolveAbs(workdir, a.Path)
				if isExistingFile(abs) {
					if msg := guardMessage(tr.check(turnSession(ctx), abs, true), a.Path, "overwriting"); msg != "" {
						return msg, nil
					}
				}
			}
			out, err := inner.Call(ctx, arguments)
			if err != nil {
				return out, err
			}
			if a.Path != "" {
				tr.refresh(turnSession(ctx), resolveAbs(workdir, a.Path))
			}
			return out, nil
		})
	return t
}

// guardMessage renders the model-facing instruction for a failed check (""
// when the check passed). verb is "editing" / "overwriting".
func guardMessage(c readCheck, path, verb string) string {
	switch c {
	case readMissing:
		return fmt.Sprintf("You must read %s before %s it. Use the read tool first.", path, verb)
	case readStale:
		return fmt.Sprintf("%s changed since you last read it (edited by the user or a tool). Read it again before %s it.", path, verb)
	case readPartial:
		return fmt.Sprintf("You only read part of %s. Read the whole file before %s it.", path, verb)
	default:
		return ""
	}
}

func resolveAbs(workdir, path string) string {
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	return filepath.Clean(filepath.Join(workdir, path))
}

func hashFile(abs string) ([32]byte, error) {
	data, err := os.ReadFile(abs)
	if err != nil {
		return [32]byte{}, err
	}
	return sha256.Sum256(data), nil
}

func isExistingFile(abs string) bool {
	info, err := os.Stat(abs)
	return err == nil && !info.IsDir()
}

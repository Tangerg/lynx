package server

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
)

// workspaceHub fans workspace events out to the live workspace.subscribe
// streams (AUX_API §3). It is the non-run, ephemeral counterpart to the
// per-run hubs: lossy (a slow subscriber drops the event rather than
// back-pressuring the publisher — workspace events are "changed → re-fetch",
// so a drop self-heals on the next change or a resync), connection-scoped (no
// durable replay), shared by the whole app.
type workspaceHub struct {
	mu   sync.Mutex
	subs map[chan protocol.WorkspaceEvent]struct{}
}

func newWorkspaceHub() *workspaceHub {
	return &workspaceHub{subs: map[chan protocol.WorkspaceEvent]struct{}{}}
}

// register adds a caller-owned channel to the broadcast fan-out and returns an
// idempotent unregister. It does NOT close the channel — the owner does, after
// it has stopped every other writer (the file watcher), so a late broadcast
// can't send on a closed channel.
func (h *workspaceHub) register(ch chan protocol.WorkspaceEvent) func() {
	h.mu.Lock()
	h.subs[ch] = struct{}{}
	h.mu.Unlock()
	return func() {
		h.mu.Lock()
		delete(h.subs, ch)
		h.mu.Unlock()
	}
}

// publish fans ev to every subscriber, dropping it for any whose buffer is
// full (lossy by design — see the type doc).
func (h *workspaceHub) publish(ev protocol.WorkspaceEvent) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.subs {
		select {
		case ch <- ev:
		default: // full subscriber: drop (it re-fetches on the next change / resync)
		}
	}
}

// WorkspaceSubscribe opens the workspace event stream (AUX_API §3.1). The
// stream's lifetime is the subscription; it ends when the request ctx does
// (client disconnect). Broadcast events (mcp.serverChanged, skills.changed) go
// to every subscription; when the request carries watches, the subscription
// also monitors those cwds' git state and emits a debounced resync on any
// change (commit / stage / checkout / merge) — the client then re-fetches
// workspace.getDiff. (Working-tree file edits aren't watched directly — see
// gitWatcher; the agent's own edits arrive as files.changed from its tools.)
func (s *Server) WorkspaceSubscribe(ctx context.Context, in protocol.WorkspaceSubscribeRequest) (*protocol.WorkspaceSubscribeResponse, <-chan protocol.WorkspaceEvent, error) {
	gitDirs, err := s.resolveWatchGitDirs(in.Watches)
	if err != nil {
		return nil, nil, err
	}
	streamCtx, release, ok := s.tasks.AttachLinked(ctx)
	if !ok {
		return nil, nil, errServerClosed
	}

	// WorkspaceSubscribe owns the channel: the hub broadcasts to it and (when
	// watches are present) the git watcher emits to it. Closing it only after
	// the watcher has stopped keeps emit from racing the close.
	out := make(chan protocol.WorkspaceEvent, 64)
	unregister := s.wsHub.register(out)

	var watcher *gitWatcher
	if len(gitDirs) > 0 {
		watcher, err = startGitWatcher(gitDirs, func(ev protocol.WorkspaceEvent) {
			select {
			case out <- ev: // lossy: a slow subscriber drops the event (re-fetch on next change)
			default:
			}
		})
		if err != nil {
			unregister()
			close(out)
			release()
			return nil, nil, fmt.Errorf("workspace.subscribe: start git watcher: %w", err)
		}
	}

	context.AfterFunc(streamCtx, func() {
		if watcher != nil {
			watcher.Close() // joins the watch goroutine — no emit after this
		}
		unregister() // hub stops broadcasting to out
		close(out)
		release()
	})
	return &protocol.WorkspaceSubscribeResponse{}, out, nil
}

// resolveWatchGitDirs validates each watch spec and resolves the DISTINCT .git
// directories to monitor. A watch's cwd defaults to the serve directory; a
// non-repo cwd contributes no git dir (its watch is inert — getDiff would
// report vcs_unavailable too). Returns invalid_params for a watch missing its
// id or an unresolvable cwd.
func (s *Server) resolveWatchGitDirs(specs []protocol.WatchSpec) ([]string, error) {
	if len(specs) == 0 {
		return nil, nil
	}
	seen := map[string]struct{}{}
	var gitDirs []string
	for _, spec := range specs {
		if spec.WatchID == "" {
			return nil, fmt.Errorf("%w: watchId is required", protocol.ErrInvalidParams)
		}
		root, err := s.workspaceRoot(spec.Cwd)
		if err != nil {
			return nil, err
		}
		g, ok := gitDirOf(root)
		if !ok {
			continue // not a repo → nothing to watch for this cwd
		}
		if _, dup := seen[g]; dup {
			continue
		}
		seen[g] = struct{}{}
		gitDirs = append(gitDirs, g)
	}
	return gitDirs, nil
}

// PublishWorkspaceEvent fans one workspace event out to subscribers. The
// runtime / engine call this when a non-run state change happens (mcp
// serverChanged, skills.changed, files.changed). Safe to call with no
// subscribers (no-op).
func (s *Server) PublishWorkspaceEvent(ev protocol.WorkspaceEvent) {
	if s.wsHub != nil {
		s.wsHub.publish(ev)
	}
}

// sessionCwd resolves a session's working directory (empty on any error) — used
// to scope tool-derived files.changed events.
func (s *Server) sessionCwd(ctx context.Context, sessionID string) string {
	sess, err := s.rt.SessionByID(ctx, sessionID)
	if err != nil {
		return ""
	}
	return sess.Cwd
}

// fileMutatingTools are the agent tools whose completed call means a specific
// file changed — their path argument names it exactly, so a workspace
// subscriber can refresh without watching the working tree. shell is excluded
// (its file effects aren't knowable from arguments); git-affecting commands
// surface via the .git watch instead.
var fileMutatingTools = map[string]struct{}{"write": {}, "edit": {}}

// toolFileChangedPaths extracts file-change paths from a completed, successful
// file-mutating tool call. This is the precise, fd-free half of the watch
// model: the agent's own edits are known exactly from tool arguments, so the
// git watcher only needs to cover out-of-band (git) state changes.
func toolFileChangedPaths(se protocol.StreamEvent) []string {
	if se.Type != protocol.StreamItemCompleted || se.Item == nil {
		return nil
	}
	it := se.Item
	if it.Type != protocol.ItemTypeToolCall || it.Status != protocol.ItemStatusCompleted || it.Error != nil || it.Tool == nil {
		return nil
	}
	if _, ok := fileMutatingTools[strings.ToLower(it.Tool.Name)]; !ok {
		return nil
	}
	path, _ := it.Tool.Arguments["file_path"].(string)
	if path == "" {
		return nil
	}
	return []string{path}
}

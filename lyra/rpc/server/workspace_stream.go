package server

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/Tangerg/lynx/lyra/internal/git"
	"github.com/Tangerg/lynx/lyra/rpc/protocol"
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

// subscribe is the broadcast-only convenience: a hub-owned channel + an
// unsubscribe that unregisters AND closes it. For subscriptions that also run a
// file watcher, WorkspaceSubscribe owns the channel directly (via register) so
// it can close it only after stopping the watcher.
func (h *workspaceHub) subscribe() (<-chan protocol.WorkspaceEvent, func()) {
	ch := make(chan protocol.WorkspaceEvent, 64)
	unregister := h.register(ch)
	return ch, func() {
		unregister()
		close(ch)
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
// to every subscription; the optional watches register per-subscription file
// monitoring, whose debounced files.changed events ride the same channel
// (features.fileWatch). An invalid watch (escaping the cwd jail, missing dir)
// fails the call rather than silently dropping the watch.
func (s *Server) WorkspaceSubscribe(ctx context.Context, in protocol.WorkspaceSubscribeRequest) (*protocol.WorkspaceSubscribeResponse, <-chan protocol.WorkspaceEvent, error) {
	targets, err := s.resolveWatches(in.Watches)
	if err != nil {
		return nil, nil, err
	}

	// WorkspaceSubscribe owns the channel: the hub broadcasts to it and (when
	// watches are present) the file watcher emits to it. Closing it only after
	// the watcher has stopped keeps emit from racing the close.
	out := make(chan protocol.WorkspaceEvent, 64)
	unregister := s.wsHub.register(out)

	var watcher *fileWatcher
	if len(targets) > 0 {
		watcher, err = startFileWatcher(targets, func(ev protocol.WorkspaceEvent) {
			select {
			case out <- ev: // lossy: a slow subscriber drops the event (re-fetch on next change)
			default:
			}
		})
		if err != nil {
			unregister()
			close(out)
			return nil, nil, fmt.Errorf("workspace.subscribe: start file watcher: %w", err)
		}
	}

	context.AfterFunc(ctx, func() {
		if watcher != nil {
			watcher.Close() // joins the watch goroutine — no emit after this
		}
		unregister() // hub stops broadcasting to out
		close(out)
	})
	return &protocol.WorkspaceSubscribeResponse{}, out, nil
}

// resolveWatches validates + resolves each watch spec onto an absolute, jailed
// directory to watch. A watch path is relative to its cwd (default the serve
// dir) and confined to it (path_outside_root on escape); empty path watches the
// cwd root. The target must be an existing directory (fsnotify watches dirs).
func (s *Server) resolveWatches(specs []protocol.WatchSpec) ([]watchTarget, error) {
	if len(specs) == 0 {
		return nil, nil
	}
	targets := make([]watchTarget, 0, len(specs))
	for _, spec := range specs {
		if spec.WatchID == "" {
			return nil, fmt.Errorf("%w: watchId is required", protocol.ErrInvalidParams)
		}
		root, err := s.workspaceRoot(spec.Cwd)
		if err != nil {
			return nil, err
		}
		abs := root
		if spec.Path != "" {
			rel, rerr := resolveInRoot(root, spec.Path)
			if rerr != nil {
				return nil, rerr
			}
			abs = filepath.Join(root, rel)
		}
		if info, err := os.Stat(abs); err != nil || !info.IsDir() {
			return nil, fmt.Errorf("%w: watch path %q is not a directory", protocol.ErrInvalidParams, spec.Path)
		}
		targets = append(targets, watchTarget{
			watchID: spec.WatchID,
			cwdRoot: root,
			absPath: abs,
			ignore:  git.LoadIgnore(root), // skip the cwd's gitignored subtrees + file events
		})
	}
	return targets, nil
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

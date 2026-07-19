package server

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
)

// errServerClosed reports that a request-detached delivery operation could not
// start because the Server is shutting down (its task group is closed).
var errServerClosed = errors.New("server: closed")

// workspaceHub fans workspace events out to the live workspace.subscribe
// streams (AUX_API §3). It is the non-run, ephemeral counterpart to the
// per-run hubs: lossy (a slow subscriber drops the event rather than
// back-pressuring the publisher — workspace events are "changed → re-fetch",
// so a drop self-heals on the next change or a resync), connection-scoped (no
// durable replay), shared by the whole app.
type workspaceHub struct {
	mu     sync.Mutex
	subs   map[*workspaceSubscription]struct{}
	closed bool
}

func newWorkspaceHub() *workspaceHub {
	return &workspaceHub{subs: make(map[*workspaceSubscription]struct{})}
}

type workspaceSubscription struct {
	events   chan protocol.WorkspaceEvent
	sequence uint64
}

// register adds a caller-owned channel to the broadcast fan-out and returns an
// idempotent unregister. It does NOT close the channel — the owner does, after
// it has stopped every other writer (the file watcher), so a late broadcast
// can't send on a closed channel.
func (h *workspaceHub) register(ch chan protocol.WorkspaceEvent) (*workspaceSubscription, func(), bool) {
	sub := &workspaceSubscription{events: ch}
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return nil, nil, false
	}
	h.subs[sub] = struct{}{}
	h.mu.Unlock()
	return sub, func() {
		h.mu.Lock()
		delete(h.subs, sub)
		h.mu.Unlock()
	}, true
}

// closeAdmissions linearizes Server.Close with workspace subscription
// registration. Existing request-owned streams keep running until their own
// contexts end, but once this returns no racing check-then-register path can
// create another subscription.
func (h *workspaceHub) closeAdmissions() {
	h.mu.Lock()
	h.closed = true
	h.mu.Unlock()
}

// observe wires the run pump's live file-change nudges (delivered through the
// composition-root bridge) into the hub: each nudge becomes a files.changed
// workspace event fanned to subscribers. The wire WorkspaceEvent shape stays
// here in delivery; the bridge itself carries only neutral (cwd, paths).
func (h *workspaceHub) observe(src FileChangeSource) {
	src.Observe(func(cwd string, paths []string) {
		h.publish(protocol.WorkspaceEvent{
			Type:  protocol.WorkspaceEventFilesChanged,
			Cwd:   cwd,
			Paths: paths,
		})
	})
}

// publish fans ev to every subscriber, dropping it for any whose buffer is
// full (lossy by design — see the type doc).
func (h *workspaceHub) publish(ev protocol.WorkspaceEvent) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for sub := range h.subs {
		h.sendLocked(sub, ev)
	}
}

// publishTo sends a subscription-local event through the same serialization
// point as broadcasts. This keeps each subscriber's sequence strictly ordered
// even when its git watcher races a global workspace event.
func (h *workspaceHub) publishTo(sub *workspaceSubscription, ev protocol.WorkspaceEvent) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, registered := h.subs[sub]; registered {
		h.sendLocked(sub, ev)
	}
}

func (*workspaceHub) sendLocked(sub *workspaceSubscription, ev protocol.WorkspaceEvent) {
	sub.sequence++
	ev = cloneWorkspaceEvent(ev)
	ev.Sequence = sub.sequence
	select {
	case sub.events <- ev:
	default: // full subscriber: drop; the sequence gap tells it to re-fetch
	}
}

// cloneWorkspaceEvent gives each subscription sole ownership of every mutable
// field. The hub sends asynchronously-consumed values; sharing the producer's
// slices or pointers would let a caller reuse them while a transport is still
// encoding the event, and sharing between subscriptions would let one
// in-process consumer corrupt another's frame.
func cloneWorkspaceEvent(ev protocol.WorkspaceEvent) protocol.WorkspaceEvent {
	ev.Paths = slices.Clone(ev.Paths)
	if ev.ToolCount != nil {
		toolCount := *ev.ToolCount
		ev.ToolCount = &toolCount
	}
	if ev.Error != nil {
		problem := *ev.Error
		problem.Errors = slices.Clone(problem.Errors)
		ev.Error = &problem
	}
	return ev
}

// WorkspaceSubscribe opens the workspace event stream (AUX_API §3.1). The
// stream's lifetime is the request ctx: it ends on client disconnect and on
// server shutdown (the transport force-closes the connection, canceling the
// request), at which point the cleanup below runs and the transport's own
// shutdown joins this still-active handler. Broadcast events (mcp.serverChanged,
// skills.changed) go to every subscription; when the request carries watches, the
// subscription also monitors those cwds' git state and emits a debounced resync
// on any change (commit / stage / checkout / merge) — the client then re-fetches
// workspace.getDiff. (Working-tree file edits aren't watched directly — see
// gitWatcher; the agent's own edits arrive as files.changed from its tools.)
func (s *Server) WorkspaceSubscribe(ctx context.Context, in protocol.WorkspaceSubscribeRequest) (*protocol.WorkspaceSubscribeResponse, <-chan protocol.WorkspaceEvent, error) {
	gitDirs, err := s.resolveWatchGitDirs(in.Watches)
	if err != nil {
		return nil, nil, err
	}

	// WorkspaceSubscribe owns the channel: the hub broadcasts to it and (when
	// watches are present) the git watcher emits to it. Closing it only after
	// the watcher has stopped keeps emit from racing the close.
	out := make(chan protocol.WorkspaceEvent, 64)
	subscription, unregister, registered := s.wsHub.register(out)
	if !registered {
		close(out)
		return nil, nil, errServerClosed
	}

	var watcher *gitWatcher
	if len(gitDirs) > 0 {
		watcher, err = startGitWatcher(gitDirs, func(ev protocol.WorkspaceEvent) {
			s.wsHub.publishTo(subscription, ev)
		})
		if err != nil {
			unregister()
			close(out)
			return nil, nil, fmt.Errorf("workspace.subscribe: start git watcher: %w", err)
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

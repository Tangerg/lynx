package server

import "github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"

// subscribe is a broadcast-only convenience used by the workspace-hub tests: a
// hub-owned channel + an unsubscribe that unregisters AND closes it. Production
// subscriptions (WorkspaceSubscribe) own their channel via register instead, so
// they can close it only after stopping the git watcher — hence this lives in
// test, where the broadcast-only shape is all the tests need.
func (h *workspaceHub) subscribe() (<-chan protocol.WorkspaceEvent, func()) {
	ch := make(chan protocol.WorkspaceEvent, 64)
	unregister := h.register(ch)
	return ch, func() {
		unregister()
		close(ch)
	}
}

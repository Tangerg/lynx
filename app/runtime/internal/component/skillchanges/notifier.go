// Package skillchanges carries durable skill-library changes from the workspace
// application use case to the delivery consumer that republishes them as a
// workspace refresh event.
package skillchanges

import "sync"

// Notifier is the composition-root bridge between a successful skill-library
// mutation and the one delivery observer. The signal is intentionally payload-
// free: it means only "re-fetch the skill views". It is lossy before Observe,
// which is correct for a refresh nudge and avoids persisting transport state in
// the application layer.
type Notifier struct {
	mu   sync.RWMutex
	sink func()
}

// Publish forwards a skill-library refresh nudge to the installed observer.
func (n *Notifier) Publish() {
	n.mu.RLock()
	sink := n.sink
	n.mu.RUnlock()
	if sink != nil {
		sink()
	}
}

// Observe installs the consumer for future refresh nudges, replacing any prior
// observer.
func (n *Notifier) Observe(sink func()) {
	n.mu.Lock()
	n.sink = sink
	n.mu.Unlock()
}

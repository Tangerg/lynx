package schedules

import "sync"

// FireNotifier carries accepted scheduled-run notifications to the one delivery
// observer that projects them onto the workspace stream. It deliberately drops
// notifications published before an observer is installed: durable schedule and
// session state remains the source of truth, while this is only a refetch nudge.
type FireNotifier struct {
	mu   sync.RWMutex
	sink func(scheduleID string)
}

// Publish forwards an accepted firing to the installed observer. Safe for
// concurrent use.
func (n *FireNotifier) Publish(scheduleID string) {
	n.mu.RLock()
	sink := n.sink
	n.mu.RUnlock()
	if sink != nil {
		sink(scheduleID)
	}
}

// Observe installs the consumer for subsequent accepted firings, replacing a
// prior consumer.
func (n *FireNotifier) Observe(sink func(scheduleID string)) {
	n.mu.Lock()
	n.sink = sink
	n.mu.Unlock()
}

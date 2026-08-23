package goalflow

import (
	"slices"
	"sync"
)

// Signals is a lossless coalescing wakeup channel. Durable Goal rows remain
// truth; a wake carries only session identities whose rows should be reread.
type Signals struct {
	mu      sync.Mutex
	pending map[string]struct{}
	wake    chan struct{}
}

func NewSignals() *Signals {
	return &Signals{pending: make(map[string]struct{}), wake: make(chan struct{}, 1)}
}

func (signals *Signals) Publish(sessionID string) {
	if signals == nil || sessionID == "" {
		return
	}
	signals.mu.Lock()
	signals.pending[sessionID] = struct{}{}
	signals.mu.Unlock()
	select {
	case signals.wake <- struct{}{}:
	default:
	}
}

func (signals *Signals) Wake() <-chan struct{} { return signals.wake }

func (signals *Signals) Drain() []string {
	if signals == nil {
		return nil
	}
	signals.mu.Lock()
	values := make([]string, 0, len(signals.pending))
	for sessionID := range signals.pending {
		values = append(values, sessionID)
	}
	clear(signals.pending)
	signals.mu.Unlock()
	slices.Sort(values)
	return values
}

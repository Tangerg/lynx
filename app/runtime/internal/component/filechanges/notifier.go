// Package filechanges carries the run pump's live file-change nudges from the
// application producer to the single delivery consumer that republishes them as
// wire workspace events.
package filechanges

import "sync"

// Notifier is the composition-root pipeline (§2.5) between the run segment's
// file-change nudges (Publish, invoked by the runs.Effects adapter) and the one
// delivery consumer that maps them to protocol workspace events (Observe,
// installed by the delivery Server). It exists to break the construction cycle
// that would otherwise force the run coordinator to be built inside the delivery
// Server: the coordinator's durable effects need a publish sink, but that sink
// (the delivery workspace hub) is constructed after the coordinator.
//
// Lossy + single-sink by design: a nudge published before Observe, or with no
// observer, is dropped — a workspace nudge is "changed → re-fetch", so a drop
// self-heals on the next change. The one production consumer is the delivery
// workspace hub; Observe replaces the sink rather than fanning out.
type Notifier struct {
	mu   sync.RWMutex
	sink func(cwd string, paths []string)
}

// Publish forwards a file-change nudge to the installed observer, or drops it
// when none is installed. Safe for concurrent use.
func (n *Notifier) Publish(cwd string, paths []string) {
	n.mu.RLock()
	sink := n.sink
	n.mu.RUnlock()
	if sink != nil {
		sink(cwd, paths)
	}
}

// Observe installs the consumer that receives every subsequent Publish,
// replacing any prior observer.
func (n *Notifier) Observe(sink func(cwd string, paths []string)) {
	n.mu.Lock()
	n.sink = sink
	n.mu.Unlock()
}

package runs

import "iter"

// consumeEvents drains a finite test stream when the scenario asserts the
// resulting durable state rather than individual event payloads.
func consumeEvents(events iter.Seq[Event]) {
	for event := range events {
		_ = event
	}
}

// consumePulledEvents drains the remainder of a stream already opened with
// iter.Pull. The caller remains responsible for invoking the paired stop.
func consumePulledEvents(next func() (Event, bool)) {
	for {
		event, available := next()
		if !available {
			return
		}
		_ = event
	}
}

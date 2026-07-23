package schedules

import "testing"

func TestFireNotifierDeliversOnlyAfterObservation(t *testing.T) {
	var notifier FireNotifier
	notifier.Publish("sch_dropped")

	var received []string
	notifier.Observe(func(id string) { received = append(received, id) })
	notifier.Publish("sch_accepted")

	if len(received) != 1 || received[0] != "sch_accepted" {
		t.Fatalf("received = %v, want only accepted notification", received)
	}
}

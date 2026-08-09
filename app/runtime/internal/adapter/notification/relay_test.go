package notification

import "testing"

func TestRelayForwardsToCurrentObserver(t *testing.T) {
	var relay Relay[int]
	var observed []int

	relay.Publish(0)
	relay.Observe(func(value int) { observed = append(observed, value) })
	relay.Publish(1)
	relay.Observe(func(value int) { observed = append(observed, value*10) })
	relay.Publish(2)

	if len(observed) != 2 || observed[0] != 1 || observed[1] != 20 {
		t.Fatalf("observed = %v, want [1 20]", observed)
	}
}

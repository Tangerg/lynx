package bootstrap

import (
	"sync"
	"testing"
)

func TestNotificationRelayForwardsToCurrentObserver(t *testing.T) {
	relay := newNotificationRelay[int]()

	relay.Publish(1)
	got := 0
	relay.Observe(func(value int) { got += value })
	relay.Publish(2)
	relay.Observe(func(value int) { got += 10 * value })
	relay.Publish(3)

	if got != 32 {
		t.Fatalf("observed value = %d, want 32", got)
	}
}

func TestNotificationRelayIsSafeForConcurrentPublication(t *testing.T) {
	relay := newNotificationRelay[int]()
	var mu sync.Mutex
	total := 0
	relay.Observe(func(value int) {
		mu.Lock()
		total += value
		mu.Unlock()
	})

	const publishers = 32
	var wait sync.WaitGroup
	wait.Add(publishers)
	for range publishers {
		go func() {
			defer wait.Done()
			relay.Publish(1)
		}()
	}
	wait.Wait()

	mu.Lock()
	defer mu.Unlock()
	if total != publishers {
		t.Fatalf("observed total = %d, want %d", total, publishers)
	}
}

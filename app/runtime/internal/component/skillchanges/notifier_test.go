package skillchanges

import "testing"

func TestNotifierDeliversOnlyAfterObserverInstalled(t *testing.T) {
	notifier := new(Notifier)
	called := 0
	notifier.Publish()
	notifier.Observe(func() { called++ })
	notifier.Publish()

	if called != 1 {
		t.Fatalf("observer calls = %d, want 1", called)
	}
}

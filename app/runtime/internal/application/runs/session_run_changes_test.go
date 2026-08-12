package runs

import "testing"

func TestSessionRunChangesWakesAllObserversAndReleasesGeneration(t *testing.T) {
	var changes sessionRunChanges

	first, stopFirst := changes.observe("ses_1")
	second, stopSecond := changes.observe("ses_1")

	changes.notify("ses_1")

	assertClosed(t, first)
	assertClosed(t, second)
	stopFirst()
	stopSecond()
	stopSecond()

	if got := len(changes.sessions); got != 0 {
		t.Fatalf("retained notified generation = %d, want 0", got)
	}

	next, stopNext := changes.observe("ses_1")
	select {
	case <-next:
		t.Fatal("new observation inherited the previous generation's notification")
	default:
	}
	stopNext()

	if got := len(changes.sessions); got != 0 {
		t.Fatalf("retained replacement generation = %d, want 0", got)
	}
}

func TestSessionRunChangesKeepsGenerationUntilLastObserverStops(t *testing.T) {
	var changes sessionRunChanges

	_, stopFirst := changes.observe("ses_1")
	second, stopSecond := changes.observe("ses_1")
	stopFirst()

	if got := len(changes.sessions); got != 1 {
		t.Fatalf("observed generations after first stop = %d, want 1", got)
	}

	changes.notify("ses_1")
	assertClosed(t, second)
	stopSecond()

	if got := len(changes.sessions); got != 0 {
		t.Fatalf("retained last-observer generation = %d, want 0", got)
	}
}

func assertClosed(t *testing.T, changed <-chan struct{}) {
	t.Helper()
	select {
	case <-changed:
	default:
		t.Fatal("change observation was not notified")
	}
}

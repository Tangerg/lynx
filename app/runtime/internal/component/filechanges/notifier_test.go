package filechanges

import (
	"slices"
	"testing"
)

func TestNotifierSnapshotsPublishedPaths(t *testing.T) {
	notifier := new(Notifier)
	var observed []string
	notifier.Observe(func(_ string, paths []string) {
		observed = paths
	})

	paths := []string{"a.go", "b.go"}
	notifier.Publish("/repo", paths)
	paths[0] = "mutated.go"

	if !slices.Equal(observed, []string{"a.go", "b.go"}) {
		t.Fatalf("observed paths = %v, want a stable publish snapshot", observed)
	}
}

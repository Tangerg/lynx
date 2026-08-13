package persistence

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
	"github.com/Tangerg/lynx/app/runtime/internal/testsupport/sessionfixture"
)

func TestObserveExternalChangesIgnoresLocalAndReportsOtherRuntimeCommit(t *testing.T) {
	root := t.TempDir()
	config := Config{DataDirectory: filepath.Join(root, "data"), DefaultWorkspacePath: root}
	first, err := Open(t.Context(), config)
	if err != nil {
		t.Fatalf("Open first: %v", err)
	}
	t.Cleanup(func() { _ = first.Close() })
	second, err := Open(t.Context(), config)
	if err != nil {
		t.Fatalf("Open second: %v", err)
	}
	t.Cleanup(func() { _ = second.Close() })

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	changed := make(chan struct{}, 4)
	done, err := first.StartExternalChangeObserver(ctx, func() { changed <- struct{}{} })
	if err != nil {
		t.Fatalf("start external change observer: %v", err)
	}
	local := sessionfixture.MustRestore(session.Snapshot{ID: "session-local", CWD: root})
	if err := first.Sessions.Insert(t.Context(), local); err != nil {
		t.Fatalf("insert through observed Runtime: %v", err)
	}
	select {
	case <-changed:
		t.Fatal("local commit produced an external-change notification")
	case <-time.After(2 * externalChangePollInterval):
	}

	remote := sessionfixture.MustRestore(session.Snapshot{ID: "session-remote", CWD: root})
	if err := second.Sessions.Insert(t.Context(), remote); err != nil {
		t.Fatalf("insert through second Runtime: %v", err)
	}
	select {
	case <-changed:
	case <-time.After(10 * externalChangePollInterval):
		t.Fatal("other Runtime commit did not trigger convergence")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("observer did not stop with its context")
	}
}

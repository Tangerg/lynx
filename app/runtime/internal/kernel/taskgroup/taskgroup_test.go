package taskgroup

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestGroupCloseCancelsAndWaits(t *testing.T) {
	var tasks Group
	started := make(chan struct{})
	stopped := make(chan struct{})
	if !tasks.Start(context.Background(), func(ctx context.Context) {
		close(started)
		<-ctx.Done()
		close(stopped)
	}) {
		t.Fatal("Start rejected before Close")
	}
	<-started

	tasks.Close()
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("Close returned before task observed cancellation")
	}
	if tasks.Start(context.Background(), func(context.Context) {}) {
		t.Fatal("Start accepted a task after Close")
	}
}

func TestGroupStartRacesClose(t *testing.T) {
	for range 25 {
		var tasks Group
		var starters sync.WaitGroup
		for range 16 {
			starters.Add(1)
			go func() {
				defer starters.Done()
				tasks.Start(context.Background(), func(ctx context.Context) {
					<-ctx.Done()
				})
			}()
		}
		closed := make(chan struct{})
		go func() {
			tasks.Close()
			close(closed)
		}()
		starters.Wait()
		select {
		case <-closed:
		case <-time.After(time.Second):
			t.Fatal("Close did not drain concurrently-started tasks")
		}
	}
}

func TestGroupDetachesRequestCancellationAndKeepsValues(t *testing.T) {
	type contextKey struct{}
	parent, cancel := context.WithCancel(context.WithValue(context.Background(), contextKey{}, "trace"))
	cancel()

	var tasks Group
	result := make(chan bool, 1)
	if !tasks.Start(parent, func(ctx context.Context) {
		result <- ctx.Err() == nil && ctx.Value(contextKey{}) == "trace"
	}) {
		t.Fatal("Start rejected")
	}
	if !<-result {
		t.Fatal("task context did not detach cancellation while preserving values")
	}
	tasks.Close()
}

func TestGroupAttachIsCanceledByCloseAndReleaseIsIdempotent(t *testing.T) {
	var tasks Group
	ctx, release, ok := tasks.Attach(context.Background())
	if !ok {
		t.Fatal("Attach rejected before Close")
	}
	done := make(chan struct{})
	go func() {
		<-ctx.Done()
		release()
		release()
		close(done)
	}()
	tasks.Close()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Close did not cancel and join attached work")
	}
	if _, _, ok := tasks.Attach(context.Background()); ok {
		t.Fatal("Attach accepted work after Close")
	}
}

func TestGroupAttachLinkedKeepsParentCancellation(t *testing.T) {
	var tasks Group
	parent, cancelParent := context.WithCancel(context.Background())
	ctx, release, ok := tasks.AttachLinked(parent)
	if !ok {
		t.Fatal("AttachLinked rejected before Close")
	}
	cancelParent()
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("linked context ignored parent cancellation")
	}
	release()
	tasks.Close()
}

package inprocess

import (
	"context"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/dispatch"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/transport"
)

func TestPumpStreamStopsWhenCallCanceledUnderBackpressure(t *testing.T) {
	blocker, err := transport.NewNotification("test.blocker", nil)
	if err != nil {
		t.Fatal(err)
	}
	notif, err := transport.NewNotification("test.event", nil)
	if err != nil {
		t.Fatal(err)
	}
	tp := &Transport{
		in:    make(chan transport.Message, 1),
		close: make(chan struct{}),
		done:  make(chan struct{}),
	}
	tp.in <- blocker

	ctx, cancel := context.WithCancel(t.Context())
	inCh := make(chan dispatch.StreamFrame)
	events := func(yield func(dispatch.StreamFrame) bool) {
		for f := range inCh {
			if !yield(f) {
				return
			}
		}
	}
	done := make(chan struct{})
	go func() {
		tp.pumpStream(ctx, events)
		close(done)
	}()
	frameReceived := make(chan struct{})
	go func() {
		inCh <- dispatch.StreamFrame{Notification: notif}
		close(frameReceived)
	}()
	<-frameReceived
	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("stream pump remained blocked on Recv after its call context was canceled")
	}
	tp.BeginShutdown()
	if err := tp.AwaitShutdown(t.Context()); err != nil {
		t.Fatalf("AwaitShutdown: %v", err)
	}
}

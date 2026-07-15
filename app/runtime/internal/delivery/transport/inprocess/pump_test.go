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
	}
	tp.in <- blocker

	ctx, cancel := context.WithCancel(t.Context())
	events := make(chan dispatch.StreamFrame)
	done := make(chan struct{})
	go func() {
		tp.pumpStream(ctx, events)
		close(done)
	}()
	frameReceived := make(chan struct{})
	go func() {
		events <- dispatch.StreamFrame{Notif: notif}
		close(frameReceived)
	}()
	<-frameReceived
	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("stream pump remained blocked on Recv after its call context was canceled")
	}
	if err := tp.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

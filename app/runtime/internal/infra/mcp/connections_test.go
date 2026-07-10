package mcp

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestConnectionsRejectMutationsAfterClose(t *testing.T) {
	c := &Connections{client: newClient()}
	if err := c.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	cfg := ServerConfig{Name: "closed", Transport: TransportHTTP, Endpoint: "https://example.invalid"}
	for name, call := range map[string]func() error{
		"configure": func() error { return c.Configure(context.Background(), cfg) },
		"reconnect": func() error { return c.Reconnect(context.Background(), cfg.Name) },
		"authorize": func() error { return c.Authorize(context.Background(), cfg.Name) },
	} {
		t.Run(name, func(t *testing.T) {
			if err := call(); !errors.Is(err, ErrConnectionsClosed) {
				t.Fatalf("error = %v, want ErrConnectionsClosed", err)
			}
		})
	}

	c.Remove(context.Background(), cfg.Name)
	if got := c.Statuses(); len(got) != 0 {
		t.Fatalf("statuses after Close + Remove = %v, want empty", got)
	}
	if err := c.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}

func TestConnectionsCloseSerializesWithMutations(t *testing.T) {
	c := &Connections{}
	c.reconnectMu.Lock()
	done := make(chan error, 1)
	go func() { done <- c.Close() }()

	select {
	case err := <-done:
		t.Fatalf("Close returned before the active mutation released its lock: %v", err)
	case <-time.After(20 * time.Millisecond):
	}

	c.reconnectMu.Unlock()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Close: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Close did not finish after the mutation lock was released")
	}
}

func TestCloneServerConfigOwnsMutableFields(t *testing.T) {
	original := ServerConfig{
		Args:    []string{"one"},
		Env:     []string{"A=1"},
		Headers: map[string]string{"X-Test": "before"},
	}
	cloned := cloneServerConfig(original)
	original.Args[0] = "two"
	original.Env[0] = "A=2"
	original.Headers["X-Test"] = "after"

	if cloned.Args[0] != "one" || cloned.Env[0] != "A=1" || cloned.Headers["X-Test"] != "before" {
		t.Fatalf("clone retained caller-owned storage: %+v", cloned)
	}
}

package core

import (
	"errors"
	"fmt"
	"io"
	"sync"
)

// OutputChannel is the action-level "say something to the user" sink. It's
// modeled as an interface so tests can capture output, real apps can wire it
// to a Server-Sent-Events stream, and library code never assumes anything
// stronger than "write a string".
//
// Both write methods return an error so implementations backed by a real
// transport (file, network socket, bounded channel, …) can surface I/O
// failures rather than silently dropping data.
type OutputChannel interface {
	Write(msg string) error
	WriteTyped(topic string, payload any) error
	Close() error
}

// DevNullOutputChannel discards everything — the runtime's default when no
// channel is configured.
var DevNullOutputChannel OutputChannel = devNullChannel{}

type devNullChannel struct{}

func (devNullChannel) Write(string) error           { return nil }
func (devNullChannel) WriteTyped(string, any) error { return nil }
func (devNullChannel) Close() error                 { return nil }

// WriterOutputChannel adapts any io.Writer (os.Stdout, a buffer, ...) into the
// OutputChannel surface. Typed payloads are formatted as "[topic] %+v\n".
// Errors from the underlying writer propagate to the caller.
type WriterOutputChannel struct {
	mu sync.Mutex
	w  io.Writer
}

func NewWriterOutputChannel(w io.Writer) *WriterOutputChannel {
	return &WriterOutputChannel{w: w}
}

func (c *WriterOutputChannel) Write(msg string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, err := fmt.Fprintln(c.w, msg)
	return err
}

func (c *WriterOutputChannel) WriteTyped(topic string, payload any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, err := fmt.Fprintf(c.w, "[%s] %+v\n", topic, payload)
	return err
}

func (c *WriterOutputChannel) Close() error {
	if cl, ok := c.w.(io.Closer); ok {
		return cl.Close()
	}
	return nil
}

// errOutputChannelClosed is returned by [ChannelOutputChannel] writes after
// the channel has been closed. Callers can [errors.Is] against it to
// distinguish "downstream gone away" from other I/O failures.
var errOutputChannelClosed = errors.New("core.OutputChannel: channel is closed")

// ChannelOutputChannel forwards plain-text writes onto a Go channel — useful
// when the caller wants backpressure or selective consumption. Typed writes
// are serialized with %v formatting; structured forwarding is not in scope.
//
// A send on a closed underlying channel would panic; the wrapper recovers
// from that and returns [errOutputChannelClosed] instead so action code
// doesn't have to bake panic-recovery into every write.
type ChannelOutputChannel struct {
	ch     chan<- string
	mu     sync.Mutex
	closed bool
}

func NewChannelOutputChannel(ch chan<- string) *ChannelOutputChannel {
	return &ChannelOutputChannel{ch: ch}
}

func (c *ChannelOutputChannel) Write(msg string) error {
	return c.send(msg)
}

func (c *ChannelOutputChannel) WriteTyped(topic string, p any) error {
	return c.send(fmt.Sprintf("[%s] %v", topic, p))
}

func (c *ChannelOutputChannel) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil
	}
	close(c.ch)
	c.closed = true
	return nil
}

// send centralises the panic-on-closed-channel guard so both write paths
// expose the same behaviour.
func (c *ChannelOutputChannel) send(msg string) (err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return errOutputChannelClosed
	}
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%w: %v", errOutputChannelClosed, r)
		}
	}()
	c.ch <- msg
	return nil
}

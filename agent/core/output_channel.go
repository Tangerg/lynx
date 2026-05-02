package core

import (
	"fmt"
	"io"
	"sync"
)

// OutputChannel is the action-level "say something to the user" sink. It's
// modeled as an interface so tests can capture output, real apps can wire it
// to a Server-Sent-Events stream, and library code never assumes anything
// stronger than "write a string".
type OutputChannel interface {
	Write(msg string)
	WriteTyped(topic string, payload any)
	Close() error
}

// DevNullOutputChannel discards everything — the runtime's default when no
// channel is configured.
var DevNullOutputChannel OutputChannel = devNullChannel{}

type devNullChannel struct{}

func (devNullChannel) Write(string)              {}
func (devNullChannel) WriteTyped(string, any)    {}
func (devNullChannel) Close() error              { return nil }

// WriterOutputChannel adapts any io.Writer (os.Stdout, a buffer, ...) into the
// OutputChannel surface. Typed payloads are formatted as "[topic] %+v\n".
type WriterOutputChannel struct {
	mu sync.Mutex
	w  io.Writer
}

func NewWriterOutputChannel(w io.Writer) *WriterOutputChannel {
	return &WriterOutputChannel{w: w}
}

func (c *WriterOutputChannel) Write(msg string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	fmt.Fprintln(c.w, msg)
}

func (c *WriterOutputChannel) WriteTyped(topic string, payload any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	fmt.Fprintf(c.w, "[%s] %+v\n", topic, payload)
}

func (c *WriterOutputChannel) Close() error {
	if cl, ok := c.w.(io.Closer); ok {
		return cl.Close()
	}
	return nil
}

// ChannelOutputChannel forwards plain-text writes onto a Go channel — useful
// when the caller wants backpressure or selective consumption. Typed writes
// are serialized with %v formatting; structured forwarding is not in scope.
type ChannelOutputChannel struct {
	ch chan<- string
}

func NewChannelOutputChannel(ch chan<- string) *ChannelOutputChannel {
	return &ChannelOutputChannel{ch: ch}
}

func (c *ChannelOutputChannel) Write(msg string)              { c.ch <- msg }
func (c *ChannelOutputChannel) WriteTyped(topic string, p any) { c.ch <- fmt.Sprintf("[%s] %v", topic, p) }
func (c *ChannelOutputChannel) Close() error                  { close(c.ch); return nil }

package sse

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// heartBeatPing is the byte sequence sent as a keep-alive ping message to clients.
// The colon prefix indicates a comment line in SSE protocol, which clients will ignore
// but will keep the connection alive.
var heartBeatPing = []byte(":ping")

// Writer provides functionality for sending Server-Sent Events (SSE) to clients.
// It handles the complexities of SSE stream management including:
// - Asynchronous message processing
// - Proper connection maintenance with heartbeats
// - Graceful shutdown handling
// - Error tracking and propagation
// The Writer is designed to be created once per client connection.
type Writer struct {
	isShutdown     atomic.Bool         // Flag to signal shutdown state
	lastError      error               // Stores the last error that occurred
	waitGroup      sync.WaitGroup      // Tracks active goroutines for graceful shutdown
	ctx            context.Context     // Context for cancellation control
	messageQueue   chan *Message       // Channel for queueing messages to be sent
	messageEncoder *messageEncoder     // Converts Message objects to SSE wire format
	httpResponse   http.ResponseWriter // HTTP response writer for the connection
	httpFlusher    http.Flusher        // Interface for flushing buffered data to client
	ticker         *time.Ticker        // Timer for sending periodic heartbeats
	shutDownSignal chan struct{}
}

// WriterConfig contains the configuration options for creating a new SSE Writer.
// All fields have sensible defaults except for Context and ResponseWriter which are required.
type WriterConfig struct {
	Context        context.Context     // Required: Controls the lifecycle of the Writer
	ResponseWriter http.ResponseWriter // Required: The HTTP response to write SSE data to
	QueueSize      int                 // Optional: Size of the message queue buffer (default: 64)
	HeartBeat      time.Duration       // Optional: Interval for sending heartbeat pings (default: 30s)
}

// validate checks that the WriterConfig contains valid settings and sets
// default values where appropriate. It returns an error if any required field
// is missing or invalid.
//
// Validation rules:
// - Context must not be nil
// - ResponseWriter must not be nil and must implement http.Flusher
// - QueueSize defaults to 64 if unspecified
// - HeartBeat defaults to 30 seconds if unspecified
func (c *WriterConfig) validate() error {
	if c.Context == nil {
		return errors.New("missing context")
	}
	if c.ResponseWriter == nil {
		return errors.New("missing responseWriter")
	}
	_, ok := c.ResponseWriter.(http.Flusher)
	if !ok {
		return errors.New("httpResponse does not implement http.Flusher")
	}
	if c.QueueSize == 0 {
		c.QueueSize = 64
	}
	if c.HeartBeat == 0 {
		c.HeartBeat = 30 * time.Second
	}
	return nil
}

// NewWriter creates and initializes a new SSE Writer with the given configuration.
// It validates the configuration, sets up the internal state, and starts the
// background processing goroutine to handle message sending.
//
// Returns:
// - A pointer to the initialized Writer
// - An error if the configuration is invalid
//
// Example usage:
//
//	conf := &sse.WriterConfig{
//	    Context:        r.Context(),
//	    ResponseWriter: w,
//	    HeartBeat:      15 * time.Second,
//	}
//	writer, err := sse.NewWriter(conf)
//	if err != nil {
//	    // handle error
//	}
//	defer writer.Shutdown()
//
//	// Send messages with writer.Send(), writer.SendData(), etc.
func NewWriter(conf *WriterConfig) (*Writer, error) {
	err := conf.validate()
	if err != nil {
		return nil, errors.New("httpResponse does not implement http.Flusher")
	}

	w := &Writer{
		ctx:            conf.Context,
		messageQueue:   make(chan *Message, conf.QueueSize),
		messageEncoder: newMessageEncoder(),
		httpResponse:   conf.ResponseWriter,
		httpFlusher:    conf.ResponseWriter.(http.Flusher),
		ticker:         time.NewTicker(conf.HeartBeat),
		shutDownSignal: make(chan struct{}),
	}
	w.run()
	return w, nil
}

// run initializes the SSE stream by setting appropriate HTTP headers
// and starts the background message processing goroutine.
// This is called automatically by NewWriter.
func (w *Writer) run() {
	SetSSEHeaders(w.httpResponse.Header())
	w.waitGroup.Add(1)
	go w.process()
}

// Shutdown gracefully stops the Writer's background processing and closes resources.
// It waits for any in-flight messages to be processed before returning.
//
// This method:
// - Sets the shutdown flag to prevent new messages from being processed
// - Waits for the background goroutine to complete
// - Closes the message queue channel
// - Returns any error that occurred during processing
//
// It is safe to call Shutdown multiple times; subsequent calls will have no effect.
// Clients should always call Shutdown when finished with the Writer to free resources.
func (w *Writer) Shutdown() error {
	if w.isShutdown.Load() {
		return nil
	}
	w.isShutdown.Store(true)
	w.shutDownSignal <- struct{}{}
	w.waitGroup.Wait()
	close(w.messageQueue)
	close(w.shutDownSignal)
	return w.lastError
}

// process is the main event loop that runs in a background goroutine.
// It continuously processes messages from the queue and heartbeat signals
// until the Writer is shut down or an error occurs.
//
// The event loop handles:
// - Context cancellation
// - Periodic heartbeats to keep the connection alive
// - Processing and sending messages from the queue
//
// If any error occurs during processing, it's stored in lastError
// and the goroutine exits.
func (w *Writer) process() {
	defer w.waitGroup.Done()
	defer w.ticker.Stop()

	for !w.isShutdown.Load() {
		select {
		case <-w.shutDownSignal:
			return
		case <-w.ctx.Done():
			w.lastError = w.ctx.Err()
			return
		case <-w.ticker.C:
			_, _ = w.httpResponse.Write(heartBeatPing)
			w.httpFlusher.Flush()
		case event, ok := <-w.messageQueue:
			if !ok {
				_, w.lastError = w.httpResponse.Write(byteLFLF)
				if w.lastError != nil {
					return
				}
				w.httpFlusher.Flush()
				w.lastError = w.ctx.Err()
				return
			}

			encode, err := w.messageEncoder.Encode(event)
			if err != nil {
				w.lastError = err
				return
			}
			_, w.lastError = w.httpResponse.Write(encode)
			if w.lastError != nil {
				return
			}
			w.httpFlusher.Flush()
		}
	}
}

// Send queues a message to be sent to the client.
// The message is added to the message queue and will be processed
// asynchronously by the background goroutine.
//
// This method is non-blocking unless the message queue is full.
//
// Parameters:
//   - msg: The SSE message to send with fields like ID, Event, Data, and Retry
func (w *Writer) Send(msg *Message) {
	w.messageQueue <- msg
}

// SendID sends a message containing only an ID field.
// This is useful for updating the last event ID on the client without
// sending any data, which can be used for resuming connections.
//
// Parameters:
//   - id: The event ID to send
func (w *Writer) SendID(id string) {
	w.Send(&Message{
		ID: id,
	})
}

// SendEvent sends a message containing only an Event field.
// This is useful for sending event type notifications without data.
// Clients can listen for specific event types using EventSource.addEventListener().
//
// Parameters:
//   - event: The event type to send
func (w *Writer) SendEvent(event string) {
	w.Send(&Message{
		Event: event,
	})
}

// SendData sends a message containing data serialized to JSON.
// The provided value is JSON-marshaled and sent as the Data field of a message.
// If JSON marshaling fails, the method silently returns without sending anything.
//
// Parameters:
//   - data: Any value that can be marshaled to JSON
//
// Note: This is a convenience method for sending structured data. For raw data,
// use Send() with a manually constructed Message.
func (w *Writer) SendData(data any) {
	marshal, err := json.Marshal(data)
	if err != nil {
		return
	}
	w.Send(&Message{
		Data: marshal,
	})
}

// Error returns any error that occurred during SSE stream processing.
// This can be used to check if the connection was closed due to an error.
// The error will be nil if no error has occurred.
//
// Common errors include:
// - Context cancellation errors
// - Network connection errors
// - Message encoding errors
func (w *Writer) Error() error {
	return w.lastError
}

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

// `heartBeatPing` is the keep-alive ping message sent to clients.
// Comments in SSE start with a colon `:`, meaning they are ignored by the client but maintain the connection.
var heartBeatPing = []byte(delimiter + whitespace + "ping" + string(byteLFLF)) // ": ping\n\n"

// WriterConfig contains the configuration options for creating a new SSE Writer.
// All fields have reasonable defaults, except for Context and ResponseWriter which are required.
type WriterConfig struct {
	Context        context.Context     // Required: Controls the lifecycle of the Writer.
	ResponseWriter http.ResponseWriter // Required: HTTP response writer to send SSE data.
	QueueSize      int                 // Optional: Size of the message queue buffer (default: 64).
	HeartBeat      time.Duration       // Optional: Interval for sending heartbeat pings (default: disabled if 0).
}

// validate checks the validity of the WriterConfig settings and configures defaults when needed.
// Returns an error if any required parameter is missing or invalid.
//
// Validation Rules:
// - Context must not be nil.
// - ResponseWriter must not be nil and must implement http.Flusher.
// - QueueSize defaults to 64 if not provided.
func (c *WriterConfig) validate() error {
	if c.Context == nil {
		return errors.New("missing context")
	}
	if c.ResponseWriter == nil {
		return errors.New("missing responseWriter")
	}
	_, ok := c.ResponseWriter.(http.Flusher)
	if !ok {
		return errors.New("responseWriter does not implement http.Flusher")
	}
	if c.QueueSize <= 0 {
		c.QueueSize = 64
	}
	return nil
}

// Writer encapsulates the logic for sending Server-Sent Events (SSE) to clients.
// It provides features like:
// - Asynchronous message processing.
// - Connection maintenance with heartbeats.
// - Graceful shutdown handling.
// - Error tracking and propagation.
// A Writer is intended to be tied to a single client connection.
type Writer struct {
	config         *WriterConfig
	isClosed       atomic.Bool         // Tracks if the writer has been closed.
	waitGroup      sync.WaitGroup      // Manages active goroutines for graceful shutdown.
	ctx            context.Context     // Context to control the writer's lifecycle.
	messageEncoder *Encoder            // Encodes messages in SSE-compliant format.
	httpResponse   http.ResponseWriter // HTTP response writer for client communication.
	httpFlusher    http.Flusher        // Handles flushing the response to the client.
	closeSignal    chan struct{}       // Channel signaling graceful shutdown.
	messageQueue   chan []byte         // Buffered message queue for asynchronous processing.
	errors         []error             // Stores any errors encountered during processing.
}

// NewWriter initializes and returns a new SSE Writer with the provided configuration.
// It validates the configuration, sets up the internal dependencies, and starts
// background processes for message handling.
//
// Returns:
// - A pointer to the initialized Writer instance.
// - An error if the configuration is invalid.
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
//	defer writer.Close()
//
//	// Send messages using writer.Send(), writer.SendData(), etc.
func NewWriter(config *WriterConfig) (*Writer, error) {
	err := config.validate()
	if err != nil {
		return nil, err // Return validation error if the configuration is invalid.
	}

	w := &Writer{
		config:         config,
		ctx:            config.Context,
		messageEncoder: NewEncoder(),
		httpResponse:   config.ResponseWriter,
		httpFlusher:    config.ResponseWriter.(http.Flusher),
		closeSignal:    make(chan struct{}),
		messageQueue:   make(chan []byte, config.QueueSize),
		errors:         make([]error, 0, config.QueueSize),
	}
	w.initialize()
	return w, nil
}

// initialize sets up the SSE response by configuring the necessary HTTP headers and
// starts background goroutines to process messages and send heartbeats.
// This is called automatically by NewWriter.
func (w *Writer) initialize() {
	SetSSEHeaders(w.httpResponse.Header())
	w.waitGroup.Add(2)
	go w.processMessageQueue()
	go w.startHeartbeatLoop()
}

// writeDataToClient sends raw data to the HTTP response writer and flushes it to the client.
// Returns an error if writing to the response fails.
func (w *Writer) writeDataToClient(data []byte) error {
	_, err := w.httpResponse.Write(data)
	if err != nil {
		return err
	}
	w.httpFlusher.Flush()
	return nil
}

// recordError adds an error to the Writer's error list. Skips if the error is nil.
func (w *Writer) recordError(err error) {
	if err == nil {
		return
	}
	w.errors = append(w.errors, err)
}

// sendHeartbeatNonBlocking attempts to send a heartbeat ping message to the message queue.
// It first checks if the writer is closed, and only attempts to send if the writer is still active.
// The send operation is non-blocking; if the queue is full, the message is dropped.
func (w *Writer) sendHeartbeatNonBlocking() {
	if w.isClosed.Load() {
		return
	}

	select {
	case w.messageQueue <- heartBeatPing:
	default:
	}
}

// startHeartbeatLoop sends periodic heartbeat messages to the client to keep the connection alive.
// The heartbeat message is a comment line (": ping\n\n") as per the SSE protocol.
func (w *Writer) startHeartbeatLoop() {
	defer w.waitGroup.Done()

	if w.config.HeartBeat <= 0 { // Heartbeat is disabled if duration is not set.
		return
	}

	ticker := time.NewTicker(w.config.HeartBeat)
	defer ticker.Stop()

	for {
		select {
		case <-w.closeSignal:
			return
		case <-w.ctx.Done():
			return
		case <-ticker.C:
			w.sendHeartbeatNonBlocking()
		}
	}
}

// drainMessageQueue closes the message queue and processes any remaining messages.
// This ensures all pending messages are sent to the client before shutting down.
// After all messages are processed, it sends a final double line feed to properly
// terminate the SSE stream according to the protocol.
// Any errors encountered during this process are collected.
func (w *Writer) drainMessageQueue() {
	close(w.messageQueue)
	for msg := range w.messageQueue {
		w.recordError(w.writeDataToClient(msg))
	}
	w.recordError(w.writeDataToClient(byteLFLF))
}

// processMessageQueue handles the message queue and sends messages to the client asynchronously.
// If the writer is closed or the context is canceled, it stops processing.
func (w *Writer) processMessageQueue() {
	defer w.waitGroup.Done()

	for {
		select {
		case <-w.closeSignal:
			w.drainMessageQueue()
			return
		case <-w.ctx.Done():
			w.recordError(w.ctx.Err())
			return
		case msg := <-w.messageQueue:
			w.recordError(w.writeDataToClient(msg))
		}
	}
}

// Close gracefully shuts down the Writer, ensuring all queued messages are processed
// and releasing resources. It blocks until all background goroutines complete.
//
// This method performs the following:
// - Signals background goroutines to shut down.
// - Sets the `isClosed` flag to prevent new message processing.
// - Waits for all active processes to finish.
// - Closes internal channels and queues.
// - Returns any errors encountered during operation.
//
// Calling Close multiple times is safe; subsequent calls are no-ops.
func (w *Writer) Close() error {
	if w.isClosed.Load() {
		return nil
	}

	w.isClosed.Store(true)
	close(w.closeSignal)

	w.waitGroup.Wait()
	return w.Error()
}

// Send enqueues a message to be delivered to the client.
//
// The method encodes the message in SSE protocol format and adds it to the queue
// to be processed by the background goroutines asynchronously. It is non-blocking
// unless the queue is full.
//
// Parameters:
//   - msg: The SSE Message to send, which may include fields like ID, Event, Data, and Retry.
func (w *Writer) Send(msg *Message) error {
	if w.isClosed.Load() {
		return nil
	}

	message, err := w.messageEncoder.Encode(msg)
	if err != nil {
		return err
	}
	w.messageQueue <- message
	return nil
}

// SendEvent sends an SSE message that includes only an Event field.
// This is useful for notifying clients of specific event types without data payloads.
//
// Parameters:
//   - event: Identifier for the event type, which clients can listen for using EventSource.addEventListener().
func (w *Writer) SendEvent(event string) error {
	return w.Send(&Message{
		Event: event,
	})
}

// SendData sends a structured message containing JSON-encoded data.
// The data is marshaled into JSON format and set as the `Data` field of the SSE message.
//
// Parameters:
//   - data: Any value that is JSON-marshalable.
//
// Note: For raw, unstructured data, use `Send()` with a manually constructed Message.
func (w *Writer) SendData(data any) error {
	marshal, err := json.Marshal(data)
	if err != nil {
		return err
	}
	return w.Send(&Message{
		Data: marshal,
	})
}

// Error retrieves any errors that occurred during the Writer's operation.
// In case of multiple errors, they are joined and returned as a single error value.
// Returns nil if no errors were recorded.
func (w *Writer) Error() error {
	return errors.Join(w.errors...)
}

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

// heartBeatPing is the keep-alive ping message sent to clients.
// Comments in SSE start with a colon `:`, meaning they are ignored by the client but maintain the connection.
var heartBeatPing = []byte(delimiter + whitespace + "ping" + string(byteLFLF)) // ": ping\n\n"

// WriterConfig contains the configuration options for creating a new SSE Writer.
// All fields have reasonable defaults, except for Context and ResponseWriter which are required.
type WriterConfig struct {
	Context        context.Context     // Required: Controls the lifecycle of the Writer
	ResponseWriter http.ResponseWriter // Required: HTTP response writer to send SSE data
	QueueSize      int                 // Optional: Size of the message queue buffer (default: 64)
	HeartBeat      time.Duration       // Optional: Interval for sending heartbeat pings (default: disabled if 0)
}

// validate checks the validity of the WriterConfig settings and configures defaults when needed.
//
// Validation rules:
//   - Context must not be nil
//   - ResponseWriter must not be nil and must implement http.Flusher
//   - QueueSize defaults to 64 if not provided or zero
//
// Returns:
//   - nil if the configuration is valid
//   - An error describing the validation failure
func (c *WriterConfig) validate() error {
	if c.Context == nil {
		return errors.New("missing context")
	}

	if c.ResponseWriter == nil {
		return errors.New("missing responseWriter")
	}

	if _, ok := c.ResponseWriter.(http.Flusher); !ok {
		return errors.New("responseWriter does not implement http.Flusher")
	}

	if c.QueueSize <= 0 {
		c.QueueSize = 64
	}

	return nil
}

// Writer encapsulates the logic for sending Server-Sent Events (SSE) to clients.
//
// Features:
//   - Asynchronous message processing
//   - Connection maintenance with heartbeats
//   - Graceful shutdown handling
//   - Error tracking and propagation
//
// A Writer is intended to be tied to a single client connection.
type Writer struct {
	config         *WriterConfig
	isClosed       atomic.Bool         // Tracks if the writer has been closed
	waitGroup      sync.WaitGroup      // Manages active goroutines for graceful shutdown
	ctx            context.Context     // Context to control the writer's lifecycle
	messageEncoder *Encoder            // Encodes messages in SSE-compliant format
	httpResponse   http.ResponseWriter // HTTP response writer for client communication
	httpFlusher    http.Flusher        // Handles flushing the response to the client
	closeSignal    chan struct{}       // Channel signaling graceful shutdown
	messageQueue   chan []byte         // Buffered message queue for asynchronous processing
	errors         []error             // Stores any errors encountered during processing
}

// NewWriter initializes and returns a new SSE Writer with the provided configuration.
// It validates the configuration, sets up the internal dependencies, and starts
// background processes for message handling.
//
// Parameters:
//   - config: Configuration settings for the Writer
//
// Returns:
//   - A pointer to the initialized Writer instance
//   - An error if the configuration is invalid
//
// Example usage:
//
//	config := &sse.WriterConfig{
//	    Context:        r.Context(),
//	    ResponseWriter: w,
//	    HeartBeat:      15 * time.Second,
//	}
//	writer, err := sse.NewWriter(config)
//	if err != nil {
//	    // handle error
//	}
//	defer writer.Close()
//
//	// Send messages using writer.Send(), writer.SendData(), etc.
func NewWriter(config *WriterConfig) (*Writer, error) {
	if err := config.validate(); err != nil {
		return nil, err
	}

	writer := &Writer{
		config:         config,
		ctx:            config.Context,
		messageEncoder: NewEncoder(),
		httpResponse:   config.ResponseWriter,
		httpFlusher:    config.ResponseWriter.(http.Flusher),
		closeSignal:    make(chan struct{}),
		messageQueue:   make(chan []byte, config.QueueSize),
		errors:         make([]error, 0, config.QueueSize),
	}

	writer.initialize()
	return writer, nil
}

// initialize sets up the SSE response by configuring the necessary HTTP headers and
// starts background goroutines to process messages and send heartbeats.
// This is called automatically by NewWriter.
func (w *Writer) initialize() {
	w.setSSEHeaders(w.httpResponse.Header())

	w.waitGroup.Add(3)
	go w.listenContext()
	go w.processMessageQueue()
	go w.startHeartbeatLoop()
}

// setSSEHeaders sets the necessary HTTP headers for a Server-Sent Events stream.
//
// According to the SSE specification, the following headers are set:
//   - Content-Type: text/event-stream; charset=utf-8 (required for SSE)
//   - Connection: keep-alive (maintains persistent connection)
//   - Cache-Control: no-cache (prevents caching of events, only if not already set)
//
// Parameters:
//   - header: The HTTP header collection to modify
func (w *Writer) setSSEHeaders(header http.Header) {
	header.Set("Content-Type", "text/event-stream; charset=utf-8")
	header.Set("Connection", "keep-alive")

	if len(header.Get("Cache-Control")) == 0 {
		header.Set("Cache-Control", "no-cache")
	}
}

// writeDataToClient sends raw data to the HTTP response writer and flushes it to the client.
//
// Parameters:
//   - data: The raw bytes to write
//
// Returns:
//   - An error if writing to the response fails
func (w *Writer) writeDataToClient(data []byte) error {
	if _, err := w.httpResponse.Write(data); err != nil {
		return err
	}

	w.httpFlusher.Flush()
	return nil
}

// recordError adds an error to the Writer's error list. Skips if the error is nil.
//
// Parameters:
//   - err: The error to record
func (w *Writer) recordError(err error) {
	if err != nil {
		w.errors = append(w.errors, err)
	}
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
		// Queue is full, drop the heartbeat
	}
}

// startHeartbeatLoop sends periodic heartbeat messages to the client to keep the connection alive.
// The heartbeat message is a comment line (": ping\n\n") as per the SSE protocol.
// If HeartBeat is set to 0 or negative in the config, this function returns immediately.
func (w *Writer) startHeartbeatLoop() {
	defer w.waitGroup.Done()

	if w.config.HeartBeat <= 0 {
		return // Heartbeat is disabled
	}

	ticker := time.NewTicker(w.config.HeartBeat)
	defer ticker.Stop()

	for {
		select {
		case <-w.closeSignal:
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

	for remainingMsg := range w.messageQueue {
		w.recordError(w.writeDataToClient(remainingMsg))
	}

	// Send final termination sequence
	w.recordError(w.writeDataToClient(byteLFLF))
}

// processMessageQueue handles the message queue and sends messages to the client asynchronously.
// If the writer is closed, it stops processing after draining the queue.
func (w *Writer) processMessageQueue() {
	defer w.waitGroup.Done()
	defer w.drainMessageQueue()

	for {
		select {
		case <-w.closeSignal:
			return
		case queuedMsg := <-w.messageQueue:
			w.recordError(w.writeDataToClient(queuedMsg))
		}
	}
}

// listenContext monitors the parent context and initiates a graceful shutdown
// when the context is canceled. This ensures proper resource cleanup when the
// parent context signals termination.
//
// This goroutine is responsible for:
//   - Watching for context cancellation signals
//   - Recording the context error when it occurs
//   - Triggering the Writer's close process
//
// This design separates context monitoring from message processing,
// ensuring reliable cleanup regardless of the message queue state.
func (w *Writer) listenContext() {
	defer w.waitGroup.Done()

	for {
		select {
		case <-w.closeSignal:
			return
		case <-w.ctx.Done():
			w.recordError(w.ctx.Err())
			_ = w.Close()
			return
		}
	}
}

// Close gracefully shuts down the Writer, ensuring all queued messages are processed
// and releasing resources. It blocks until all background goroutines complete.
//
// Behavior:
//   - Signals background goroutines to shut down
//   - Sets the isClosed flag to prevent new message processing
//   - Waits for all active processes to finish
//   - Closes internal channels and queues
//
// Calling Close multiple times is safe; subsequent calls are no-ops but will still
// return any errors that occurred during operation.
//
// Error handling:
//   - Collects and joins all errors encountered during the shutdown process
//   - Uses errors.Join() to combine multiple errors into a single return value
//   - Returns nil on successful closure with no errors
//   - All resources are released even if errors occur during the shutdown process
//
// Timeout handling:
//   - Close() will wait for all messages to be processed, which may block
//   - For timeout-based shutdown, cancel the context provided in WriterConfig first
//   - When the context is canceled, the Writer will stop processing new messages and begin shutdown
//
// Returns:
//   - nil if no errors occurred during operation
//   - A joined error containing all errors encountered during the Writer's lifetime
func (w *Writer) Close() error {
	if w.isClosed.Load() {
		return w.Error()
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
// Behavior:
//   - If the Writer is closed, this method will silently return without error
//   - If the message has no content (all fields empty), returns ErrMessageNoContent
//   - If the event name is invalid, returns ErrMessageInvalidEventName
//   - If the message queue is full, this method will block until space is available or the context is canceled
//   - There is no hard limit on message size, but very large messages may cause memory pressure
//
// Parameters:
//   - msg: The SSE Message to send, which may include fields like ID, Event, Data, and Retry
//
// Returns:
//   - nil if the message was successfully enqueued
//   - An error if message encoding fails or validation fails
func (w *Writer) Send(msg *Message) error {
	if w.isClosed.Load() {
		return nil
	}

	encodedMsg, err := w.messageEncoder.Encode(msg)
	if err != nil {
		return err
	}

	w.messageQueue <- encodedMsg
	return nil
}

// SendEvent sends an SSE message that includes only an Event field.
// This is useful for notifying clients of specific event types without data payloads.
//
// To ensure maximum client compatibility, this method includes a minimal data field
// containing a single newline character. This helps certain browser implementations
// that might not correctly process event-only messages.
//
// Parameters:
//   - event: Identifier for the event type, which clients can listen for using EventSource.addEventListener()
//
// Returns:
//   - nil if the event was successfully sent
//   - An error if the send operation fails
func (w *Writer) SendEvent(event string) error {
	eventMsg := &Message{
		Event: event,
		Data:  byteLF,
	}

	return w.Send(eventMsg)
}

// SendData sends a structured message containing JSON-encoded data.
// The data is marshaled into JSON format and set as the Data field of the SSE message.
//
// Parameters:
//   - data: Any value that is JSON-marshalable
//
// Returns:
//   - nil if the data was successfully sent
//   - An error if JSON marshaling fails or the send operation fails
//
// Note:
//   - If the Writer is closed, this method will silently return without error
//   - For raw, unstructured data, use Send() with a manually constructed Message
func (w *Writer) SendData(data any) error {
	if w.isClosed.Load() {
		return nil
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}

	dataMsg := &Message{
		Data: jsonData,
	}

	return w.Send(dataMsg)
}

// Error retrieves any errors that occurred during the Writer's operation.
// In case of multiple errors, they are joined and returned as a single error value.
//
// Returns:
//   - nil if no errors were recorded
//   - A joined error containing all errors encountered during operation
func (w *Writer) Error() error {
	return errors.Join(w.errors...)
}

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
	CloseTimeout   time.Duration       // Optional: Timeout for close operation (default: 30s)
	OnError        func(error)         // Optional: Callback invoked when an error occurs during operation
}

// validate checks the validity of the WriterConfig settings and configures defaults when needed.
//
// Validation rules:
//   - Context must not be nil
//   - ResponseWriter must not be nil and must implement http.Flusher
//   - QueueSize defaults to 64 if not provided or zero
//   - CloseTimeout defaults to 30 seconds if not provided
//
// Returns:
//   - nil if the configuration is valid
//   - An error describing the validation failure
func (c *WriterConfig) validate() error {
	// Default configuration values.
	const (
		defaultQueueSize    = 64
		defaultCloseTimeout = 30 * time.Second
	)

	if c == nil {
		return errors.New("sse: nil config")
	}

	if c.Context == nil {
		return errors.New("sse: missing context in config")
	}

	if c.ResponseWriter == nil {
		return errors.New("sse: missing responseWriter in config")
	}

	if _, ok := c.ResponseWriter.(http.Flusher); !ok {
		return errors.New("sse: responseWriter does not implement http.Flusher")
	}

	if c.QueueSize <= 0 {
		c.QueueSize = defaultQueueSize
	}

	if c.CloseTimeout <= 0 {
		c.CloseTimeout = defaultCloseTimeout
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
//   - Thread-safe operations
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
	errorsMu       sync.Mutex          // Protects concurrent access to errors slice
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
//	    OnError: func(err error) {
//	        log.Printf("SSE error: %v", err)
//	    },
//	}
//	writer, err := sse.NewWriter(config)
//	if err != nil {
//	    http.Error(w, err.Error(), http.StatusInternalServerError)
//	    return
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

	// Immediately send headers by writing status and flushing
	// This ensures headers are sent before any message processing begins
	w.httpResponse.WriteHeader(http.StatusOK)
	w.httpFlusher.Flush()

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
//   - X-Accel-Buffering: no (disables buffering in nginx)
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

// recordError adds an error to the Writer's error list in a thread-safe manner.
// If an OnError callback is configured, it will be invoked immediately.
// Skips if the error is nil.
//
// Parameters:
//   - err: The error to record
func (w *Writer) recordError(err error) {
	if err == nil {
		return
	}

	w.errorsMu.Lock()
	w.errors = append(w.errors, err)
	w.errorsMu.Unlock()

	// Invoke error callback if configured
	if w.config.OnError != nil {
		// Run callback in a separate goroutine to avoid blocking
		go w.config.OnError(err)
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
		// This is acceptable as heartbeats are only for connection maintenance
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

	select {
	case <-w.closeSignal:
		return
	case <-w.ctx.Done():
		w.recordError(w.ctx.Err())
		// Close is safe to call here because:
		// 1. Close() checks isClosed before doing anything
		// 2. This goroutine will exit when it sees closeSignal
		// 3. waitGroup.Wait() will only proceed after all goroutines exit
		_ = w.Close()
		return
	}
}

// IsClosed returns whether the Writer has been closed.
// This is useful for checking the Writer's state without attempting operations.
//
// Returns:
//   - true if the Writer is closed
//   - false if the Writer is still active
func (w *Writer) IsClosed() bool {
	return w.isClosed.Load()
}

// Close gracefully shuts down the Writer, ensuring all queued messages are processed
// and releasing resources. It blocks until all background goroutines complete or until
// the configured CloseTimeout is reached.
//
// Behavior:
//   - Signals background goroutines to shut down
//   - Sets the isClosed flag to prevent new message processing
//   - Waits for all active processes to finish (with timeout)
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
//   - If the close operation exceeds CloseTimeout, returns ErrWriterCloseTimeout
//   - Even on timeout, isClosed flag is set to prevent further operations
//   - For immediate shutdown without waiting, cancel the context provided in WriterConfig
//
// Returns:
//   - nil if no errors occurred during operation
//   - A joined error containing all errors encountered during the Writer's lifetime
//   - ErrWriterCloseTimeout if the close operation times out
func (w *Writer) Close() error {
	if w.isClosed.Load() {
		return w.Error()
	}

	w.isClosed.Store(true)
	close(w.closeSignal)

	// Wait for all goroutines to complete with timeout
	done := make(chan struct{})
	go func() {
		w.waitGroup.Wait()
		close(done)
	}()

	select {
	case <-done:
		return w.Error()
	case <-time.After(w.config.CloseTimeout):
		// Record timeout error
		w.recordError(errors.New("close timeout"))
		return w.Error()
	}
}

// Send enqueues a message to be delivered to the client.
//
// The method encodes the message in SSE protocol format and adds it to the queue
// to be processed by the background goroutines asynchronously.
//
// Behavior:
//   - If the Writer is closed, returns ErrWriterClosed
//   - If the message has no content (all fields empty), returns ErrMessageNoContent
//   - If the event name is invalid, returns ErrMessageInvalidEventName
//   - The method blocks if the message queue is full, until:
//     a) Space becomes available in the queue
//     b) The Writer is closed (returns ErrWriterClosed)
//     c) The context is canceled (returns context error)
//   - There is no hard limit on message size, but very large messages may cause memory pressure
//
// Thread safety:
//   - This method is safe to call from multiple goroutines
//   - The select statement prevents panics from sending to a closed channel
//
// Parameters:
//   - msg: The SSE Message to send, which may include fields like ID, Event, Data, and Retry
//
// Returns:
//   - nil if the message was successfully enqueued
//   - ErrWriterClosed if the Writer is closed
//   - An error from Encode() if message encoding fails
//   - Context error if the context is canceled while waiting to enqueue
//
// Example:
//
//	err := writer.Send(&sse.Message{
//	    ID:    "123",
//	    Event: "update",
//	    Data:  []byte(`{"status":"ok"}`),
//	    Retry: 3000,
//	})
//	if err != nil {
//	    log.Printf("Failed to send message: %v", err)
//	}
func (w *Writer) Send(msg *Message) error {
	if w.isClosed.Load() {
		return errors.New("sse: already closed")
	}

	encodedMsg, err := w.messageEncoder.Encode(msg)
	if err != nil {
		return err
	}

	// Use select to prevent panic when sending to closed channel
	// and to respect context cancellation
	select {
	case w.messageQueue <- encodedMsg:
		return nil
	case <-w.closeSignal:
		return errors.New("sse: already closed")
	case <-w.ctx.Done():
		return w.ctx.Err()
	}
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
//   - ErrWriterClosed if the Writer is closed
//   - An error if the send operation fails
//
// Example:
//
//	err := writer.SendEvent("heartbeat")
//	if err != nil {
//	    log.Printf("Failed to send event: %v", err)
//	}
func (w *Writer) SendEvent(event string) error {
	eventMsg := &Message{
		Event: event,
		Data:  byteLF, // Minimal data for compatibility
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
//   - ErrWriterClosed if the Writer is closed
//   - An error if JSON marshaling fails or the send operation fails
//
// Note:
//   - For raw, unstructured data, use Send() with a manually constructed Message
//
// Example:
//
//	type Update struct {
//	    Status  string `json:"status"`
//	    Message string `json:"message"`
//	}
//
//	err := writer.SendData(Update{
//	    Status:  "success",
//	    Message: "Operation completed",
//	})
//	if err != nil {
//	    log.Printf("Failed to send data: %v", err)
//	}
func (w *Writer) SendData(data any) error {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}

	dataMsg := &Message{
		Data: jsonData,
	}

	return w.Send(dataMsg)
}

// Error retrieves any errors that occurred during the Writer's operation in a thread-safe manner.
// In case of multiple errors, they are joined and returned as a single error value.
//
// This method can be called multiple times and will always return the accumulated errors.
// It's particularly useful for checking errors after Close() or during operation.
//
// Returns:
//   - nil if no errors were recorded
//   - A joined error containing all errors encountered during operation
//
// Example:
//
//	writer.Close()
//	if err := writer.Error(); err != nil {
//	    log.Printf("Writer encountered errors: %v", err)
//	}
func (w *Writer) Error() error {
	w.errorsMu.Lock()
	defer w.errorsMu.Unlock()

	if len(w.errors) == 0 {
		return nil
	}

	return errors.Join(w.errors...)
}

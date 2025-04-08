package sse

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	w.setSSEHeaders(w.httpResponse.Header())
	w.waitGroup.Add(3)
	go w.listenContext()
	go w.processMessageQueue()
	go w.startHeartbeatLoop()
}

// setSSEHeaders sets the necessary HTTP headers for a Server-Sent Events stream.
// According to the SSE specification, the following headers should be set:
// - Content-Type: text/event-stream; charset=utf-8 (required for SSE)
// - Connection: keep-alive (maintains persistent connection)
// - Cache-Control: no-cache (prevents caching of events)
//
// The function preserves any existing Cache-Control header if already set.
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
// If the writer is closed, it stops processing after draining the queue.
func (w *Writer) processMessageQueue() {
	defer w.waitGroup.Done()
	defer w.drainMessageQueue()

	for {
		select {
		case <-w.closeSignal:
			return
		case msg := <-w.messageQueue:
			w.recordError(w.writeDataToClient(msg))
		}
	}
}

// listenContext monitors the parent context and initiates a graceful shutdown
// when the context is canceled. This ensures proper resource cleanup when the
// parent context signals termination.
//
// This goroutine is responsible for:
// - Watching for context cancellation signals
// - Recording the context error when it occurs
// - Triggering the Writer's close process
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
// This method performs the following:
// - Signals background goroutines to shut down.
// - Sets the `isClosed` flag to prevent new message processing.
// - Waits for all active processes to finish.
// - Closes internal channels and queues.
// - Returns any errors encountered during operation.
//
// Calling Close multiple times is safe; subsequent calls are no-ops but will still
// return any errors that occurred during operation.
//
// Error handling strategy:
// - Collects and joins all errors encountered during the shutdown process
// - Uses errors.Join() to combine multiple errors into a single return value
// - Returns nil on successful closure with no errors
// - All resources are released even if errors occur during the shutdown process
//
// Timeout handling:
// - Close() will wait for all messages to be processed, which may block
// - For timeout-based shutdown, cancel the context provided in WriterConfig first
// - When the context is canceled, the Writer will stop processing new messages and begin shutdown
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
// Parameters:
//   - msg: The SSE Message to send, which may include fields like ID, Event, Data, and Retry.
//
// Boundary conditions:
// - If the Writer is closed (Close() has been called), this method will silently return without error
// - If the message has no content (all fields empty), returns ErrMessageNoContent
// - If the event name is invalid, returns ErrMessageInvalidEventName
// - If the message queue is full, this method will block until space is available or the context is canceled
// - There is no hard limit on message size, but very large messages may cause memory pressure
func (w *Writer) Send(msg *Message) error {
	if w.isClosed.Load() {
		return errors.New("writer is closed")
	}

	buffer := GetBuffer()
	defer ReleaseBuffer(buffer)

	if !w.messageEncoder.isValidMessage(msg) {
		return ErrMessageNoContent
	}

	if !isValidSSEEventName(msg.Event) {
		return errors.Join(ErrMessageInvalidEventName, fmt.Errorf("event name: %s", msg.Event))
	}

	w.messageEncoder.writeID(msg.ID, buffer)
	w.messageEncoder.writeEvent(msg.Event, buffer)
	w.messageEncoder.writeData(msg.Data, buffer)
	w.messageEncoder.writeRetry(msg.Retry, buffer)
	buffer.Write(byteLFLF)

	select {
	case w.messageQueue <- buffer.Bytes():
		return nil
	case <-w.closeSignal:
		return errors.New("writer is closed")
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
//   - event: Identifier for the event type, which clients can listen for using EventSource.addEventListener().
func (w *Writer) SendEvent(event string) error {
	return w.Send(&Message{
		Event: event,
		Data:  byteLF,
	})
}

// SendData sends a structured message containing JSON-encoded data.
// The data is marshaled into JSON format and set as the `Data` field of the SSE message.
//
// Parameters:
//   - data: Any value that is JSON-marshalable.
//
// Note: For raw, unstructured data, use `Send()` with a manually constructed Message.
func (w *Writer) SendData(data interface{}) error {
	msg := GetMessage()
	defer ReleaseMessage(msg)

	var err error
	msg.Data, err = json.Marshal(data)
	if err != nil {
		return err
	}

	return w.Send(msg)
}

// Error retrieves any errors that occurred during the Writer's operation.
// In case of multiple errors, they are joined and returned as a single error value.
// Returns nil if no errors were recorded.
func (w *Writer) Error() error {
	return errors.Join(w.errors...)
}

// WithSSE establishes a Server-Sent Events stream that sends messages
// from the provided channel to the client over HTTP.
//
// This function:
// 1. Sets appropriate SSE headers
// 2. Verifies that the response writer supports flushing (required for streaming)
// 3. Creates a message encoder for converting Message objects to SSE wire format
// 4. Continuously processes messages from the channel until:
//   - The context is canceled
//   - The message channel is closed
//   - A write error occurs
//
// Error handling strategy:
// - Returns an error immediately if the ResponseWriter doesn't support http.Flusher
// - If message encoding fails, closes the Writer and returns a joined error
// - If writing fails, closes the Writer and returns a joined error
// - If the context is canceled, closes the Writer normally and returns the context error
// - If the message channel is closed, closes the Writer normally and returns nil
//
// Important notes:
// - This function blocks until an error occurs or the channel is closed
// - No heartbeat mechanism is provided; use NewWriter with HeartBeat for heartbeats
// - If the client disconnects, the underlying http.ResponseWriter write will fail and cause the function to return
// - All resources are properly released when the function returns
//
// Usage example:
//
//	messageChan := make(chan *sse.Message)
//	go func() {
//	  // Send messages to the channel
//	  messageChan <- &sse.Message{Data: []byte("update")}
//	  // Close the channel when done
//	  close(messageChan)
//	}()
//
//	http.HandleFunc("/events", func(w http.ResponseWriter, r *http.Request) {
//	  err := sse.WithSSE(r.Context(), w, messageChan)
//	  if err != nil {
//	    // Handle error
//	  }
//	})
func WithSSE(ctx context.Context, response http.ResponseWriter, messageChan chan *Message) error {
	writer, err := NewWriter(&WriterConfig{
		Context:        ctx,
		ResponseWriter: response,
		QueueSize:      len(messageChan),
	})
	if err != nil {
		return err
	}

	for {
		select {
		case <-ctx.Done():
			return writer.Close()

		case message, ok := <-messageChan:
			if !ok {
				return writer.Close()
			}
			err = writer.Send(message)
			if err != nil {
				return errors.Join(err, writer.Close())
			}
		}
	}
}

package sse

import (
	"net/http"
)

// Reader provides a high-level interface for consuming Server-Sent Events from an HTTP httpResponse.
// It wraps the lower-level Decoder to provide a more convenient API for clients.
// The Reader handles:
// - Processing SSE messages from an HTTP httpResponse
// - Tracking the current message and any errors
// - Maintaining the last event ID for reconnection support
// - Proper resource cleanup
type Reader struct {
	lastError      error          // Stores lastError that occurred during processing
	currentMessage Message        // Holds the most recently read message
	httpResponse   *http.Response // The HTTP httpResponse containing the SSE stream
	messageDecoder *Decoder       // Low-level messageDecoder that parses the SSE format
}

// NewReader creates a new SSE reader for the given HTTP httpResponse.
// The provided httpResponse should be from an SSE endpoint with Content-Type: text/event-stream.
// The Reader takes ownership of the httpResponse body and will close it when Close() is called.
//
// Example usage:
//
//	resp, err := http.Get("https://example.com/events")
//	if err != nil {
//	    // handle lastError
//	}
//	reader := sse.NewReader(resp)
//	defer reader.Close()
//
//	for reader.Next() {
//	    msg, _ := reader.Current()
//	    // process message
//	}
//	if err := reader.Error(); err != nil {
//	    // handle lastError
//	}
func NewReader(response *http.Response) *Reader {
	return &Reader{
		httpResponse:   response,
		messageDecoder: NewDecoder(response.Body),
	}
}

// Current returns the most recently read SSE message and any lastError that occurred.
// This method should be called after Next() returns true to access the completed message.
// The lastError will be nil unless an lastError occurred during processing.
//
// Returns:
// - The current message with all its fields (ID, Event, Data, Retry)
// - Any lastError that occurred during message processing
func (r *Reader) Current() (Message, error) {
	return r.currentMessage, r.lastError
}

// Next advances to the next SSE message in the stream.
// It returns true if a complete message was successfully read and is available via Current().
// It returns false when either:
// - The end of the stream is reached (check Error() for nil)
// - An lastError occurred during processing (check Error() for details)
//
// This method handles the underlying message parsing according to the SSE specification.
// Each call to Next() will block until either a complete message is read or an lastError occurs.
//
// According to the SSE specification, messages are separated by blank lines (two consecutive newlines).
func (r *Reader) Next() bool {
	// Check if there was a previous lastError from the Decoder
	r.lastError = r.messageDecoder.Error()
	if r.lastError != nil {
		return false
	}

	// Try to read the next message
	if !r.messageDecoder.Next() {
		return false
	}
	r.currentMessage = r.messageDecoder.Current()

	return true
}

// LastID returns the ID of the most recently received message.
// According to the SSE specification, the ID can be used when reconnecting to an SSE stream
// by sending it in the Last-Event-ID header to resume from where the client left off.
//
// If no message has been received yet or if the last message didn't have an ID field,
// this method returns an empty string.
func (r *Reader) LastID() string {
	return r.currentMessage.ID
}

// Close releases resources associated with the Reader.
// This closes the underlying HTTP httpResponse body and should be called when
// the Reader is no longer needed to prevent resource leaks.
//
// It's recommended to defer this call after creating a new Reader:
//
//	reader := sse.NewReader(resp)
//	defer reader.Close()
func (r *Reader) Close() error {
	return r.httpResponse.Body.Close()
}

// Error returns any error that occurred during SSE stream processing.
// This should be checked after Next() returns false to determine if the stream
// ended normally or due to an error condition.
//
// Error handling strategy:
// 1. Normal stream end: If the server gracefully closed the connection, Error() returns nil
// 2. Parsing errors: If invalid format is encountered, returns specific format errors
//   - Invalid event name: Returns ErrMessageInvalidEventName
//   - Invalid UTF-8: Automatically replaced with U+FFFD, no error returned
//
// 3. I/O errors: Underlying read errors are propagated (e.g., connection reset, timeout)
// 4. Context cancellation: If the HTTP request context is canceled, returns context.Canceled
//
// Error handling best practices:
// - Always check Error() after Next() returns false
// - Distinguish between normal termination (Error() == nil) and abnormal termination
// - For network errors, consider implementing retry logic
// - Parsing errors typically indicate the server sent invalid data and should be logged and investigated
func (r *Reader) Error() error {
	return r.lastError
}

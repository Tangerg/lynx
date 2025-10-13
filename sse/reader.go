package sse

import (
	"fmt"
	"iter"
	"net/http"
	"strings"
)

// Reader provides a high-level interface for consuming Server-Sent Events from an HTTP response.
// It wraps the lower-level Decoder to offer a more convenient API for clients.
//
// Reader handles:
// - Parsing SSE messages from HTTP responses
// - Tracking current message and error state
// - Maintaining last event ID for reconnection support
// - Proper resource cleanup
type Reader struct {
	currentMessage Message        // Most recently read message
	httpResponse   *http.Response // HTTP response containing the SSE stream
	messageDecoder *Decoder       // Low-level decoder for parsing SSE format
}

// NewReader creates a new SSE reader for the given HTTP response.
// The response should be from an SSE endpoint with Content-Type: text/event-stream.
// The Reader takes ownership of the response body and will close it when Close() is called.
//
// Example:
//
//	resp, err := http.Get("https://example.com/events")
//	if err != nil {
//	    // handle error
//	}
//	reader := sse.NewReader(resp)
//	defer reader.Close()
//
//	for reader.Next() {
//	    msg, _ := reader.Current()
//	    // process message
//	}
//	if err := reader.Error(); err != nil {
//	    // handle error
//	}
func NewReader(httpResponse *http.Response) (*Reader, error) {
	contentType := httpResponse.Header.Get("Content-Type")
	if !strings.HasPrefix(contentType, "text/event-stream") {
		return nil, fmt.Errorf("expected Content-Type 'text/event-stream', got %s", contentType)
	}
	return &Reader{
		httpResponse:   httpResponse,
		messageDecoder: NewDecoder(httpResponse.Body),
	}, nil
}

// Iter creates an iterator-based SSE reader for simplified stream consumption.
// It returns an iter.Seq2 that yields Message and error pairs, providing a convenient
// way to consume SSE streams using Go 1.23+ range-over-function syntax.
//
// This is a convenience wrapper around Reader that simplifies common use cases.
// For more control over the reading process (e.g., checking LastID() during iteration,
// conditional processing, custom error handling), use NewReader directly.
//
// The iterator automatically closes the HTTP response body when iteration completes,
// either normally or through early termination (break/return).
//
// Trade-offs:
//   - Simpler API: No need to manually call Next() or Close()
//   - Automatic cleanup: Response body is closed when iteration ends
//   - Less control: Cannot access Reader methods like LastID() during iteration
//
// Example - Simple iteration:
//
//	resp, err := http.Get("https://example.com/events")
//	if err != nil {
//	    // handle error
//	}
//	// No need for defer resp.Body.Close() - iterator handles cleanup
//
//	for msg, err := range sse.NewIterReader(resp) {
//	    if err != nil {
//	        log.Printf("Error: %v", err)
//	        break  // Body is automatically closed
//	    }
//	    fmt.Printf("Event: %s, Data: %s\n", msg.Event, msg.Data)
//	}
//
// Example - When you need more control, use Reader instead:
//
//	reader := sse.NewReader(resp)
//	defer reader.Close()
//
//	for reader.Next() {
//	    msg, err := reader.Current()
//	    if err != nil {
//	        // handle error
//	    }
//	    // Can access reader.LastID() here for reconnection logic
//	    lastID := reader.LastID()
//	}
//
// Parameters:
//   - httpResponse: HTTP response from an SSE endpoint (Content-Type: text/event-stream)
//
// Returns:
//   - An iterator that yields (Message, error) pairs for each SSE message
func Iter(httpResponse *http.Response) iter.Seq2[Message, error] {
	reader, err := NewReader(httpResponse)
	return func(yield func(Message, error) bool) {
		defer reader.Close()
		if err != nil {
			yield(Message{}, err)
			return
		}
		for reader.Next() {
			if reader.Error() != nil {
				yield(Message{}, reader.Error())
				return
			}
			if !yield(reader.Current()) {
				return
			}
		}
	}
}

// Current returns the most recently read SSE message and any associated error.
// This method should be called after Next() returns true to access the message.
// The error will be nil unless an error occurred during processing.
//
// Returns:
// - The current message with all fields (ID, Event, Data, Retry)
// - Any error that occurred during message processing
func (r *Reader) Current() (Message, error) {
	return r.currentMessage, r.Error()
}

// Next advances to the next SSE message in the stream.
// Returns true if a complete message was successfully read and is available via Current().
// Returns false when:
// - End of stream is reached (check Error() for nil)
// - An error occurred during processing (check Error() for details)
//
// This method blocks until either a complete message is read or an error occurs.
// Messages are separated by blank lines (two consecutive newlines) per SSE specification.
func (r *Reader) Next() bool {
	if r.messageDecoder.Next() {
		r.currentMessage = r.messageDecoder.Current()
		return true
	}
	return false
}

// LastID returns the ID of the most recently received message.
// Per SSE specification, this ID can be used when reconnecting to resume
// from where the client left off by sending it in the Last-Event-ID header.
//
// Returns empty string if no message has been received or if the last
// message didn't have an ID field.
func (r *Reader) LastID() string {
	return r.currentMessage.ID
}

// Close releases resources associated with the Reader.
// This closes the underlying HTTP response body and should be called
// when the Reader is no longer needed to prevent resource leaks.
//
// Recommended usage:
//
//	reader := sse.NewReader(resp)
//	defer reader.Close()
func (r *Reader) Close() error {
	return r.httpResponse.Body.Close()
}

// Error returns any error that occurred during SSE stream processing.
// Should be checked after Next() returns false to determine if the stream
// ended normally or due to an error.
//
// Error types:
//   - Normal stream end: Returns nil when server gracefully closes connection
//   - Parsing errors: Returns specific format errors for invalid SSE data
//     (Invalid UTF-8 is automatically replaced with U+FFFD, no error returned)
//   - I/O errors: Propagates underlying read errors (connection reset, timeout, etc.)
//   - Context cancellation: Returns context.Canceled if HTTP request context is canceled
//
// Best practices:
// - Always check Error() after Next() returns false
// - Distinguish between normal (Error() == nil) and abnormal termination
// - Consider retry logic for network errors
// - Log and investigate parsing errors as they indicate invalid server data
func (r *Reader) Error() error {
	return r.messageDecoder.Error()
}

package sse

import (
	"context"
	"errors"
	"net/http"
)

// SetSSEHeaders sets the necessary HTTP headers for a Server-Sent Events stream.
// According to the SSE specification, the following headers should be set:
// - Content-Type: text/event-stream; charset=utf-8 (required for SSE)
// - Connection: keep-alive (maintains persistent connection)
// - Cache-Control: no-cache (prevents caching of events)
//
// The function preserves any existing Cache-Control header if already set.
//
// Parameters:
//   - header: The HTTP header collection to modify
func SetSSEHeaders(header http.Header) {
	header.Set("Content-Type", "text/event-stream; charset=utf-8")
	header.Set("Connection", "keep-alive")
	if len(header.Get("Cache-Control")) == 0 {
		header.Set("Cache-Control", "no-cache")
	}
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
// Each message is encoded to SSE format, written to the response, and immediately
// flushed to ensure real-time delivery to the client.
//
// Parameters:
//   - ctx: Context for cancellation control
//   - response: HTTP ResponseWriter to write the SSE stream to
//   - messageChan: Channel providing messages to be sent to the client
//
// Returns:
//   - Error if the response doesn't support flushing
//   - Error if message encoding fails
//   - Error if writing to the response fails
//   - Context error if the context is canceled
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
	flusher, ok := response.(http.Flusher)
	if !ok {
		return errors.New("httpResponse is not a http.Flusher")
	}

	SetSSEHeaders(response.Header())

	encoder := newMessageEncoder()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case message, ok1 := <-messageChan:
			if !ok1 {
				_, err := response.Write(byteLFLF)
				if err != nil {
					return err
				}
				flusher.Flush()
				return ctx.Err()
			}

			encode, err := encoder.Encode(message)
			if err != nil {
				return err
			}

			_, err = response.Write(encode)
			if err != nil {
				return err
			}

			flusher.Flush()
		}
	}
}

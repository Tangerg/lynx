// Package sse implements the Server-Sent Events (SSE) protocol according to the W3C specification.
// See: https://www.w3.org/TR/2009/WD-eventsource-20091029/
//
// SSE is a one-way communication protocol that allows servers to push real-time updates
// to clients over a single HTTP connection. This package provides both server-side and
// client-side implementations with three layers of abstraction for different use cases.
//
// # Features
//
//   - Complete SSE protocol implementation (encoding and decoding)
//   - Server-side: Asynchronous message writing with HTTP streaming
//   - Client-side: Iterator-based message reading from HTTP responses
//   - Support for all SSE fields: id, event, data, and retry
//   - Multiline data processing according to specification
//   - Message validation and sanitization
//   - Graceful shutdown and error handling
//   - Connection keep-alive with heartbeat support
//   - Reconnection support via Last-Event-ID header
//
// # Architecture
//
// The package is organized into three logical layers:
//
//   - Low-level: Encoder and Decoder handle the SSE wire format
//   - Mid-level: Writer and Reader provide HTTP-aware streaming abstractions
//   - High-level: Iter functions offer modern iterator-based APIs (Go 1.23+)
//
// This layered design allows you to choose the appropriate level of abstraction
// for your use case, from fine-grained control to convenient high-level APIs.
//
// # Message Structure
//
// An SSE message consists of four optional fields:
//
//	type Message struct {
//	    ID    string // Unique identifier for reconnection support
//	    Event string // Event type (defaults to "message")
//	    Data  []byte // Event payload (can be multiline)
//	    Retry int    // Reconnection time in milliseconds
//	}
//
// The wire format follows the SSE specification:
//
//	id: message-id
//	event: event-type
//	data: payload line 1
//	data: payload line 2
//	retry: 3000
//	<blank line>
//
// # Server-Side Usage
//
// Use Writer for sending SSE events to HTTP clients. Writer handles HTTP headers,
// asynchronous message queuing, and graceful shutdown:
//
//	func handleSSE(w http.ResponseWriter, r *http.Request) {
//	    writer, err := sse.NewWriter(&sse.WriterConfig{
//	        Context:        r.Context(),
//	        ResponseWriter: w,
//	        HeartBeat:      15 * time.Second,  // Keep connection alive
//	        QueueSize:      64,                // Message buffer size
//	        CloseTimeout:   30 * time.Second,  // Graceful shutdown timeout
//	        OnError: func(err error) {
//	            log.Printf("SSE error: %v", err)
//	        },
//	    })
//	    if err != nil {
//	        http.Error(w, err.Error(), http.StatusInternalServerError)
//	        return
//	    }
//	    defer writer.Close()
//
//	    // Send structured JSON data
//	    writer.SendData(map[string]interface{}{
//	        "status": "connected",
//	        "time":   time.Now(),
//	    })
//
//	    // Send custom events
//	    writer.Send(&sse.Message{
//	        ID:    "msg-123",
//	        Event: "user.created",
//	        Data:  []byte(`{"name":"Alice"}`),
//	        Retry: 3000,
//	    })
//
//	    // Send event-only messages
//	    writer.SendEvent("heartbeat")
//
//	    // Stream from a channel
//	    for event := range eventChannel {
//	        if err := writer.SendData(event); err != nil {
//	            log.Printf("Send failed: %v", err)
//	            return
//	        }
//	    }
//	}
//
// Writer features:
//
//   - Asynchronous message processing via internal queue
//   - Automatic HTTP header configuration (Content-Type, Cache-Control, etc.)
//   - Optional heartbeat to detect disconnected clients
//   - Graceful shutdown with message queue draining
//   - Context-aware lifecycle management
//   - Thread-safe: all Send methods can be called concurrently
//
// For low-level encoding without HTTP, use Encoder directly:
//
//	encoder := sse.NewEncoder()
//	encoded, err := encoder.Encode(&sse.Message{
//	    Event: "notification",
//	    Data:  []byte("hello world"),
//	})
//	if err != nil {
//	    log.Fatal(err)
//	}
//	// encoded: "event: notification\ndata: hello world\n\n"
//
// # Client-Side Usage
//
// Use Reader for traditional iterator-style message consumption:
//
//	resp, err := http.Get("https://api.example.com/events")
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	reader, err := sse.NewReader(resp)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer reader.Close()
//
//	for reader.Next() {
//	    msg := reader.Current()
//	    fmt.Printf("Event: %s, Data: %s\n", msg.Event, msg.Data)
//
//	    // Access last message ID for reconnection
//	    lastID := reader.LastID()
//	}
//
//	if err := reader.Error(); err != nil {
//	    log.Printf("Stream error: %v", err)
//	}
//
// For Go 1.23+, use the modern range-over-function iterator:
//
//	resp, err := http.Get("https://api.example.com/events")
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// Automatic resource cleanup - no need for defer
//	for msg, err := range sse.Iter(resp) {
//	    if err != nil {
//	        log.Printf("Error: %v", err)
//	        break
//	    }
//	    fmt.Printf("Event: %s, Data: %s\n", msg.Event, msg.Data)
//	}
//
// Reader features:
//
//   - Validates Content-Type header (must be text/event-stream)
//   - Handles UTF-8 BOM (U+FEFF) at stream start
//   - Supports all line ending formats (LF, CR, CRLF)
//   - Tracks last message ID for reconnection
//   - Automatic resource cleanup with Close()
//
// For low-level decoding from any io.Reader, use Decoder directly:
//
//	decoder := sse.NewDecoder(reader)
//	for decoder.Next() {
//	    msg := decoder.Current()
//	    processMessage(msg)
//	}
//	if err := decoder.Error(); err != nil {
//	    log.Printf("Decode error: %v", err)
//	}
//
// # Reconnection Support
//
// SSE supports automatic reconnection using the Last-Event-ID header.
// Clients should track the last received message ID and include it when reconnecting:
//
//	func subscribeWithReconnect(url string) {
//	    var lastID string
//	    backoff := time.Second
//
//	    for {
//	        req, _ := http.NewRequest("GET", url, nil)
//	        if lastID != "" {
//	            // Resume from last received message
//	            req.Header.Set("Last-Event-ID", lastID)
//	        }
//
//	        resp, err := http.DefaultClient.Do(req)
//	        if err != nil {
//	            log.Printf("Connection failed: %v", err)
//	            time.Sleep(backoff)
//	            backoff = min(backoff*2, 30*time.Second)
//	            continue
//	        }
//
//	        reader, err := sse.NewReader(resp)
//	        if err != nil {
//	            resp.Body.Close()
//	            time.Sleep(backoff)
//	            continue
//	        }
//
//	        backoff = time.Second // Reset on successful connection
//
//	        for reader.Next() {
//	            msg := reader.Current()
//	            processEvent(msg)
//	            lastID = reader.LastID()
//	        }
//
//	        if err := reader.Error(); err != nil {
//	            log.Printf("Stream error: %v", err)
//	        }
//
//	        reader.Close()
//
//	        // Use retry interval from server, or default
//	        retryAfter := time.Duration(reader.Current().Retry) * time.Millisecond
//	        if retryAfter == 0 {
//	            retryAfter = 5 * time.Second
//	        }
//	        time.Sleep(retryAfter)
//	    }
//	}
//
// # Protocol Details
//
// SSE messages are transmitted as UTF-8 encoded text. Each message consists of
// one or more fields, terminated by a blank line:
//
//	Field format:
//	  field-name: field-value\n
//
//	Comment format (ignored by clients):
//	  : comment text\n
//
// Field specifications:
//
//   - id: Sets the event ID. Persists across messages until explicitly changed.
//   - event: Sets the event type. Clients can filter events by type. Defaults to "message".
//   - data: Adds a line to the message data. Multiple data fields are joined with newlines.
//   - retry: Sets the reconnection time in milliseconds. Only positive integers are valid.
//
// Event names must follow DOM naming rules:
//
//   - Cannot be empty
//   - Cannot start with digits or special characters (., -, _)
//   - Can only contain letters, digits, dots, hyphens, and underscores
//
// Examples of valid event names:
//
//	"message", "user.created", "notification-received", "event_1"
//
// Examples of invalid event names:
//
//	"", ".invalid", "-invalid", "_invalid", "123invalid"
//
// # Concurrency
//
// Thread-safe components:
//
//   - Writer: All Send methods are safe for concurrent use
//   - Encoder: Stateless, safe for concurrent use from multiple goroutines
//
// NOT thread-safe:
//
//   - Reader: Use one instance per goroutine
//   - Decoder: Use one instance per goroutine
//   - Message: Plain data structure with no internal synchronization
//
// Example of concurrent writing:
//
//	go writer.SendData(data1)  // Safe
//	go writer.SendData(data2)  // Safe
//	go writer.SendEvent("ping") // Safe
//
// # Error Handling
//
// The package defines sentinel errors for common validation failures:
//
//	var (
//	    ErrMessageNoContent         error // Message has no data or event
//	    ErrMessageInvalidEventName  error // Event name violates naming rules
//	)
//
// Server-side error handling:
//
//   - Writer.Send methods return validation errors immediately
//   - Writer.Error() returns accumulated errors from async operations
//   - Writer.Close() returns final error state
//   - OnError callback receives errors as they occur
//
// Client-side error handling:
//
//   - Reader.Error() and Decoder.Error() return stream errors
//   - Distinguishes normal EOF (Error() == nil) from error conditions
//   - Invalid UTF-8 is replaced with U+FFFD, no error returned
//
// # Resource Management
//
// Server-side:
//
//	writer, err := sse.NewWriter(config)
//	if err != nil {
//	    return err
//	}
//	defer writer.Close() // Always close to ensure cleanup
//
//	// Writer automatically closes when context is canceled
//	// Close() waits for pending messages with timeout
//
// Client-side with Reader:
//
//	reader, err := sse.NewReader(resp)
//	if err != nil {
//	    return err
//	}
//	defer reader.Close() // Closes HTTP response body
//
// Client-side with Iter:
//
//	for msg, err := range sse.Iter(resp) {
//	    // Automatic cleanup - no defer needed
//	}
//
// # Performance Considerations
//
// Server-side optimizations:
//
//   - Configure QueueSize based on expected message rate
//   - Enable HeartBeat to detect dead connections early
//   - Use SendData() for automatic JSON marshaling
//   - Monitor queue pressure via OnError callback
//   - Consider message batching for high-throughput scenarios
//
// Client-side optimizations:
//
//   - Use HTTP client with connection pooling
//   - Set appropriate timeouts on HTTP requests
//   - Process messages asynchronously to avoid blocking
//   - Implement exponential backoff for reconnections
//
// Memory considerations:
//
//   - Each Writer maintains a message queue (default: 64 messages)
//   - Large messages are cloned during decoding to prevent buffer reuse issues
//   - Consider implementing message size limits at application level
//
// # Standards Compliance
//
// This implementation strictly follows the W3C Server-Sent Events specification:
//
//   - Handles UTF-8 BOM (U+FEFF) at stream start per spec
//   - Supports all line ending formats (LF, CR, CRLF) for cross-platform compatibility
//   - Validates event names according to DOM naming rules
//   - Escapes newlines in ID and Event fields (replaced with \\n and \\r)
//   - Preserves multiline Data fields with proper line joining
//   - Replaces invalid UTF-8 sequences with U+FFFD replacement character
//   - Implements comment lines (starting with ':') for keep-alive
//   - Correctly handles empty field values per specification
//
// # Limitations
//
// Known limitations of the current implementation:
//
//   - No built-in message size limits (must be enforced at application level)
//   - No automatic reconnection (client must implement retry logic)
//   - Iter() cannot access LastID during iteration (use Reader for reconnection logic)
//   - No built-in authentication/authorization (use HTTP middleware)
//   - No message compression (SSE streams should not be compressed for real-time delivery)
//
// # Comparison with WebSockets
//
// Choose SSE when:
//
//   - Communication is primarily server-to-client
//   - You need automatic reconnection support
//   - You want to use standard HTTP/HTTPS
//   - Firewall/proxy compatibility is important
//   - You need built-in event typing
//
// Choose WebSockets when:
//
//   - You need bidirectional communication
//   - You need binary data support
//   - You need lower latency
//   - You need custom protocols
//
// # Examples
//
// Basic server:
//
//	http.HandleFunc("/events", func(w http.ResponseWriter, r *http.Request) {
//	    writer, _ := sse.NewWriter(&sse.WriterConfig{
//	        Context:        r.Context(),
//	        ResponseWriter: w,
//	    })
//	    defer writer.Close()
//
//	    ticker := time.NewTicker(time.Second)
//	    defer ticker.Stop()
//
//	    for {
//	        select {
//	        case <-r.Context().Done():
//	            return
//	        case t := <-ticker.C:
//	            writer.SendData(map[string]interface{}{
//	                "time": t.Format(time.RFC3339),
//	            })
//	        }
//	    }
//	})
//
// Basic client:
//
//	resp, _ := http.Get("http://localhost:8080/events")
//	for msg, err := range sse.Iter(resp) {
//	    if err != nil {
//	        log.Fatal(err)
//	    }
//	    fmt.Printf("Received: %s\n", msg.Data)
//	}
//
// Fan-out to multiple clients:
//
//	type Hub struct {
//	    clients map[*sse.Writer]bool
//	    mu      sync.Mutex
//	}
//
//	func (h *Hub) Broadcast(msg *sse.Message) {
//	    h.mu.Lock()
//	    defer h.mu.Unlock()
//	    for client := range h.clients {
//	        client.Send(msg)
//	    }
//	}
//
// See package examples for more complete demonstrations.
package sse

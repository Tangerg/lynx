package sse_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/Tangerg/lynx/sse"
)

// TestMessageField represents a typical message payload structure
type TestMessageField struct {
	ID        int   `json:"id"`
	Timestamp int64 `json:"ts"`
}

// testServer wraps httptest.Server for SSE testing
type testServer struct {
	*httptest.Server
	t *testing.T
}

// newTestServer creates a test server with the given handler
func newTestServer(t *testing.T, handler http.HandlerFunc) *testServer {
	t.Helper()

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	return &testServer{
		Server: server,
		t:      t,
	}
}

// newReader creates an SSE reader connected to the test server
func (s *testServer) newReader() (*sse.Reader, error) {
	resp, err := http.Get(s.URL)
	if err != nil {
		return nil, fmt.Errorf("failed to connect: %w", err)
	}

	reader, err := sse.NewReader(resp)
	if err != nil {
		resp.Body.Close()
		return nil, fmt.Errorf("failed to create reader: %w", err)
	}

	return reader, nil
}

// newIterResponse creates an HTTP response for use with sse.Iter
func (s *testServer) newIterResponse() (*http.Response, error) {
	resp, err := http.Get(s.URL)
	if err != nil {
		return nil, fmt.Errorf("failed to connect: %w", err)
	}
	return resp, nil
}

// sseHandlerConfig configures the test SSE handler
type sseHandlerConfig struct {
	messageCount int
	delay        time.Duration
	heartbeat    time.Duration
	queueSize    int
	sendError    bool           // Simulate send errors
	onMessage    func(int) bool // Return false to stop sending
}

// defaultHandlerConfig returns default handler configuration
func defaultHandlerConfig() sseHandlerConfig {
	return sseHandlerConfig{
		messageCount: 10,
		delay:        50 * time.Millisecond,
		heartbeat:    0,
		queueSize:    128,
		sendError:    false,
	}
}

// createSSEHandler creates an SSE handler with the given configuration
func createSSEHandler(cfg sseHandlerConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writer, err := sse.NewWriter(&sse.WriterConfig{
			Context:        r.Context(),
			ResponseWriter: w,
			QueueSize:      cfg.queueSize,
			HeartBeat:      cfg.heartbeat,
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer writer.Close()

		// Small delay to ensure client is ready
		time.Sleep(100 * time.Millisecond)

		for i := 0; i < cfg.messageCount; i++ {
			// Allow custom per-message logic
			if cfg.onMessage != nil && !cfg.onMessage(i) {
				return
			}

			data := TestMessageField{
				ID:        i + 1,
				Timestamp: time.Now().Unix(),
			}

			if err := writer.SendData(data); err != nil {
				if !errors.Is(err, context.Canceled) {
					// Log non-cancellation errors
					fmt.Printf("Send error at message %d: %v\n", i+1, err)
				}
				return
			}

			time.Sleep(cfg.delay)
		}
	}
}

// assertMessageCount checks if the received message count matches expected
func assertMessageCount(t *testing.T, got, want int) {
	t.Helper()
	if got != want {
		t.Errorf("Message count mismatch: got %d, want %d", got, want)
	}
}

// assertNoError checks that error is nil
func assertNoError(t *testing.T, err error, context string) {
	t.Helper()
	if err != nil {
		t.Fatalf("%s: %v", context, err)
	}
}

// logMessage logs a message with proper formatting
func logMessage(t *testing.T, prefix string, msg sse.Message) {
	t.Helper()

	dataStr := string(msg.Data)
	if len(dataStr) > 100 {
		dataStr = dataStr[:100] + "..."
	}

	t.Logf("%s | ID: %q, Event: %q, Data: %s, Retry: %d",
		prefix, msg.ID, msg.Event, dataStr, msg.Retry)
}

// TestSSEBasicReaderWriter tests basic SSE communication
func TestSSEBasicReaderWriter(t *testing.T) {
	cfg := defaultHandlerConfig()
	server := newTestServer(t, createSSEHandler(cfg))

	reader, err := server.newReader()
	assertNoError(t, err, "Creating reader")
	defer reader.Close()

	count := 0
	for reader.Next() {
		assertNoError(t, reader.Error(), "Reading message")

		msg := reader.Current()
		count++
		logMessage(t, fmt.Sprintf("Message %02d", count), msg)

		// Validate JSON structure
		var data TestMessageField
		if err := json.Unmarshal(msg.Data, &data); err != nil {
			t.Errorf("Failed to unmarshal message %d: %v", count, err)
		}

		// Verify sequential IDs
		if data.ID != count {
			t.Errorf("Message %d: ID mismatch, got %d, want %d", count, data.ID, count)
		}
	}

	assertNoError(t, reader.Error(), "Final reader state")
	assertMessageCount(t, count, cfg.messageCount)
	t.Logf("✓ Successfully received all %d messages", count)
}

// TestSSELargeMessageVolume tests handling of large message volumes
func TestSSELargeMessageVolume(t *testing.T) {
	cfg := sseHandlerConfig{
		messageCount: 100,
		delay:        10 * time.Millisecond,
		queueSize:    128,
	}
	server := newTestServer(t, createSSEHandler(cfg))

	reader, err := server.newReader()
	assertNoError(t, err, "Creating reader")
	defer reader.Close()

	count := 0
	startTime := time.Now()

	for reader.Next() {
		assertNoError(t, reader.Error(), "Reading message")

		msg := reader.Current()
		count++

		// Log only first 5, every 10th, and last 5 messages
		if count <= 5 || count%10 == 0 || count > 95 {
			logMessage(t, fmt.Sprintf("Message %03d", count), msg)
		}

		var data TestMessageField
		if err := json.Unmarshal(msg.Data, &data); err != nil {
			t.Errorf("Message %d: unmarshal error: %v", count, err)
		}
	}

	duration := time.Since(startTime)
	assertNoError(t, reader.Error(), "Final reader state")
	assertMessageCount(t, count, cfg.messageCount)

	messagesPerSecond := float64(count) / duration.Seconds()
	t.Logf("✓ Received %d messages in %v (%.2f msg/s)", count, duration, messagesPerSecond)
}

// TestSSEWithIter tests the iterator-based API
func TestSSEWithIter(t *testing.T) {
	cfg := defaultHandlerConfig()
	server := newTestServer(t, createSSEHandler(cfg))

	resp, err := server.newIterResponse()
	assertNoError(t, err, "Creating response")

	count := 0
	for msg, err := range sse.Iter(resp) {
		assertNoError(t, err, fmt.Sprintf("Iter at message %d", count+1))

		count++
		logMessage(t, fmt.Sprintf("Message %02d", count), msg)

		var data TestMessageField
		if err := json.Unmarshal(msg.Data, &data); err != nil {
			t.Errorf("Message %d: unmarshal error: %v", count, err)
		}

		if data.ID != count {
			t.Errorf("Message %d: ID mismatch, got %d, want %d", count, data.ID, count)
		}
	}

	assertMessageCount(t, count, cfg.messageCount)
	t.Logf("✓ Iter successfully processed all %d messages", count)
}

// TestSSELastID verifies Last-Event-ID tracking
func TestSSELastID(t *testing.T) {
	// Handler that sends messages with explicit IDs
	handler := func(w http.ResponseWriter, r *http.Request) {
		writer, err := sse.NewWriter(&sse.WriterConfig{
			Context:        r.Context(),
			ResponseWriter: w,
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer writer.Close()

		for i := 1; i <= 5; i++ {
			msg := &sse.Message{
				ID:    fmt.Sprintf("msg-%d", i),
				Event: "test",
				Data:  []byte(fmt.Sprintf(`{"id":%d}`, i)),
			}
			if err := writer.Send(msg); err != nil {
				return
			}
			time.Sleep(50 * time.Millisecond)
		}
	}

	server := newTestServer(t, handler)
	reader, err := server.newReader()
	assertNoError(t, err, "Creating reader")
	defer reader.Close()

	var lastIDs []string
	count := 0

	for reader.Next() {
		assertNoError(t, reader.Error(), "Reading message")

		msg := reader.Current()
		count++

		currentLastID := reader.LastID()
		lastIDs = append(lastIDs, currentLastID)

		logMessage(t, fmt.Sprintf("Message %02d", count), msg)
		t.Logf("  → LastID: %q", currentLastID)

		if msg.ID != "" && currentLastID != msg.ID {
			t.Errorf("Message %d: LastID mismatch, got %q, want %q", count, currentLastID, msg.ID)
		}
	}

	assertNoError(t, reader.Error(), "Final reader state")
	assertMessageCount(t, count, 5)

	finalLastID := reader.LastID()
	expectedFinalID := "msg-5"
	if finalLastID != expectedFinalID {
		t.Errorf("Final LastID: got %q, want %q", finalLastID, expectedFinalID)
	}

	t.Logf("✓ LastID tracking verified: %v", lastIDs)
}

// TestSSEEarlyTermination tests breaking out of iteration early
func TestSSEEarlyTermination(t *testing.T) {
	cfg := sseHandlerConfig{
		messageCount: 100,
		delay:        10 * time.Millisecond,
	}
	server := newTestServer(t, createSSEHandler(cfg))

	resp, err := server.newIterResponse()
	assertNoError(t, err, "Creating response")

	const maxMessages = 5
	count := 0

	for msg, err := range sse.Iter(resp) {
		assertNoError(t, err, fmt.Sprintf("Iter at message %d", count+1))

		count++
		logMessage(t, fmt.Sprintf("Message %02d", count), msg)

		if count >= maxMessages {
			t.Logf("Breaking early at message %d", count)
			break
		}
	}

	assertMessageCount(t, count, maxMessages)
	t.Logf("✓ Successfully terminated early after %d messages", count)
}

// TestSSEGracefulShutdown tests server-side graceful shutdown
func TestSSEGracefulShutdown(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		writer, err := sse.NewWriter(&sse.WriterConfig{
			Context:        r.Context(),
			ResponseWriter: w,
			CloseTimeout:   2 * time.Second,
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer writer.Close()

		for i := 1; i <= 3; i++ {
			data := TestMessageField{ID: i, Timestamp: time.Now().Unix()}
			if err := writer.SendData(data); err != nil {
				return
			}
			time.Sleep(50 * time.Millisecond)
		}
	}

	server := newTestServer(t, handler)
	reader, err := server.newReader()
	assertNoError(t, err, "Creating reader")
	defer reader.Close()

	count := 0
	for reader.Next() {
		assertNoError(t, reader.Error(), "Reading message")

		msg := reader.Current()
		count++
		logMessage(t, fmt.Sprintf("Message %02d", count), msg)
	}

	// Server closes gracefully, client should receive all messages
	assertNoError(t, reader.Error(), "Final reader state")
	t.Logf("✓ Received %d messages before graceful shutdown", count)
}

// TestSSEConcurrentClients tests multiple concurrent clients
func TestSSEConcurrentClients(t *testing.T) {
	cfg := sseHandlerConfig{
		messageCount: 10,
		delay:        50 * time.Millisecond,
	}
	server := newTestServer(t, createSSEHandler(cfg))

	const numClients = 3
	var wg sync.WaitGroup
	results := make(chan int, numClients)

	for clientID := 1; clientID <= numClients; clientID++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			reader, err := server.newReader()
			if err != nil {
				t.Errorf("Client %d: failed to create reader: %v", id, err)
				results <- -1
				return
			}
			defer reader.Close()

			count := 0
			for reader.Next() {
				if err := reader.Error(); err != nil {
					t.Errorf("Client %d: read error: %v", id, err)
					break
				}

				msg := reader.Current()
				count++
				logMessage(t, fmt.Sprintf("Client %d Message %02d", id, count), msg)
			}

			if err := reader.Error(); err != nil {
				t.Errorf("Client %d: final error: %v", id, err)
			}

			t.Logf("Client %d: received %d messages", id, count)
			results <- count
		}(clientID)

		// Stagger client connections
		time.Sleep(100 * time.Millisecond)
	}

	// Wait for all clients with timeout
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		close(results)
	case <-time.After(10 * time.Second):
		t.Fatal("Test timeout waiting for concurrent clients")
	}

	// Verify all clients received all messages
	for count := range results {
		if count != cfg.messageCount && count != -1 {
			t.Errorf("Client received %d messages, expected %d", count, cfg.messageCount)
		}
	}

	t.Logf("✓ All %d clients completed successfully", numClients)
}

// TestSSEInvalidContentType tests rejection of invalid Content-Type
func TestSSEInvalidContentType(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json") // Wrong content type
		w.Write([]byte(`{"error":"wrong content type"}`))
	}

	server := newTestServer(t, handler)

	resp, err := http.Get(server.URL)
	assertNoError(t, err, "HTTP GET")
	defer resp.Body.Close()

	_, err = sse.NewReader(resp)
	if err == nil {
		t.Fatal("Expected error for invalid Content-Type, got nil")
	}

	t.Logf("✓ Correctly rejected invalid Content-Type: %v", err)
}

// TestSSEContextCancellation tests context cancellation handling
func TestSSEContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	handler := func(w http.ResponseWriter, r *http.Request) {
		writer, err := sse.NewWriter(&sse.WriterConfig{
			Context:        ctx,
			ResponseWriter: w,
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer writer.Close()

		for i := 1; i <= 100; i++ {
			data := TestMessageField{ID: i, Timestamp: time.Now().Unix()}
			if err := writer.SendData(data); err != nil {
				return
			}
			time.Sleep(50 * time.Millisecond)
		}
	}

	server := newTestServer(t, handler)
	reader, err := server.newReader()
	assertNoError(t, err, "Creating reader")
	defer reader.Close()

	count := 0
	// Cancel after receiving 3 messages
	for reader.Next() {
		count++
		if count == 3 {
			cancel()
			t.Log("Context canceled after 3 messages")
		}
	}

	t.Logf("✓ Received %d messages before context cancellation", count)
}

// TestSSEHeartbeat tests heartbeat functionality
func TestSSEHeartbeat(t *testing.T) {
	cfg := sseHandlerConfig{
		messageCount: 5,
		delay:        2 * time.Second, // Long delay between messages
		heartbeat:    500 * time.Millisecond,
		queueSize:    128,
	}
	server := newTestServer(t, createSSEHandler(cfg))

	reader, err := server.newReader()
	assertNoError(t, err, "Creating reader")
	defer reader.Close()

	count := 0
	dataMessages := 0
	startTime := time.Now()

	for reader.Next() {
		assertNoError(t, reader.Error(), "Reading message")

		msg := reader.Current()
		count++

		// Distinguish between data messages and heartbeats
		if msg.Event == "" && len(msg.Data) > 0 {
			dataMessages++
			logMessage(t, fmt.Sprintf("Data Message %02d", dataMessages), msg)
		} else {
			// Likely a heartbeat (comment line, which may not appear as a message)
			t.Logf("Heartbeat or empty message at count %d", count)
		}
	}

	duration := time.Since(startTime)
	assertNoError(t, reader.Error(), "Final reader state")

	t.Logf("✓ Received %d data messages (total events: %d) in %v", dataMessages, count, duration)
	t.Logf("  Expected heartbeats every 500ms during 2s delays")
}

// BenchmarkSSEWriter benchmarks writer throughput
func BenchmarkSSEWriter(b *testing.B) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		writer, err := sse.NewWriter(&sse.WriterConfig{
			Context:        r.Context(),
			ResponseWriter: w,
			QueueSize:      1024,
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer writer.Close()

		data := TestMessageField{ID: 1, Timestamp: time.Now().Unix()}
		for i := 0; i < b.N; i++ {
			if err := writer.SendData(data); err != nil {
				return
			}
		}
	}

	server := httptest.NewServer(http.HandlerFunc(handler))
	defer server.Close()

	resp, err := http.Get(server.URL)
	if err != nil {
		b.Fatal(err)
	}
	defer resp.Body.Close()

	reader, err := sse.NewReader(resp)
	if err != nil {
		b.Fatal(err)
	}
	defer reader.Close()

	b.ResetTimer()

	for reader.Next() {
		_ = reader.Current()
	}

	if err := reader.Error(); err != nil {
		b.Fatal(err)
	}
}

// BenchmarkSSEIterator benchmarks iterator performance
func BenchmarkSSEIterator(b *testing.B) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		writer, err := sse.NewWriter(&sse.WriterConfig{
			Context:        r.Context(),
			ResponseWriter: w,
			QueueSize:      1024,
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer writer.Close()

		data := TestMessageField{ID: 1, Timestamp: time.Now().Unix()}
		for i := 0; i < b.N; i++ {
			if err := writer.SendData(data); err != nil {
				return
			}
		}
	}

	server := httptest.NewServer(http.HandlerFunc(handler))
	defer server.Close()

	resp, err := http.Get(server.URL)
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()

	for msg, err := range sse.Iter(resp) {
		if err != nil {
			b.Fatal(err)
		}
		_ = msg
	}
}

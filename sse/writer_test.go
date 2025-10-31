package sse

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestWriterConfig_validate tests configuration validation
func TestWriterConfig_validate(t *testing.T) {
	tests := []struct {
		name    string
		config  *WriterConfig
		wantErr string
	}{
		{
			name: "valid config",
			config: &WriterConfig{
				Context:        context.Background(),
				ResponseWriter: httptest.NewRecorder(),
			},
			wantErr: "",
		},
		{
			name:    "nil config",
			config:  nil,
			wantErr: "nil config",
		},
		{
			name: "missing context",
			config: &WriterConfig{
				ResponseWriter: httptest.NewRecorder(),
			},
			wantErr: "missing context",
		},
		{
			name: "missing response writer",
			config: &WriterConfig{
				Context: context.Background(),
			},
			wantErr: "missing responseWriter",
		},
		{
			name: "response writer without Flusher",
			config: &WriterConfig{
				Context:        context.Background(),
				ResponseWriter: &nonFlusherWriter{},
			},
			wantErr: "does not implement http.Flusher",
		},
		{
			name: "negative queue size defaults to 64",
			config: &WriterConfig{
				Context:        context.Background(),
				ResponseWriter: httptest.NewRecorder(),
				QueueSize:      -1,
			},
			wantErr: "",
		},
		{
			name: "zero close timeout defaults to 30s",
			config: &WriterConfig{
				Context:        context.Background(),
				ResponseWriter: httptest.NewRecorder(),
				CloseTimeout:   0,
			},
			wantErr: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.validate()

			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("validate() error = %v, want nil", err)
				}
				// Check defaults are set
				if tt.config != nil && tt.config.Context != nil {
					if tt.config.QueueSize != 64 {
						t.Errorf("QueueSize = %d, want 64", tt.config.QueueSize)
					}
					if tt.config.CloseTimeout != 30*time.Second {
						t.Errorf("CloseTimeout = %v, want 30s", tt.config.CloseTimeout)
					}
				}
			} else {
				if err == nil {
					t.Errorf("validate() error = nil, want error containing %q", tt.wantErr)
				} else if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("validate() error = %v, want error containing %q", err, tt.wantErr)
				}
			}
		})
	}
}

// nonFlusherWriter implements http.ResponseWriter but not http.Flusher
type nonFlusherWriter struct{}

func (w *nonFlusherWriter) Header() http.Header       { return http.Header{} }
func (w *nonFlusherWriter) Write([]byte) (int, error) { return 0, nil }
func (w *nonFlusherWriter) WriteHeader(int)           {}

// TestNewWriter tests Writer creation
func TestNewWriter(t *testing.T) {
	t.Run("successful creation", func(t *testing.T) {
		writer, err := NewWriter(&WriterConfig{
			Context:        context.Background(),
			ResponseWriter: httptest.NewRecorder(),
		})

		if err != nil {
			t.Fatalf("NewWriter() error = %v", err)
		}
		defer writer.Close()

		if writer == nil {
			t.Fatal("NewWriter() returned nil")
		}
	})

	t.Run("invalid config", func(t *testing.T) {
		_, err := NewWriter(&WriterConfig{})
		if err == nil {
			t.Error("NewWriter() with invalid config should return error")
		}
	})
}

// TestWriter_Headers tests SSE header setting
func TestWriter_Headers(t *testing.T) {
	recorder := httptest.NewRecorder()
	writer, _ := NewWriter(&WriterConfig{
		Context:        context.Background(),
		ResponseWriter: recorder,
	})
	defer writer.Close()

	// Give time for headers to be set
	time.Sleep(10 * time.Millisecond)

	tests := []struct {
		key  string
		want string
	}{
		{"Content-Type", "text/event-stream; charset=utf-8"},
		{"Connection", "keep-alive"},
		{"Cache-Control", "no-cache"},
	}

	for _, tt := range tests {
		if got := recorder.Header().Get(tt.key); got != tt.want {
			t.Errorf("Header %q = %q, want %q", tt.key, got, tt.want)
		}
	}

	if recorder.Code != http.StatusOK {
		t.Errorf("Status code = %d, want %d", recorder.Code, http.StatusOK)
	}
}

// TestWriter_Send tests basic message sending
func TestWriter_Send(t *testing.T) {
	t.Run("send single message", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		writer, _ := NewWriter(&WriterConfig{
			Context:        context.Background(),
			ResponseWriter: recorder,
		})
		defer writer.Close()

		err := writer.Send(&Message{
			ID:    "1",
			Event: "test",
			Data:  []byte("hello"),
		})
		if err != nil {
			t.Fatalf("Send() error = %v", err)
		}

		// Wait for message to be processed
		time.Sleep(50 * time.Millisecond)

		body := recorder.Body.String()
		expectations := []string{"id: 1", "event: test", "data: hello"}
		for _, expect := range expectations {
			if !strings.Contains(body, expect) {
				t.Errorf("Body missing %q, got: %s", expect, body)
			}
		}
	})

	t.Run("send after close", func(t *testing.T) {
		writer, _ := NewWriter(&WriterConfig{
			Context:        context.Background(),
			ResponseWriter: httptest.NewRecorder(),
		})
		writer.Close()

		err := writer.Send(&Message{Data: []byte("test")})
		if err == nil {
			t.Error("Send() after close should return error")
		}
		if !strings.Contains(err.Error(), "closed") {
			t.Errorf("Error = %v, want 'closed' error", err)
		}
	})

	t.Run("concurrent sends", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		writer, _ := NewWriter(&WriterConfig{
			Context:        context.Background(),
			ResponseWriter: recorder,
			QueueSize:      100,
		})
		defer writer.Close()

		const numGoroutines = 10
		const messagesPerGoroutine = 5

		var wg sync.WaitGroup
		wg.Add(numGoroutines)

		for i := 0; i < numGoroutines; i++ {
			go func() {
				defer wg.Done()
				for j := 0; j < messagesPerGoroutine; j++ {
					if err := writer.Send(&Message{Data: []byte("test")}); err != nil {
						t.Errorf("Send() error = %v", err)
					}
				}
			}()
		}

		wg.Wait()
		time.Sleep(100 * time.Millisecond)

		body := recorder.Body.String()
		count := strings.Count(body, "data: test")
		expected := numGoroutines * messagesPerGoroutine
		if count != expected {
			t.Errorf("Sent %d messages, want %d", count, expected)
		}
	})
}

// TestWriter_SendEvent tests event sending
func TestWriter_SendEvent(t *testing.T) {
	recorder := httptest.NewRecorder()
	writer, _ := NewWriter(&WriterConfig{
		Context:        context.Background(),
		ResponseWriter: recorder,
	})
	defer writer.Close()

	err := writer.SendEvent("heartbeat")
	if err != nil {
		t.Fatalf("SendEvent() error = %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	body := recorder.Body.String()
	if !strings.Contains(body, "event: heartbeat") {
		t.Errorf("Body missing 'event: heartbeat', got: %s", body)
	}
	// Should contain minimal data
	if !strings.Contains(body, "data:") {
		t.Error("Body missing data field for compatibility")
	}
}

// TestWriter_SendData tests JSON data sending
func TestWriter_SendData(t *testing.T) {
	type TestData struct {
		Message string `json:"message"`
		Count   int    `json:"count"`
	}

	recorder := httptest.NewRecorder()
	writer, _ := NewWriter(&WriterConfig{
		Context:        context.Background(),
		ResponseWriter: recorder,
	})
	defer writer.Close()

	data := TestData{Message: "hello", Count: 42}
	err := writer.SendData(data)
	if err != nil {
		t.Fatalf("SendData() error = %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	body := recorder.Body.String()
	expectations := []string{`"message":"hello"`, `"count":42`}
	for _, expect := range expectations {
		if !strings.Contains(body, expect) {
			t.Errorf("Body missing %q, got: %s", expect, body)
		}
	}
}

// TestWriter_Heartbeat tests heartbeat functionality
func TestWriter_Heartbeat(t *testing.T) {
	recorder := httptest.NewRecorder()
	writer, _ := NewWriter(&WriterConfig{
		Context:        context.Background(),
		ResponseWriter: recorder,
		HeartBeat:      50 * time.Millisecond,
	})
	defer writer.Close()

	// Wait for multiple heartbeats
	time.Sleep(150 * time.Millisecond)

	body := recorder.Body.String()
	count := strings.Count(body, ": ping")
	if count < 2 {
		t.Errorf("Expected at least 2 heartbeats, got %d", count)
	}
}

// TestWriter_Close tests graceful closure
func TestWriter_Close(t *testing.T) {
	t.Run("close waits for messages", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		writer, _ := NewWriter(&WriterConfig{
			Context:        context.Background(),
			ResponseWriter: recorder,
			QueueSize:      50,
		})

		// Send multiple messages
		for i := 0; i < 10; i++ {
			_ = writer.Send(&Message{Data: []byte("test")})
		}

		err := writer.Close()
		if err != nil {
			t.Errorf("Close() error = %v", err)
		}

		// All messages should be sent
		body := recorder.Body.String()
		count := strings.Count(body, "data: test")
		if count != 10 {
			t.Errorf("Sent %d messages, want 10", count)
		}

		// Should end with double newline
		if !strings.HasSuffix(body, "\n\n") {
			t.Error("Stream should end with \\n\\n")
		}
	})

	t.Run("multiple close calls are safe", func(t *testing.T) {
		writer, _ := NewWriter(&WriterConfig{
			Context:        context.Background(),
			ResponseWriter: httptest.NewRecorder(),
		})

		err1 := writer.Close()
		err2 := writer.Close()

		if err1 != nil {
			t.Errorf("First Close() error = %v", err1)
		}
		if err2 != nil {
			t.Errorf("Second Close() error = %v", err2)
		}
	})

	t.Run("close with timeout", func(t *testing.T) {
		writer, _ := NewWriter(&WriterConfig{
			Context:        context.Background(),
			ResponseWriter: &slowWriter{delay: 200 * time.Millisecond},
			CloseTimeout:   50 * time.Millisecond,
			QueueSize:      50,
		})

		// Fill queue
		for i := 0; i < 20; i++ {
			writer.Send(&Message{Data: []byte("test")})
		}

		err := writer.Close()
		if err == nil {
			t.Error("Close() should timeout")
		}
		if !strings.Contains(err.Error(), "timeout") {
			t.Errorf("Close() error = %v, want timeout error", err)
		}
	})
}

// slowWriter simulates slow network
type slowWriter struct {
	delay time.Duration
}

func (w *slowWriter) Header() http.Header { return http.Header{} }
func (w *slowWriter) Write(p []byte) (int, error) {
	time.Sleep(w.delay)
	return len(p), nil
}
func (w *slowWriter) WriteHeader(int) {}
func (w *slowWriter) Flush()          {}

// TestWriter_ContextCancellation tests context handling
func TestWriter_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	recorder := httptest.NewRecorder()

	writer, _ := NewWriter(&WriterConfig{
		Context:        ctx,
		ResponseWriter: recorder,
	})

	cancel()
	time.Sleep(50 * time.Millisecond)

	if !writer.IsClosed() {
		t.Error("Writer should be closed after context cancellation")
	}

	err := writer.Error()
	if err == nil {
		t.Error("Error() should return context error")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Error() = %v, want context.Canceled", err)
	}
}

// TestWriter_ErrorCallback tests error callback
func TestWriter_ErrorCallback(t *testing.T) {
	var callbackCalled bool
	var callbackErr error
	var mu sync.Mutex

	onError := func(err error) {
		mu.Lock()
		callbackCalled = true
		callbackErr = err
		mu.Unlock()
	}

	writer, _ := NewWriter(&WriterConfig{
		Context:        context.Background(),
		ResponseWriter: httptest.NewRecorder(),
		OnError:        onError,
	})
	defer writer.Close()

	testErr := errors.New("test error")
	writer.recordError(testErr)

	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	if !callbackCalled {
		t.Error("OnError callback was not called")
	}
	if !errors.Is(callbackErr, testErr) {
		t.Errorf("Callback error = %v, want %v", callbackErr, testErr)
	}
}

// TestWriter_Error tests error aggregation
func TestWriter_Error(t *testing.T) {
	t.Run("no errors", func(t *testing.T) {
		writer, _ := NewWriter(&WriterConfig{
			Context:        context.Background(),
			ResponseWriter: httptest.NewRecorder(),
		})
		defer writer.Close()

		if err := writer.Error(); err != nil {
			t.Errorf("Error() = %v, want nil", err)
		}
	})

	t.Run("multiple errors", func(t *testing.T) {
		writer, _ := NewWriter(&WriterConfig{
			Context:        context.Background(),
			ResponseWriter: httptest.NewRecorder(),
		})
		defer writer.Close()

		err1 := errors.New("error 1")
		err2 := errors.New("error 2")

		writer.recordError(err1)
		writer.recordError(err2)

		err := writer.Error()
		if err == nil {
			t.Fatal("Error() should return joined errors")
		}

		errStr := err.Error()
		if !strings.Contains(errStr, "error 1") || !strings.Contains(errStr, "error 2") {
			t.Errorf("Error() = %v, want both errors", err)
		}
	})
}

// TestWriter_IsClosed tests close status check
func TestWriter_IsClosed(t *testing.T) {
	writer, _ := NewWriter(&WriterConfig{
		Context:        context.Background(),
		ResponseWriter: httptest.NewRecorder(),
	})

	if writer.IsClosed() {
		t.Error("New writer should not be closed")
	}

	writer.Close()

	if !writer.IsClosed() {
		t.Error("Writer should be closed after Close()")
	}
}

// TestWriter_InvalidMessages tests error handling for invalid messages
func TestWriter_InvalidMessages(t *testing.T) {
	writer, _ := NewWriter(&WriterConfig{
		Context:        context.Background(),
		ResponseWriter: httptest.NewRecorder(),
	})
	defer writer.Close()

	tests := []struct {
		name string
		msg  *Message
	}{
		{
			name: "empty message",
			msg:  &Message{},
		},
		{
			name: "invalid event name",
			msg: &Message{
				Event: ".invalid",
				Data:  []byte("test"),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := writer.Send(tt.msg)
			if err == nil {
				t.Error("Send() with invalid message should return error")
			}
		})
	}
}

// TestWriter_ConcurrentOperations tests thread safety
func TestWriter_ConcurrentOperations(t *testing.T) {
	writer, _ := NewWriter(&WriterConfig{
		Context:        context.Background(),
		ResponseWriter: httptest.NewRecorder(),
		QueueSize:      1000,
	})
	defer writer.Close()

	var wg sync.WaitGroup
	operations := []func(){
		func() { writer.Send(&Message{Data: []byte("test")}) },
		func() { writer.SendEvent("event") },
		func() { writer.SendData(map[string]string{"key": "value"}) },
		func() { writer.Error() },
		func() { writer.IsClosed() },
	}

	// Run 100 concurrent operations of each type
	for _, op := range operations {
		for i := 0; i < 100; i++ {
			wg.Add(1)
			go func(operation func()) {
				defer wg.Done()
				operation()
			}(op)
		}
	}

	wg.Wait()
	// Test passes if no race conditions detected
}

// BenchmarkWriter_Send benchmarks message sending
func BenchmarkWriter_Send(b *testing.B) {
	writer, _ := NewWriter(&WriterConfig{
		Context:        context.Background(),
		ResponseWriter: httptest.NewRecorder(),
		QueueSize:      10000,
	})
	defer writer.Close()

	msg := &Message{
		ID:    "bench",
		Event: "test",
		Data:  []byte("benchmark data"),
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		writer.Send(msg)
	}
}

// BenchmarkWriter_SendData benchmarks JSON sending
func BenchmarkWriter_SendData(b *testing.B) {
	writer, _ := NewWriter(&WriterConfig{
		Context:        context.Background(),
		ResponseWriter: httptest.NewRecorder(),
		QueueSize:      10000,
	})
	defer writer.Close()

	data := map[string]interface{}{
		"id":      123,
		"message": "benchmark",
		"count":   456,
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		writer.SendData(data)
	}
}

// BenchmarkWriter_Concurrent benchmarks concurrent operations
func BenchmarkWriter_Concurrent(b *testing.B) {
	writer, _ := NewWriter(&WriterConfig{
		Context:        context.Background(),
		ResponseWriter: httptest.NewRecorder(),
		QueueSize:      10000,
	})
	defer writer.Close()

	msg := &Message{Data: []byte("test")}

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			writer.Send(msg)
		}
	})
}

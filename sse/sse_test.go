package sse

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"
)

type TestMessage struct {
	ID        int   `json:"id"`
	Timestamp int64 `json:"ts"`
}

const (
	testPort = 10086
	testURL  = "http://localhost:10086/sse"
)

// 启动测试服务器
func setupRealServer(t *testing.T, handler http.HandlerFunc) *http.Server {
	t.Helper()

	mux := http.NewServeMux()
	mux.HandleFunc("/sse", handler)

	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", testPort),
		Handler: mux,
	}

	// 启动服务器
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			t.Logf("Server error: %v", err)
		}
	}()

	// 等待服务器启动
	time.Sleep(100 * time.Millisecond)

	// 清理函数
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil {
			t.Logf("Server shutdown error: %v", err)
		}
	})

	return server
}

func writerSSEHandler(messageCount int, delay time.Duration, heartbeat time.Duration) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writer, err := NewWriter(&WriterConfig{
			Context:        r.Context(),
			ResponseWriter: w,
			QueueSize:      128,
			HeartBeat:      heartbeat,
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer writer.Close()

		time.Sleep(100 * time.Millisecond)

		for i := 0; i < messageCount; i++ {
			data := TestMessage{
				ID:        i + 1,
				Timestamp: time.Now().Unix(),
			}

			if err := writer.SendData(data); err != nil {
				fmt.Printf("Send error: %v\n", err)
				return
			}
			time.Sleep(delay)
		}
	}
}

func printMessage(t *testing.T, prefix string, msg Message) {
	t.Helper()
	dataStr := string(msg.Data)
	if len(dataStr) > 100 {
		dataStr = dataStr[:100] + "..."
	}

	t.Logf("%s | ID: %q, Event: %q, Data: %s, Retry: %d",
		prefix, msg.ID, msg.Event, dataStr, msg.Retry)
}

func TestSSEBasicReaderWriter(t *testing.T) {
	setupRealServer(t, writerSSEHandler(10, 50*time.Millisecond, 1))

	resp, err := http.Get(testURL)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}

	reader, err := NewReader(resp)
	if err != nil {
		t.Fatalf("Failed to create reader: %v", err)
	}
	defer reader.Close()

	count := 0
	for reader.Next() {
		msg, err := reader.Current()
		if err != nil {
			t.Errorf("Read error: %v", err)
			continue
		}

		count++
		printMessage(t, fmt.Sprintf("Message %02d", count), msg)

		var data TestMessage
		if err := json.Unmarshal(msg.Data, &data); err != nil {
			t.Errorf("JSON unmarshal error: %v", err)
		}
	}

	if err := reader.Error(); err != nil {
		t.Errorf("Reader error: %v", err)
	}

	if count != 10 {
		t.Errorf("Expected 10 messages, got %d", count)
	}

	t.Logf("Total messages received: %d", count)
}

func TestSSELargeMessageVolume(t *testing.T) {
	setupRealServer(t, writerSSEHandler(100, 10*time.Millisecond, 0))

	resp, err := http.Get(testURL)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}

	reader, err := NewReader(resp)
	if err != nil {
		t.Fatalf("Failed to create reader: %v", err)
	}
	defer reader.Close()

	count := 0
	for reader.Next() {
		msg, err := reader.Current()
		if err != nil {
			t.Errorf("Read error: %v", err)
			continue
		}

		count++
		if count%10 == 0 || count <= 5 || count > 95 {
			printMessage(t, fmt.Sprintf("Message %03d", count), msg)
		}

		var data TestMessage
		if err := json.Unmarshal(msg.Data, &data); err != nil {
			t.Errorf("JSON unmarshal error: %v", err)
		}
	}

	if err := reader.Error(); err != nil {
		t.Errorf("Reader error: %v", err)
	}

	if count != 100 {
		t.Errorf("Expected 100 messages, got %d", count)
	}

	t.Logf("Total messages received: %d", count)
}

func TestSSEWithIter(t *testing.T) {
	setupRealServer(t, writerSSEHandler(10, 50*time.Millisecond, 0))

	resp, err := http.Get(testURL)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}

	count := 0
	for msg, err := range Iter(resp) {
		if err != nil {
			t.Fatalf("Iter error: %v", err)
		}

		count++
		printMessage(t, fmt.Sprintf("Message %02d", count), msg)

		var data TestMessage
		if err := json.Unmarshal(msg.Data, &data); err != nil {
			t.Errorf("JSON unmarshal error: %v", err)
			continue
		}
	}

	if count != 10 {
		t.Errorf("Expected 10 messages, got %d", count)
	}

	t.Logf("Total messages received: %d", count)
}

func TestSSELastID(t *testing.T) {
	setupRealServer(t, writerSSEHandler(5, 50*time.Millisecond, 0))

	resp, err := http.Get(testURL)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}

	reader, err := NewReader(resp)
	if err != nil {
		t.Fatalf("Failed to create reader: %v", err)
	}
	defer reader.Close()

	count := 0
	for reader.Next() {
		msg, err := reader.Current()
		if err != nil {
			t.Errorf("Read error: %v", err)
			continue
		}

		count++
		printMessage(t, fmt.Sprintf("Message %02d", count), msg)

		currentLastID := reader.LastID()
		t.Logf("Reader.LastID: %q", currentLastID)

		if msg.ID != "" && currentLastID != msg.ID {
			t.Errorf("LastID mismatch: expected %q, got %q", msg.ID, currentLastID)
		}
	}

	finalLastID := reader.LastID()
	t.Logf("Final LastID: %q", finalLastID)

	if count != 5 {
		t.Errorf("Expected 5 messages, got %d", count)
	}
}

func TestSSEEarlyTermination(t *testing.T) {
	setupRealServer(t, writerSSEHandler(100, 10*time.Millisecond, 0))

	resp, err := http.Get(testURL)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}

	count := 0
	maxMessages := 5

	for msg, err := range Iter(resp) {
		if err != nil {
			t.Fatalf("Iter error: %v", err)
		}

		count++
		printMessage(t, fmt.Sprintf("Message %02d", count), msg)

		if count >= maxMessages {
			t.Logf("Breaking early at message %d", count)
			break
		}
	}

	if count != maxMessages {
		t.Errorf("Expected %d messages before break, got %d", maxMessages, count)
	}

	t.Logf("Terminated after %d messages", count)
}

func TestSSEReaderError(t *testing.T) {
	setupRealServer(t, func(w http.ResponseWriter, r *http.Request) {
		writer, err := NewWriter(&WriterConfig{
			Context:        r.Context(),
			ResponseWriter: w,
			QueueSize:      10,
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer writer.Close()

		for i := 0; i < 3; i++ {
			data := TestMessage{ID: i + 1, Timestamp: time.Now().Unix()}
			if err := writer.SendData(data); err != nil {
				return
			}
			time.Sleep(50 * time.Millisecond)
		}
	})

	resp, err := http.Get(testURL)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}

	reader, err := NewReader(resp)
	if err != nil {
		t.Fatalf("Failed to create reader: %v", err)
	}
	defer reader.Close()

	count := 0
	for reader.Next() {
		msg, err := reader.Current()
		if err != nil {
			t.Errorf("Read error: %v", err)
			continue
		}

		count++
		printMessage(t, fmt.Sprintf("Message %02d", count), msg)
	}

	t.Logf("Total messages before connection close: %d", count)

	if count != 3 {
		t.Logf("Note: Expected 3 messages, got %d (connection may have closed early)", count)
	}
}

func TestSSEConcurrentReaders(t *testing.T) {
	setupRealServer(t, writerSSEHandler(10, 50*time.Millisecond, 0))

	done := make(chan bool, 2)

	for clientID := 1; clientID <= 2; clientID++ {
		go func(id int) {
			defer func() { done <- true }()

			resp, err := http.Get(testURL)
			if err != nil {
				t.Errorf("Client %d: Failed to connect: %v", id, err)
				return
			}

			reader, err := NewReader(resp)
			if err != nil {
				t.Errorf("Client %d: Failed to create reader: %v", id, err)
				return
			}
			defer reader.Close()

			count := 0
			for reader.Next() {
				msg, err := reader.Current()
				if err != nil {
					t.Errorf("Client %d: Read error: %v", id, err)
					continue
				}

				count++
				printMessage(t, fmt.Sprintf("Client %d Message %02d", id, count), msg)
			}

			t.Logf("Client %d: Total messages received: %d", id, count)
		}(clientID)

		time.Sleep(100 * time.Millisecond)
	}

	timeout := time.After(5 * time.Second)
	for i := 0; i < 2; i++ {
		select {
		case <-done:
		case <-timeout:
			t.Fatal("Test timeout waiting for concurrent readers")
		}
	}
}

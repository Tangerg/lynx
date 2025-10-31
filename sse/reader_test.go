package sse

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

type mockResponseBody struct {
	*bytes.Reader
	closed bool
}

func (m *mockResponseBody) Close() error {
	m.closed = true
	return nil
}

func createMockResponse(contentType, body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type": []string{contentType},
		},
		Body: &mockResponseBody{
			Reader: bytes.NewReader([]byte(body)),
		},
	}
}

func TestNewReader(t *testing.T) {
	tests := []struct {
		name        string
		response    *http.Response
		wantErr     bool
		errContains string
	}{
		{
			name:        "nil response",
			response:    nil,
			wantErr:     true,
			errContains: "nil http response",
		},
		{
			name: "missing content-type",
			response: &http.Response{
				Header: http.Header{},
				Body:   io.NopCloser(strings.NewReader("")),
			},
			wantErr:     true,
			errContains: "missing Content-Type",
		},
		{
			name: "wrong content-type",
			response: &http.Response{
				Header: http.Header{
					"Content-Type": []string{"application/json"},
				},
				Body: io.NopCloser(strings.NewReader("")),
			},
			wantErr:     true,
			errContains: "expected Content-Type 'text/event-stream'",
		},
		{
			name: "valid content-type",
			response: &http.Response{
				Header: http.Header{
					"Content-Type": []string{"text/event-stream"},
				},
				Body: io.NopCloser(strings.NewReader("")),
			},
			wantErr: false,
		},
		{
			name: "content-type with charset",
			response: &http.Response{
				Header: http.Header{
					"Content-Type": []string{"text/event-stream; charset=utf-8"},
				},
				Body: io.NopCloser(strings.NewReader("")),
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader, err := NewReader(tt.response)

			if tt.wantErr {
				if err == nil {
					t.Errorf("NewReader() expected error, got nil")
				} else if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("NewReader() error = %v, want error containing %q", err, tt.errContains)
				}
			} else {
				if err != nil {
					t.Errorf("NewReader() unexpected error = %v", err)
				}
				if reader == nil {
					t.Error("NewReader() returned nil reader")
				}
			}
		})
	}
}

func TestReader_Next_Current(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		expected []Message
	}{
		{
			name: "single message",
			body: "data: hello\n\n",
			expected: []Message{
				{Data: []byte("hello")},
			},
		},
		{
			name: "multiple messages",
			body: "data: first\n\ndata: second\n\n",
			expected: []Message{
				{Data: []byte("first")},
				{Data: []byte("second")},
			},
		},
		{
			name: "message with id and event",
			body: "id: 123\nevent: update\ndata: test\n\n",
			expected: []Message{
				{ID: "123", Event: "update", Data: []byte("test")},
			},
		},
		{
			name: "multiline data",
			body: "data: line1\ndata: line2\n\n",
			expected: []Message{
				{Data: []byte("line1\nline2")},
			},
		},
		{
			name:     "empty stream",
			body:     "",
			expected: []Message{},
		},
		{
			name:     "empty data field",
			body:     "data:\n\n",
			expected: []Message{},
		},
		{
			name: "binary data",
			body: "data: \x00\x01\x02\n\n",
			expected: []Message{
				{Data: []byte{0x00, 0x01, 0x02}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := createMockResponse("text/event-stream", tt.body)
			reader, err := NewReader(resp)
			if err != nil {
				t.Fatalf("NewReader() error = %v", err)
			}
			defer reader.Close()

			var messages []Message
			for reader.Next() {
				messages = append(messages, reader.Current())
			}

			if err := reader.Error(); err != nil {
				t.Errorf("Reader.Error() = %v, want nil", err)
			}

			if len(messages) != len(tt.expected) {
				t.Fatalf("got %d messages, want %d", len(messages), len(tt.expected))
			}

			for i, msg := range messages {
				if msg.ID != tt.expected[i].ID {
					t.Errorf("message[%d].ID = %q, want %q", i, msg.ID, tt.expected[i].ID)
				}
				if msg.Event != tt.expected[i].Event {
					t.Errorf("message[%d].Event = %q, want %q", i, msg.Event, tt.expected[i].Event)
				}
				if !bytes.Equal(msg.Data, tt.expected[i].Data) {
					t.Errorf("message[%d].Data = %q, want %q", i, msg.Data, tt.expected[i].Data)
				}
			}
		})
	}
}

func TestReader_LastID(t *testing.T) {
	body := "id: 1\ndata: first\n\nid: 2\ndata: second\n\ndata: third\n\n"
	resp := createMockResponse("text/event-stream", body)

	reader, err := NewReader(resp)
	if err != nil {
		t.Fatalf("NewReader() error = %v", err)
	}
	defer reader.Close()

	if id := reader.LastID(); id != "" {
		t.Errorf("initial LastID() = %q, want empty", id)
	}

	if !reader.Next() {
		t.Fatal("expected first message")
	}
	if id := reader.LastID(); id != "1" {
		t.Errorf("after first message LastID() = %q, want %q", id, "1")
	}

	if !reader.Next() {
		t.Fatal("expected second message")
	}
	if id := reader.LastID(); id != "2" {
		t.Errorf("after second message LastID() = %q, want %q", id, "2")
	}

	if !reader.Next() {
		t.Fatal("expected third message")
	}
	if id := reader.LastID(); id != "2" {
		t.Errorf("after third message LastID() = %q, want %q (should keep previous)", id, "2")
	}
}

func TestReader_Close(t *testing.T) {
	body := "data: test\n\n"
	resp := createMockResponse("text/event-stream", body)
	mockBody := resp.Body.(*mockResponseBody)

	reader, err := NewReader(resp)
	if err != nil {
		t.Fatalf("NewReader() error = %v", err)
	}

	if mockBody.closed {
		t.Error("body should not be closed before Close()")
	}

	if err := reader.Close(); err != nil {
		t.Errorf("Close() error = %v", err)
	}

	if !mockBody.closed {
		t.Error("body should be closed after Close()")
	}
}

func TestIter(t *testing.T) {
	tests := []struct {
		name        string
		body        string
		wantCount   int
		wantErr     bool
		contentType string
	}{
		{
			name:        "normal iteration",
			contentType: "text/event-stream",
			body:        "data: msg1\n\ndata: msg2\n\ndata: msg3\n\n",
			wantCount:   3,
			wantErr:     false,
		},
		{
			name:        "empty stream",
			contentType: "text/event-stream",
			body:        "",
			wantCount:   0,
			wantErr:     false,
		},
		{
			name:        "invalid content-type",
			contentType: "application/json",
			body:        "",
			wantCount:   0,
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := createMockResponse(tt.contentType, tt.body)
			mockBody := resp.Body.(*mockResponseBody)

			count := 0
			gotErr := false

			for msg, err := range Iter(resp) {
				if err != nil {
					gotErr = true
					break
				}
				count++
				if len(msg.Data) == 0 && tt.wantCount > 0 {
					t.Error("got empty message data")
				}
			}

			if gotErr != tt.wantErr {
				t.Errorf("Iter() gotErr = %v, want %v", gotErr, tt.wantErr)
			}

			if count != tt.wantCount {
				t.Errorf("Iter() processed %d messages, want %d", count, tt.wantCount)
			}

			if !mockBody.closed && !tt.wantErr {
				t.Error("Iter() should close the response body")
			}
		})
	}
}

func TestIter_EarlyBreak(t *testing.T) {
	body := "data: msg1\n\ndata: msg2\n\ndata: msg3\n\n"
	resp := createMockResponse("text/event-stream", body)
	mockBody := resp.Body.(*mockResponseBody)

	count := 0
	for _, err := range Iter(resp) {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		count++
		if count == 2 {
			break
		}
	}

	if count != 2 {
		t.Errorf("processed %d messages before break, want 2", count)
	}

	if !mockBody.closed {
		t.Error("body should be closed even with early break")
	}
}

func TestRead(t *testing.T) {
	body := "data: test\n\n"
	resp := createMockResponse("text/event-stream", body)

	count := 0
	for msg, err := range Read(resp) {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		count++
		if !bytes.Equal(msg.Data, []byte("test")) {
			t.Errorf("got data %q, want %q", msg.Data, "test")
		}
	}

	if count != 1 {
		t.Errorf("Read() processed %d messages, want 1", count)
	}
}

type errorReader struct {
	err error
}

func (e *errorReader) Read(p []byte) (n int, err error) {
	return 0, e.err
}

func (e *errorReader) Close() error {
	return nil
}

func TestReader_Error(t *testing.T) {
	expectedErr := errors.New("read error")
	resp := &http.Response{
		Header: http.Header{
			"Content-Type": []string{"text/event-stream"},
		},
		Body: &errorReader{err: expectedErr},
	}

	reader, err := NewReader(resp)
	if err != nil {
		t.Fatalf("NewReader() error = %v", err)
	}
	defer reader.Close()

	if reader.Next() {
		t.Error("Next() should return false on read error")
	}

	if err := reader.Error(); err == nil {
		t.Error("Error() should return the read error")
	}
}

func TestReader_DataNotModified(t *testing.T) {
	body := "data: original\n\n"
	resp := createMockResponse("text/event-stream", body)

	reader, err := NewReader(resp)
	if err != nil {
		t.Fatalf("NewReader() error = %v", err)
	}
	defer reader.Close()

	if !reader.Next() {
		t.Fatal("expected message")
	}

	msg := reader.Current()
	originalData := make([]byte, len(msg.Data))
	copy(originalData, msg.Data)

	msg.Data[0] = 'X'

	if !bytes.Equal(originalData, []byte("original")) {
		t.Error("original data should not be modified")
	}
}

func BenchmarkReader(b *testing.B) {
	var buf bytes.Buffer
	for i := 0; i < 100; i++ {
		buf.WriteString("id: ")
		buf.WriteString(string(rune('0' + i%10)))
		buf.WriteString("\nevent: test\ndata: message ")
		buf.WriteString(string(rune('0' + i%10)))
		buf.WriteString("\n\n")
	}
	body := buf.String()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resp := createMockResponse("text/event-stream", body)
		reader, _ := NewReader(resp)

		for reader.Next() {
			_ = reader.Current()
		}
		reader.Close()
	}
}

func BenchmarkIter(b *testing.B) {
	var buf bytes.Buffer
	for i := 0; i < 100; i++ {
		buf.WriteString("data: message\n\n")
	}
	body := buf.String()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resp := createMockResponse("text/event-stream", body)

		for _, err := range Iter(resp) {
			if err != nil {
				b.Fatal(err)
			}
		}
	}
}

func BenchmarkReader_LargeData(b *testing.B) {
	largeData := bytes.Repeat([]byte("x"), 10000)
	body := "data: " + string(largeData) + "\n\n"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resp := createMockResponse("text/event-stream", body)
		reader, _ := NewReader(resp)

		for reader.Next() {
			_ = reader.Current().Data
		}
		reader.Close()
	}
}

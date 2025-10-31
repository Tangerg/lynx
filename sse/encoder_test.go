package sse

import (
	"bytes"
	"strings"
	"testing"
)

// TestNewEncoder tests encoder creation
func TestNewEncoder(t *testing.T) {
	encoder := NewEncoder()
	if encoder == nil {
		t.Fatal("NewEncoder() returned nil")
	}
}

// TestEncoder_isValidMessage tests message validation logic
func TestEncoder_isValidMessage(t *testing.T) {
	encoder := NewEncoder()

	tests := []struct {
		name    string
		message *Message
		want    bool
	}{
		{
			name: "message with data only",
			message: &Message{
				Data: []byte("hello"),
			},
			want: true,
		},
		{
			name: "message with ID only",
			message: &Message{
				ID: "123",
			},
			want: true,
		},
		{
			name: "message with Event only",
			message: &Message{
				Event: "update",
			},
			want: true,
		},
		{
			name: "message with all fields",
			message: &Message{
				ID:    "123",
				Event: "update",
				Data:  []byte("hello"),
				Retry: 3000,
			},
			want: true,
		},
		{
			name: "message with ID and Event",
			message: &Message{
				ID:    "123",
				Event: "update",
			},
			want: true,
		},
		{
			name: "message with ID and Data",
			message: &Message{
				ID:   "123",
				Data: []byte("hello"),
			},
			want: true,
		},
		{
			name: "message with Event and Data",
			message: &Message{
				Event: "update",
				Data:  []byte("hello"),
			},
			want: true,
		},
		{
			name: "message with Retry only",
			message: &Message{
				Retry: 3000,
			},
			want: false, // Retry alone is not enough
		},
		{
			name:    "empty message",
			message: &Message{},
			want:    false,
		},
		{
			name: "message with empty data slice",
			message: &Message{
				Data: []byte{},
			},
			want: false,
		},
		{
			name: "message with empty strings",
			message: &Message{
				ID:    "",
				Event: "",
				Data:  []byte(""),
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := encoder.isValidMessage(tt.message)
			if got != tt.want {
				t.Errorf("isValidMessage() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestEncoder_writeID tests ID field encoding
func TestEncoder_writeID(t *testing.T) {
	encoder := NewEncoder()

	tests := []struct {
		name string
		id   string
		want string
	}{
		{
			name: "simple ID",
			id:   "123",
			want: "id: 123\n",
		},
		{
			name: "ID with newline",
			id:   "123\n456",
			want: "id: 123\\n456\n",
		},
		{
			name: "ID with carriage return",
			id:   "123\r456",
			want: "id: 123\\r456\n",
		},
		{
			name: "ID with CRLF",
			id:   "123\r\n456",
			want: "id: 123\\r\\n456\n",
		},
		{
			name: "empty ID",
			id:   "",
			want: "",
		},
		{
			name: "ID with multiple line breaks",
			id:   "line1\nline2\rline3",
			want: "id: line1\\nline2\\rline3\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := &bytes.Buffer{}
			encoder.writeID(tt.id, buf)
			got := buf.String()
			if got != tt.want {
				t.Errorf("writeID() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestEncoder_writeEvent tests Event field encoding
func TestEncoder_writeEvent(t *testing.T) {
	encoder := NewEncoder()

	tests := []struct {
		name  string
		event string
		want  string
	}{
		{
			name:  "simple event",
			event: "update",
			want:  "event: update\n",
		},
		{
			name:  "event with newline",
			event: "user\nlogin",
			want:  "event: user\\nlogin\n",
		},
		{
			name:  "event with carriage return",
			event: "user\rlogin",
			want:  "event: user\\rlogin\n",
		},
		{
			name:  "empty event",
			event: "",
			want:  "",
		},
		{
			name:  "event with dot notation",
			event: "user.created",
			want:  "event: user.created\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := &bytes.Buffer{}
			encoder.writeEvent(tt.event, buf)
			got := buf.String()
			if got != tt.want {
				t.Errorf("writeEvent() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestEncoder_writeData tests Data field encoding
func TestEncoder_writeData(t *testing.T) {
	encoder := NewEncoder()

	tests := []struct {
		name string
		data []byte
		want string
	}{
		{
			name: "simple data",
			data: []byte("hello"),
			want: "data: hello\n",
		},
		{
			name: "empty data",
			data: []byte{},
			want: "",
		},
		{
			name: "nil data",
			data: nil,
			want: "",
		},
		{
			name: "multiline data",
			data: []byte("line1\nline2\nline3"),
			want: "data: line1\ndata: line2\ndata: line3\n",
		},
		{
			name: "data with carriage return",
			data: []byte("hello\rworld"),
			want: "data: hello\\rworld\n",
		},
		{
			name: "data with CRLF",
			data: []byte("line1\r\nline2"),
			want: "data: line1\\r\ndata: line2\n",
		},
		{
			name: "data ending with newline",
			data: []byte("hello\n"),
			want: "data: hello\ndata: \n",
		},
		{
			name: "data with only newlines",
			data: []byte("\n\n\n"),
			want: "data: \ndata: \ndata: \ndata: \n",
		},
		{
			name: "JSON data",
			data: []byte(`{"name":"alice","age":30}`),
			want: "data: {\"name\":\"alice\",\"age\":30}\n",
		},
		{
			name: "multiline JSON",
			data: []byte("{\n  \"name\": \"alice\"\n}"),
			want: "data: {\ndata:   \"name\": \"alice\"\ndata: }\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := &bytes.Buffer{}
			encoder.writeData(tt.data, buf)
			got := buf.String()
			if got != tt.want {
				t.Errorf("writeData() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestEncoder_writeRetry tests Retry field encoding
func TestEncoder_writeRetry(t *testing.T) {
	encoder := NewEncoder()

	tests := []struct {
		name  string
		retry int
		want  string
	}{
		{
			name:  "positive retry",
			retry: 3000,
			want:  "retry: 3000\n",
		},
		{
			name:  "zero retry",
			retry: 0,
			want:  "",
		},
		{
			name:  "negative retry",
			retry: -1000,
			want:  "", // Negative values are treated as zero
		},
		{
			name:  "large retry value",
			retry: 999999999,
			want:  "retry: 999999999\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := &bytes.Buffer{}
			encoder.writeRetry(tt.retry, buf)
			got := buf.String()
			if got != tt.want {
				t.Errorf("writeRetry() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestEncoder_encodeToBytes tests internal encoding logic
func TestEncoder_encodeToBytes(t *testing.T) {
	encoder := NewEncoder()

	tests := []struct {
		name    string
		message *Message
		want    string
	}{
		{
			name: "complete message",
			message: &Message{
				ID:    "123",
				Event: "update",
				Data:  []byte("hello"),
				Retry: 3000,
			},
			want: "id: 123\nevent: update\ndata: hello\nretry: 3000\n\n",
		},
		{
			name: "message with data only",
			message: &Message{
				Data: []byte("hello world"),
			},
			want: "data: hello world\n\n",
		},
		{
			name: "message with ID and Event",
			message: &Message{
				ID:    "456",
				Event: "notification",
			},
			want: "id: 456\nevent: notification\n\n",
		},
		{
			name: "message with multiline data",
			message: &Message{
				Data: []byte("line1\nline2\nline3"),
			},
			want: "data: line1\ndata: line2\ndata: line3\n\n",
		},
		{
			name: "message with escaped characters",
			message: &Message{
				ID:    "id\nwith\nnewlines",
				Event: "event\rwith\rreturns",
				Data:  []byte("data\r\nwith\r\nboth"),
			},
			want: "id: id\\nwith\\nnewlines\nevent: event\\rwith\\rreturns\ndata: data\\r\ndata: with\\r\ndata: both\n\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := encoder.encodeToBytes(tt.message)
			if string(got) != tt.want {
				t.Errorf("encodeToBytes() = %q, want %q", string(got), tt.want)
			}
		})
	}
}

// TestEncoder_Encode tests the public encoding API
func TestEncoder_Encode(t *testing.T) {
	encoder := NewEncoder()

	tests := []struct {
		name    string
		message *Message
		want    string
		wantErr error
	}{
		{
			name: "valid complete message",
			message: &Message{
				ID:    "123",
				Event: "update",
				Data:  []byte("hello"),
				Retry: 3000,
			},
			want:    "id: 123\nevent: update\ndata: hello\nretry: 3000\n\n",
			wantErr: nil,
		},
		{
			name: "valid message with data only",
			message: &Message{
				Data: []byte("test"),
			},
			want:    "data: test\n\n",
			wantErr: nil,
		},
		{
			name: "valid message with empty event (defaults to 'message')",
			message: &Message{
				Event: "",
				Data:  []byte("test"),
			},
			want:    "data: test\n\n",
			wantErr: nil,
		},
		{
			name:    "invalid: empty message",
			message: &Message{},
			want:    "",
			wantErr: ErrMessageNoContent,
		},
		{
			name: "invalid: message with only retry",
			message: &Message{
				Retry: 3000,
			},
			want:    "",
			wantErr: ErrMessageNoContent,
		},
		{
			name: "invalid: bad event name starting with dot",
			message: &Message{
				Event: ".invalid",
				Data:  []byte("test"),
			},
			want:    "",
			wantErr: ErrMessageInvalidEventName,
		},
		{
			name: "invalid: bad event name with double dots",
			message: &Message{
				Event: "user..created",
				Data:  []byte("test"),
			},
			want:    "",
			wantErr: ErrMessageInvalidEventName,
		},
		{
			name: "invalid: bad event name with special char",
			message: &Message{
				Event: "event!",
				Data:  []byte("test"),
			},
			want:    "",
			wantErr: ErrMessageInvalidEventName,
		},
		{
			name: "valid: event with dot notation",
			message: &Message{
				Event: "user.created",
				Data:  []byte("test"),
			},
			want:    "event: user.created\ndata: test\n\n",
			wantErr: nil,
		},
		{
			name: "valid: multiline JSON data",
			message: &Message{
				ID:    "789",
				Event: "data.update",
				Data:  []byte("{\n  \"key\": \"value\"\n}"),
			},
			want:    "id: 789\nevent: data.update\ndata: {\ndata:   \"key\": \"value\"\ndata: }\n\n",
			wantErr: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := encoder.Encode(tt.message)

			// Check error
			if tt.wantErr != nil {
				if err == nil {
					t.Errorf("Encode() error = nil, wantErr %v", tt.wantErr)
					return
				}
				if !strings.Contains(err.Error(), tt.wantErr.Error()) {
					t.Errorf("Encode() error = %v, wantErr %v", err, tt.wantErr)
				}
				return
			}

			if err != nil {
				t.Errorf("Encode() unexpected error = %v", err)
				return
			}

			// Check output
			if string(got) != tt.want {
				t.Errorf("Encode() = %q, want %q", string(got), tt.want)
			}

			// Verify message ends with blank line
			if !bytes.HasSuffix(got, byteLFLF) {
				t.Error("Encode() output does not end with blank line (\\n\\n)")
			}
		})
	}
}

// TestEncoder_ConcurrentEncode tests concurrent encoding safety
func TestEncoder_ConcurrentEncode(t *testing.T) {
	encoder := NewEncoder()
	message := &Message{
		ID:    "123",
		Event: "test",
		Data:  []byte("concurrent test"),
		Retry: 1000,
	}

	const numGoroutines = 100
	done := make(chan bool, numGoroutines)
	errors := make(chan error, numGoroutines)

	// Launch multiple goroutines encoding the same message
	for i := 0; i < numGoroutines; i++ {
		go func() {
			_, err := encoder.Encode(message)
			if err != nil {
				errors <- err
			}
			done <- true
		}()
	}

	// Wait for all goroutines to complete
	for i := 0; i < numGoroutines; i++ {
		<-done
	}

	// Check for any errors
	close(errors)
	for err := range errors {
		t.Errorf("Concurrent encode error: %v", err)
	}
}

// TestEncoder_MessageTermination tests that all messages end with blank line
func TestEncoder_MessageTermination(t *testing.T) {
	encoder := NewEncoder()

	messages := []*Message{
		{Data: []byte("test")},
		{ID: "123", Data: []byte("test")},
		{Event: "update", Data: []byte("test")},
		{ID: "456", Event: "notification", Data: []byte("test"), Retry: 3000},
	}

	for i, msg := range messages {
		t.Run(string(rune('A'+i)), func(t *testing.T) {
			encoded, err := encoder.Encode(msg)
			if err != nil {
				t.Fatalf("Encode() error = %v", err)
			}

			if !bytes.HasSuffix(encoded, byteLFLF) {
				t.Error("Message does not end with blank line (\\n\\n)")
			}
		})
	}
}

// BenchmarkEncoder_Encode benchmarks the encoding process
func BenchmarkEncoder_Encode(b *testing.B) {
	encoder := NewEncoder()

	benchmarks := []struct {
		name    string
		message *Message
	}{
		{
			name: "simple message",
			message: &Message{
				Data: []byte("hello world"),
			},
		},
		{
			name: "complete message",
			message: &Message{
				ID:    "123",
				Event: "update",
				Data:  []byte("hello world"),
				Retry: 3000,
			},
		},
		{
			name: "multiline data",
			message: &Message{
				Data: []byte("line1\nline2\nline3\nline4\nline5"),
			},
		},
		{
			name: "large data",
			message: &Message{
				Data: []byte(strings.Repeat("x", 1024)),
			},
		},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_, _ = encoder.Encode(bm.message)
			}
		})
	}
}

// BenchmarkEncoder_writeData benchmarks data writing specifically
func BenchmarkEncoder_writeData(b *testing.B) {
	encoder := NewEncoder()

	benchmarks := []struct {
		name string
		data []byte
	}{
		{
			name: "single line",
			data: []byte("hello world"),
		},
		{
			name: "multiline",
			data: []byte("line1\nline2\nline3\nline4\nline5"),
		},
		{
			name: "large single line",
			data: []byte(strings.Repeat("x", 1024)),
		},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			buf := &bytes.Buffer{}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				buf.Reset()
				encoder.writeData(bm.data, buf)
			}
		})
	}
}

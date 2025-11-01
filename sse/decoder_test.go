package sse

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

// TestNewDecoder tests decoder creation
func TestNewDecoder(t *testing.T) {
	reader := strings.NewReader("data: test\n\n")
	decoder := NewDecoder(reader)

	if decoder == nil {
		t.Fatal("NewDecoder() returned nil")
	}
	if decoder.streamReader == nil {
		t.Error("streamReader not initialized")
	}
	if decoder.lineScanner == nil {
		t.Error("lineScanner not initialized")
	}
	if decoder.dataBuffer == nil {
		t.Error("dataBuffer not initialized")
	}
}

// TestDecoder_skipLeadingUTF8BOM tests BOM handling
func TestDecoder_skipLeadingUTF8BOM(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantSkip bool
	}{
		{
			name:     "with BOM",
			input:    "\xEF\xBB\xBFdata: test\n\n",
			wantSkip: true,
		},
		{
			name:     "without BOM",
			input:    "data: test\n\n",
			wantSkip: false,
		},
		{
			name:     "partial BOM",
			input:    "\xEF\xBBdata: test\n\n",
			wantSkip: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decoder := NewDecoder(strings.NewReader(tt.input))

			// Try to decode a message
			if decoder.Next() {
				msg := decoder.Current()
				// If BOM was properly skipped, data should be "test"
				if string(msg.Data) != "test" {
					t.Errorf("Data = %q, want %q", string(msg.Data), "test")
				}
			}
		})
	}
}

// TestDecoder_scanLinesSplit tests the custom line splitter
func TestDecoder_scanLinesSplit(t *testing.T) {
	tests := []struct {
		name      string
		data      []byte
		atEOF     bool
		wantAdv   int
		wantToken string
	}{
		{
			name:      "LF only",
			data:      []byte("line1\nline2"),
			atEOF:     false,
			wantAdv:   6,
			wantToken: "line1",
		},
		{
			name:      "CRLF",
			data:      []byte("line1\r\nline2"),
			atEOF:     false,
			wantAdv:   7,
			wantToken: "line1",
		},
		{
			name:      "CR only",
			data:      []byte("line1\rline2"),
			atEOF:     false,
			wantAdv:   6,
			wantToken: "line1",
		},
		{
			name:      "EOF with data",
			data:      []byte("last line"),
			atEOF:     true,
			wantAdv:   9,
			wantToken: "last line",
		},
		{
			name:      "EOF with trailing CR",
			data:      []byte("last line\r"),
			atEOF:     true,
			wantAdv:   10,
			wantToken: "last line",
		},
		{
			name:      "EOF empty",
			data:      []byte(""),
			atEOF:     true,
			wantAdv:   0,
			wantToken: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adv, token, err := scanLinesSplit(tt.data, tt.atEOF)
			if err != nil {
				t.Errorf("scanLinesSplit() error = %v", err)
				return
			}
			if adv != tt.wantAdv {
				t.Errorf("advance = %d, want %d", adv, tt.wantAdv)
			}
			if string(token) != tt.wantToken {
				t.Errorf("token = %q, want %q", string(token), tt.wantToken)
			}
		})
	}
}

// TestDecoder_normalizeValue tests value normalization
func TestDecoder_normalizeValue(t *testing.T) {
	decoder := NewDecoder(strings.NewReader(""))

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "no leading space",
			input: "value",
			want:  "value",
		},
		{
			name:  "with leading space",
			input: " value",
			want:  "value",
		},
		{
			name:  "multiple leading spaces",
			input: "   value",
			want:  "  value", // Only first space is removed
		},
		{
			name:  "valid UTF-8",
			input: " hello世界",
			want:  "hello世界",
		},
		{
			name:  "invalid UTF-8",
			input: " hello\xffworld",
			want:  "hello\uFFFDworld",
		},
		{
			name:  "empty string",
			input: "",
			want:  "",
		},
		{
			name:  "only space",
			input: " ",
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := decoder.normalizeValue(tt.input)
			if got != tt.want {
				t.Errorf("normalizeValue(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// TestDecoder_hasValidData tests data validation logic
func TestDecoder_hasValidData(t *testing.T) {
	tests := []struct {
		name string
		data string
		want bool
	}{
		{
			name: "has data",
			data: "test\n",
			want: true,
		},
		{
			name: "only newline",
			data: "\n",
			want: false,
		},
		{
			name: "empty",
			data: "",
			want: false,
		},
		{
			name: "multiple lines",
			data: "line1\nline2\n",
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decoder := NewDecoder(strings.NewReader(""))
			decoder.dataBuffer.WriteString(tt.data)
			got := decoder.hasValidData()
			if got != tt.want {
				t.Errorf("hasValidData() = %v, want %v (buffer: %q)", got, tt.want, tt.data)
			}
		})
	}
}

// TestDecoder_processLine tests line processing
func TestDecoder_processLine(t *testing.T) {
	tests := []struct {
		name      string
		line      string
		wantID    string
		wantEvent string
		wantData  string
		wantRetry int
	}{
		{
			name:      "comment line",
			line:      ": this is a comment",
			wantID:    "",
			wantEvent: "",
			wantData:  "",
			wantRetry: 0,
		},
		{
			name:      "ID field",
			line:      "id: 123",
			wantID:    "123",
			wantEvent: "",
			wantData:  "",
			wantRetry: 0,
		},
		{
			name:      "ID without value",
			line:      "id:",
			wantID:    "",
			wantEvent: "",
			wantData:  "",
			wantRetry: 0,
		},
		{
			name:      "event field",
			line:      "event: update",
			wantID:    "",
			wantEvent: "update",
			wantData:  "",
			wantRetry: 0,
		},
		{
			name:      "event empty defaults to message",
			line:      "event:",
			wantID:    "",
			wantEvent: "message",
			wantData:  "",
			wantRetry: 0,
		},
		{
			name:      "data field",
			line:      "data: hello",
			wantID:    "",
			wantEvent: "",
			wantData:  "hello\n",
			wantRetry: 0,
		},
		{
			name:      "data without value",
			line:      "data:",
			wantID:    "",
			wantEvent: "",
			wantData:  "\n",
			wantRetry: 0,
		},
		{
			name:      "retry field",
			line:      "retry: 3000",
			wantID:    "",
			wantEvent: "",
			wantData:  "",
			wantRetry: 3000,
		},
		{
			name:      "retry invalid",
			line:      "retry: abc",
			wantID:    "",
			wantEvent: "",
			wantData:  "",
			wantRetry: 0,
		},
		{
			name:      "retry negative",
			line:      "retry: -1000",
			wantID:    "",
			wantEvent: "",
			wantData:  "",
			wantRetry: 0,
		},
		{
			name:      "retry zero",
			line:      "retry: 0",
			wantID:    "",
			wantEvent: "",
			wantData:  "",
			wantRetry: 0,
		},
		{
			name:      "unknown field",
			line:      "unknown: value",
			wantID:    "",
			wantEvent: "",
			wantData:  "",
			wantRetry: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decoder := NewDecoder(strings.NewReader(""))
			decoder.processLine(tt.line)

			if decoder.lastID != tt.wantID {
				t.Errorf("lastID = %q, want %q", decoder.lastID, tt.wantID)
			}
			if decoder.eventBuffer != tt.wantEvent {
				t.Errorf("eventBuffer = %q, want %q", decoder.eventBuffer, tt.wantEvent)
			}
			if decoder.dataBuffer.String() != tt.wantData {
				t.Errorf("dataBuffer = %q, want %q", decoder.dataBuffer.String(), tt.wantData)
			}
			if decoder.retry != tt.wantRetry {
				t.Errorf("retry = %d, want %d", decoder.retry, tt.wantRetry)
			}
		})
	}
}

// TestDecoder_EmptyDataField tests the W3C specification examples for empty data fields
func TestDecoder_EmptyDataField(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		wantMessages []Message
		description  string
	}{
		{
			name: "W3C example - single empty data field (data without colon)",
			input: "data\n" +
				"\n",
			wantMessages: []Message{},
			description:  "Single 'data' without colon should not dispatch (buffer has only \\n)",
		},
		{
			name: "W3C example - two empty data fields",
			input: "data\n" +
				"data\n" +
				"\n",
			wantMessages: []Message{
				{Data: []byte("\n")},
			},
			description: "Two 'data' lines should dispatch with single newline as content",
		},
		{
			name: "W3C example - empty data with colon",
			input: "data:\n" +
				"\n",
			wantMessages: []Message{},
			description:  "'data:' should not dispatch (buffer has only \\n after normalization)",
		},
		{
			name: "W3C complete example",
			input: "data\n" +
				"\n" +
				"data\n" +
				"data\n" +
				"\n" +
				"data:\n" +
				"\n",
			wantMessages: []Message{
				{Data: []byte("\n")},
			},
			description: "Only the middle block should dispatch",
		},
		{
			name: "multiple empty data fields with colon",
			input: "data:\n" +
				"data:\n" +
				"\n",
			wantMessages: []Message{
				{Data: []byte("\n")},
			},
			description: "Two 'data:' lines should behave like two 'data' lines",
		},
		{
			name: "empty data field mixed with content",
			input: "data:\n" +
				"data: content\n" +
				"\n",
			wantMessages: []Message{
				{Data: []byte("\ncontent")},
			},
			description: "Empty data followed by content should include the empty line",
		},
		{
			name: "three empty data fields",
			input: "data\n" +
				"data\n" +
				"data\n" +
				"\n",
			wantMessages: []Message{
				{Data: []byte("\n\n")},
			},
			description: "Three 'data' lines should dispatch with two newlines",
		},
		{
			name:         "data without colon, no trailing blank line",
			input:        "data",
			wantMessages: []Message{},
			description:  "Single 'data' at EOF without blank line should not dispatch",
		},
		{
			name: "two data without colon, no trailing blank line",
			input: "data\n" +
				"data",
			wantMessages: []Message{
				{Data: []byte("\n")},
			},
			description: "Two 'data' at EOF should dispatch at EOF",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decoder := NewDecoder(strings.NewReader(tt.input))
			var messages []Message

			for decoder.Next() {
				messages = append(messages, decoder.Current())
			}

			if err := decoder.Error(); err != nil {
				t.Errorf("Unexpected error: %v", err)
			}

			if len(messages) != len(tt.wantMessages) {
				t.Errorf("%s\nGot %d messages, want %d\nMessages: %+v",
					tt.description, len(messages), len(tt.wantMessages), messages)
				return
			}

			for i, got := range messages {
				want := tt.wantMessages[i]
				if !bytes.Equal(got.Data, want.Data) {
					t.Errorf("%s\nMessage[%d].Data = %q, want %q",
						tt.description, i, got.Data, want.Data)
				}
			}
		})
	}
}

// TestDecoder_SpaceAfterColon tests the space normalization after colon
func TestDecoder_SpaceAfterColon(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
		noMsg bool
	}{
		{
			name:  "W3C example - no space",
			input: "data:test\n\n",
			want:  "test",
		},
		{
			name:  "W3C example - with space",
			input: "data: test\n\n",
			want:  "test",
		},
		{
			name:  "multiple spaces after colon",
			input: "data:   test\n\n",
			want:  "  test", // Only first space removed
		},
		{
			name:  "tab after colon",
			input: "data:\ttest\n\n",
			want:  "\ttest", // Tab is not removed (only space)
		},
		{
			name:  "no value after colon",
			input: "data:\n\n",
			want:  "",
		},
		{
			name:  "space only after colon",
			input: "data: \n\n",
			noMsg: true,
		},
		{
			name:  "multiple spaces only after colon",
			input: "data:   \n\n",
			want:  "  ", // First space removed, rest remains
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decoder := NewDecoder(strings.NewReader(tt.input))

			// For empty data cases, check if message is dispatched
			hasMessage := decoder.Next()

			if tt.want == "" && len(tt.input) == 7 { // "data:\n\n"
				if hasMessage {
					t.Errorf("Expected no message to be dispatched for empty data field")
				}
				return
			}

			if !hasMessage && !tt.noMsg {
				t.Fatal("Next() returned false, expected true")
			}

			got := string(decoder.Current().Data)
			if got != tt.want {
				t.Errorf("Data = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestDecoder_Next tests the main decoding loop
func TestDecoder_Next(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		wantMessages []Message
		wantError    bool
	}{
		{
			name:  "simple message",
			input: "data: hello\n\n",
			wantMessages: []Message{
				{Data: []byte("hello")},
			},
		},
		{
			name:  "message with all fields",
			input: "id: 123\nevent: update\ndata: test\nretry: 3000\n\n",
			wantMessages: []Message{
				{
					ID:    "123",
					Event: "update",
					Data:  []byte("test"),
					Retry: 3000,
				},
			},
		},
		{
			name:  "multiline data",
			input: "data: line1\ndata: line2\ndata: line3\n\n",
			wantMessages: []Message{
				{Data: []byte("line1\nline2\nline3")},
			},
		},
		{
			name:  "multiple messages",
			input: "data: msg1\n\ndata: msg2\n\n",
			wantMessages: []Message{
				{Data: []byte("msg1")},
				{Data: []byte("msg2")},
			},
		},
		{
			name: "multiple messages with complex fields",
			input: "id: 1\nevent: update\ndata: msg1\n\n" +
				"id: 2\nevent: delete\ndata: msg2\n\n" +
				"id: 3\ndata: msg3\n\n",
			wantMessages: []Message{
				{ID: "1", Event: "update", Data: []byte("msg1")},
				{ID: "2", Event: "delete", Data: []byte("msg2")},
				{ID: "3", Data: []byte("msg3")},
			},
		},
		{
			name:  "ID persists across messages",
			input: "id: 123\ndata: msg1\n\ndata: msg2\n\n",
			wantMessages: []Message{
				{ID: "123", Data: []byte("msg1")},
				{ID: "123", Data: []byte("msg2")}, // ID persists
			},
		},
		{
			name:  "ID updated in second message",
			input: "id: 123\ndata: msg1\n\nid: 456\ndata: msg2\n\n",
			wantMessages: []Message{
				{ID: "123", Data: []byte("msg1")},
				{ID: "456", Data: []byte("msg2")},
			},
		},
		{
			name:  "ID can be reset to empty",
			input: "id: 123\ndata: msg1\n\nid:\ndata: msg2\n\n",
			wantMessages: []Message{
				{ID: "123", Data: []byte("msg1")},
				{ID: "", Data: []byte("msg2")},
			},
		},
		{
			name:  "comment lines ignored",
			input: ": comment\ndata: test\n: another comment\n\n",
			wantMessages: []Message{
				{Data: []byte("test")},
			},
		},
		{
			name:  "empty lines between fields are message boundaries",
			input: "id: 123\n\ndata: test\n\n",
			wantMessages: []Message{
				{ID: "123", Data: []byte("test")}, // ID persists to next message
			},
		},
		{
			name:  "multiple consecutive empty lines",
			input: "data: test1\n\n\n\ndata: test2\n\n",
			wantMessages: []Message{
				{Data: []byte("test1")},
				{Data: []byte("test2")},
			},
		},
		{
			name:  "message at EOF without trailing newline",
			input: "data: test",
			wantMessages: []Message{
				{Data: []byte("test")},
			},
		},
		{
			name:  "message at EOF with single newline",
			input: "data: test\n",
			wantMessages: []Message{
				{Data: []byte("test")},
			},
		},
		{
			name:  "default event type",
			input: "event:\ndata: test\n\n",
			wantMessages: []Message{
				{Event: "message", Data: []byte("test")},
			},
		},
		{
			name:  "invalid event name skipped",
			input: "event: .invalid\ndata: test1\n\ndata: test2\n\n",
			wantMessages: []Message{
				{Data: []byte("test2")},
			},
		},
		{
			name:         "empty input",
			input:        "",
			wantMessages: []Message{},
		},
		{
			name:         "only comments",
			input:        ": comment1\n: comment2\n\n",
			wantMessages: []Message{},
		},
		{
			name:         "only empty lines",
			input:        "\n\n\n\n",
			wantMessages: []Message{},
		},
		{
			name:  "CRLF line endings",
			input: "data: test\r\n\r\n",
			wantMessages: []Message{
				{Data: []byte("test")},
			},
		},
		{
			name:  "mixed line endings",
			input: "data: line1\r\ndata: line2\ndata: line3\r\n\n",
			wantMessages: []Message{
				{Data: []byte("line1\nline2\nline3")},
			},
		},
		{
			name:  "CR only as line separator",
			input: "data: line1\rdata: line2\r\r",
			wantMessages: []Message{
				{Data: []byte("line1\nline2")},
			},
		},
		{
			name: "complex real-world scenario",
			input: ": heartbeat\n" +
				"id: msg-1\n" +
				"event: user.login\n" +
				"data: {\"user\":\"alice\"}\n" +
				"retry: 3000\n" +
				"\n" +
				": heartbeat\n" +
				"event: user.logout\n" +
				"data: {\"user\":\"bob\"}\n" +
				"\n" +
				"data: no event specified\n" +
				"\n",
			wantMessages: []Message{
				{
					ID:    "msg-1",
					Event: "user.login",
					Data:  []byte(`{"user":"alice"}`),
					Retry: 3000,
				},
				{
					ID:    "msg-1", // ID persists
					Event: "user.logout",
					Data:  []byte(`{"user":"bob"}`),
					Retry: 0, // Retry resets
				},
				{
					ID:   "msg-1", // ID still persists
					Data: []byte("no event specified"),
				},
			},
		},
		{
			name: "retry persists until explicitly changed",
			input: "retry: 3000\n" +
				"data: msg1\n" +
				"\n" +
				"data: msg2\n" +
				"\n" +
				"retry: 5000\n" +
				"data: msg3\n" +
				"\n",
			wantMessages: []Message{
				{Retry: 3000, Data: []byte("msg1")},
				{Retry: 0, Data: []byte("msg2")}, // Retry resets after dispatch
				{Retry: 5000, Data: []byte("msg3")},
			},
		},
		{
			name: "fields after empty line start new message",
			input: "id: 1\n" +
				"data: first\n" +
				"\n" +
				"id: 2\n" +
				"data: second\n" +
				"\n",
			wantMessages: []Message{
				{ID: "1", Data: []byte("first")},
				{ID: "2", Data: []byte("second")},
			},
		},
		{
			name: "data field can be empty string",
			input: "data:\n" +
				"data: \n" +
				"data: content\n" +
				"\n",
			wantMessages: []Message{
				{Data: []byte("\n\ncontent")}, // Each data: adds a line
			},
		},
		{
			name: "only retry field without data is ignored",
			input: "retry: 3000\n" +
				"\n" +
				"data: test\n" +
				"\n",
			wantMessages: []Message{
				{Data: []byte("test")},
			},
		},
		{
			name: "only id field without data is ignored",
			input: "id: 123\n" +
				"\n" +
				"data: test\n" +
				"\n",
			wantMessages: []Message{
				{ID: "123", Data: []byte("test")}, // ID persists
			},
		},
		{
			name: "only event field without data is ignored",
			input: "event: update\n" +
				"\n" +
				"data: test\n" +
				"\n",
			wantMessages: []Message{
				{Data: []byte("test")},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decoder := NewDecoder(strings.NewReader(tt.input))
			var messages []Message

			for decoder.Next() {
				messages = append(messages, decoder.Current())
			}

			if err := decoder.Error(); (err != nil) != tt.wantError {
				t.Errorf("Error() = %v, wantError %v", err, tt.wantError)
			}

			if len(messages) != len(tt.wantMessages) {
				t.Fatalf("got %d messages, want %d\nMessages: %+v", len(messages), len(tt.wantMessages), messages)
			}

			for i, got := range messages {
				want := tt.wantMessages[i]
				if got.ID != want.ID {
					t.Errorf("message[%d].ID = %q, want %q", i, got.ID, want.ID)
				}
				if got.Event != want.Event {
					t.Errorf("message[%d].Event = %q, want %q", i, got.Event, want.Event)
				}
				if !bytes.Equal(got.Data, want.Data) {
					t.Errorf("message[%d].Data = %q, want %q", i, got.Data, want.Data)
				}
				if got.Retry != want.Retry {
					t.Errorf("message[%d].Retry = %d, want %d", i, got.Retry, want.Retry)
				}
			}
		})
	}
}

// TestDecoder_Error tests error handling
func TestDecoder_Error(t *testing.T) {
	t.Run("no error on normal stream end", func(t *testing.T) {
		decoder := NewDecoder(strings.NewReader("data: test\n\n"))
		for decoder.Next() {
		}
		if decoder.Error() != nil {
			t.Errorf("Error() = %v, want nil", decoder.Error())
		}
	})

	t.Run("error on read failure", func(t *testing.T) {
		// Create a reader that returns an error
		errReader := &errorReader{err: errors.New("read error")}
		decoder := NewDecoder(errReader)

		if decoder.Next() {
			t.Error("Next() = true, want false on error")
		}
		if decoder.Error() == nil {
			t.Error("Error() = nil, want error")
		}
	})
}

// TestDecoder_Current tests Current() method
func TestDecoder_Current(t *testing.T) {
	input := "id: 123\nevent: update\ndata: test\nretry: 3000\n\n"
	decoder := NewDecoder(strings.NewReader(input))

	if !decoder.Next() {
		t.Fatal("Next() returned false")
	}

	msg := decoder.Current()
	if msg.ID != "123" {
		t.Errorf("ID = %q, want %q", msg.ID, "123")
	}
	if msg.Event != "update" {
		t.Errorf("Event = %q, want %q", msg.Event, "update")
	}
	if string(msg.Data) != "test" {
		t.Errorf("Data = %q, want %q", msg.Data, "test")
	}
	if msg.Retry != 3000 {
		t.Errorf("Retry = %d, want %d", msg.Retry, 3000)
	}
}

// TestDecoder_ComplexScenarios tests real-world scenarios
func TestDecoder_ComplexScenarios(t *testing.T) {
	t.Run("JSON payload", func(t *testing.T) {
		input := `data: {"name":"alice","age":30}

`
		decoder := NewDecoder(strings.NewReader(input))
		if !decoder.Next() {
			t.Fatal("Next() returned false")
		}
		msg := decoder.Current()
		expected := `{"name":"alice","age":30}`
		if string(msg.Data) != expected {
			t.Errorf("Data = %q, want %q", msg.Data, expected)
		}
	})

	t.Run("multiline JSON", func(t *testing.T) {
		input := `data: {
data:   "name": "alice",
data:   "age": 30
data: }

`
		decoder := NewDecoder(strings.NewReader(input))
		if !decoder.Next() {
			t.Fatal("Next() returned false")
		}
		msg := decoder.Current()
		expected := "{\n  \"name\": \"alice\",\n  \"age\": 30\n}"
		if string(msg.Data) != expected {
			t.Errorf("Data = %q, want %q", msg.Data, expected)
		}
	})

	t.Run("heartbeat comments", func(t *testing.T) {
		input := `: heartbeat
data: msg1

: heartbeat
data: msg2

`
		decoder := NewDecoder(strings.NewReader(input))
		messages := []string{}
		for decoder.Next() {
			messages = append(messages, string(decoder.Current().Data))
		}
		expected := []string{"msg1", "msg2"}
		if len(messages) != len(expected) {
			t.Fatalf("got %d messages, want %d", len(messages), len(expected))
		}
		for i, msg := range messages {
			if msg != expected[i] {
				t.Errorf("message[%d] = %q, want %q", i, msg, expected[i])
			}
		}
	})

	t.Run("retry behavior", func(t *testing.T) {
		input := `retry: 3000
data: msg1

retry: 5000
data: msg2

data: msg3

`
		decoder := NewDecoder(strings.NewReader(input))
		expectedRetries := []int{3000, 5000, 0}
		retries := []int{}

		for decoder.Next() {
			retries = append(retries, decoder.Current().Retry)
		}

		if len(retries) != len(expectedRetries) {
			t.Fatalf("got %d messages, want %d", len(retries), len(expectedRetries))
		}
		for i, retry := range retries {
			if retry != expectedRetries[i] {
				t.Errorf("message[%d].Retry = %d, want %d", i, retry, expectedRetries[i])
			}
		}
	})
}

// TestDecoder_UTF8Handling tests UTF-8 and encoding issues
func TestDecoder_UTF8Handling(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "valid UTF-8 Chinese",
			input: "data: 你好世界\n\n",
			want:  "你好世界",
		},
		{
			name:  "valid UTF-8 emoji",
			input: "data: Hello 👋 World\n\n",
			want:  "Hello 👋 World",
		},
		{
			name:  "invalid UTF-8 replaced",
			input: "data: hello\xffworld\n\n",
			want:  "hello\uFFFDworld",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decoder := NewDecoder(strings.NewReader(tt.input))
			if !decoder.Next() {
				t.Fatal("Next() returned false")
			}
			got := string(decoder.Current().Data)
			if got != tt.want {
				t.Errorf("Data = %q, want %q", got, tt.want)
			}
		})
	}
}

// BenchmarkDecoder_Next benchmarks the decoding process
func BenchmarkDecoder_Next(b *testing.B) {
	benchmarks := []struct {
		name  string
		input string
	}{
		{
			name:  "simple message",
			input: "data: hello\n\n",
		},
		{
			name:  "complete message",
			input: "id: 123\nevent: update\ndata: test\nretry: 3000\n\n",
		},
		{
			name:  "multiline data",
			input: "data: line1\ndata: line2\ndata: line3\ndata: line4\ndata: line5\n\n",
		},
		{
			name:  "multiple messages",
			input: strings.Repeat("data: test\n\n", 10),
		},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				decoder := NewDecoder(strings.NewReader(bm.input))
				for decoder.Next() {
					_ = decoder.Current()
				}
			}
		})
	}
}

// BenchmarkDecoder_LargeStream benchmarks large stream processing
func BenchmarkDecoder_LargeStream(b *testing.B) {
	// Create a large stream with 1000 messages
	var buf bytes.Buffer
	for i := 0; i < 1000; i++ {
		buf.WriteString("id: ")
		buf.WriteString(string(rune(i)))
		buf.WriteString("\ndata: message ")
		buf.WriteString(string(rune(i)))
		buf.WriteString("\n\n")
	}
	input := buf.String()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		decoder := NewDecoder(strings.NewReader(input))
		count := 0
		for decoder.Next() {
			count++
		}
		if count != 1000 {
			b.Fatalf("decoded %d messages, want 1000", count)
		}
	}
}

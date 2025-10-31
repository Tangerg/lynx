package sse

import (
	"testing"
)

// TestIsValidDOMEventName tests the DOM event name validation logic
func TestIsValidDOMEventName(t *testing.T) {
	tests := []struct {
		name      string
		eventName string
		want      bool
	}{
		// Valid cases
		{
			name:      "simple lowercase",
			eventName: "update",
			want:      true,
		},
		{
			name:      "camelCase",
			eventName: "userCreated",
			want:      true,
		},
		{
			name:      "with hyphen",
			eventName: "user-login",
			want:      true,
		},
		{
			name:      "with underscore",
			eventName: "user_logout",
			want:      true,
		},
		{
			name:      "with single dot",
			eventName: "user.created",
			want:      true,
		},
		{
			name:      "with multiple dots",
			eventName: "system.user.created",
			want:      true,
		},
		{
			name:      "with digits",
			eventName: "event123",
			want:      true,
		},
		{
			name:      "mixed valid characters",
			eventName: "user_login.success-2024",
			want:      true,
		},
		{
			name:      "uppercase start",
			eventName: "UserLogin",
			want:      true,
		},

		// Invalid cases
		{
			name:      "empty string",
			eventName: "",
			want:      false,
		},
		{
			name:      "starts with dot",
			eventName: ".update",
			want:      false,
		},
		{
			name:      "ends with dot",
			eventName: "update.",
			want:      false,
		},
		{
			name:      "double dots",
			eventName: "user..profile",
			want:      false,
		},
		{
			name:      "starts with digit",
			eventName: "123event",
			want:      false,
		},
		{
			name:      "starts with underscore",
			eventName: "_event",
			want:      false,
		},
		{
			name:      "starts with hyphen",
			eventName: "-event",
			want:      false,
		},
		{
			name:      "contains space",
			eventName: "user login",
			want:      false,
		},
		{
			name:      "contains tab",
			eventName: "user\tlogin",
			want:      false,
		},
		{
			name:      "contains newline",
			eventName: "user\nlogin",
			want:      false,
		},
		{
			name:      "contains special character !",
			eventName: "alert!",
			want:      false,
		},
		{
			name:      "contains special character @",
			eventName: "user@login",
			want:      false,
		},
		{
			name:      "contains special character #",
			eventName: "event#1",
			want:      false,
		},
		{
			name:      "contains parentheses",
			eventName: "event(1)",
			want:      false,
		},
		{
			name:      "contains brackets",
			eventName: "event[1]",
			want:      false,
		},
		{
			name:      "contains braces",
			eventName: "event{1}",
			want:      false,
		},
		{
			name:      "contains forward slash",
			eventName: "user/login",
			want:      false,
		},
		{
			name:      "contains backslash",
			eventName: "user\\login",
			want:      false,
		},
		{
			name:      "only dots",
			eventName: "...",
			want:      false,
		},
		{
			name:      "unicode space",
			eventName: "user\u00A0login",
			want:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isValidDOMEventName(tt.eventName)
			if got != tt.want {
				t.Errorf("isValidDOMEventName(%q) = %v, want %v", tt.eventName, got, tt.want)
			}
		})
	}
}

// TestIsValidSSEEventName tests the SSE event name validation
func TestIsValidSSEEventName(t *testing.T) {
	tests := []struct {
		name      string
		eventName string
		want      bool
	}{
		// Valid cases
		{
			name:      "empty string is valid (defaults to 'message')",
			eventName: "",
			want:      true,
		},
		{
			name:      "valid event name",
			eventName: "update",
			want:      true,
		},
		{
			name:      "valid with dot",
			eventName: "user.created",
			want:      true,
		},
		{
			name:      "valid with hyphen",
			eventName: "system-alert",
			want:      true,
		},

		// Invalid cases
		{
			name:      "starts with dot",
			eventName: ".update",
			want:      false,
		},
		{
			name:      "double dots",
			eventName: "user..profile",
			want:      false,
		},
		{
			name:      "contains special character",
			eventName: "alert!",
			want:      false,
		},
		{
			name:      "starts with digit",
			eventName: "1update",
			want:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isValidSSEEventName(tt.eventName)
			if got != tt.want {
				t.Errorf("isValidSSEEventName(%q) = %v, want %v", tt.eventName, got, tt.want)
			}
		})
	}
}

// TestMessageField tests the Message struct
func TestMessage(t *testing.T) {
	t.Run("create message with all fields", func(t *testing.T) {
		msg := &Message{
			ID:    "123",
			Event: "user.login",
			Data:  []byte(`{"username":"alice"}`),
			Retry: 3000,
		}

		if msg.ID != "123" {
			t.Errorf("ID = %q, want %q", msg.ID, "123")
		}
		if msg.Event != "user.login" {
			t.Errorf("Event = %q, want %q", msg.Event, "user.login")
		}
		if string(msg.Data) != `{"username":"alice"}` {
			t.Errorf("Data = %q, want %q", msg.Data, `{"username":"alice"}`)
		}
		if msg.Retry != 3000 {
			t.Errorf("Retry = %d, want %d", msg.Retry, 3000)
		}
	})

	t.Run("create message with minimal fields", func(t *testing.T) {
		msg := &Message{
			Data: []byte("hello"),
		}

		if msg.ID != "" {
			t.Errorf("ID = %q, want empty string", msg.ID)
		}
		if msg.Event != "" {
			t.Errorf("Event = %q, want empty string", msg.Event)
		}
		if string(msg.Data) != "hello" {
			t.Errorf("Data = %q, want %q", msg.Data, "hello")
		}
		if msg.Retry != 0 {
			t.Errorf("Retry = %d, want 0", msg.Retry)
		}
	})

	t.Run("create empty message", func(t *testing.T) {
		msg := &Message{}

		if msg.ID != "" {
			t.Errorf("ID = %q, want empty string", msg.ID)
		}
		if msg.Event != "" {
			t.Errorf("Event = %q, want empty string", msg.Event)
		}
		if len(msg.Data) != 0 {
			t.Errorf("Data length = %d, want 0", len(msg.Data))
		}
		if msg.Retry != 0 {
			t.Errorf("Retry = %d, want 0", msg.Retry)
		}
	})
}

// TestConstants tests package constants
func TestConstants(t *testing.T) {
	tests := []struct {
		name     string
		got      string
		expected string
	}{
		{"fieldID", fieldID, "id"},
		{"fieldEvent", fieldEvent, "event"},
		{"fieldData", fieldData, "data"},
		{"fieldRetry", fieldRetry, "retry"},
		{"delimiter", delimiter, ":"},
		{"whitespace", whitespace, " "},
		{"eventNameMessage", eventNameMessage, "message"},
		{"invalidUTF8Replacement", invalidUTF8Replacement, "\uFFFD"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.expected {
				t.Errorf("%s = %q, want %q", tt.name, tt.got, tt.expected)
			}
		})
	}
}

// TestByteConstants tests byte slice constants
func TestByteConstants(t *testing.T) {
	tests := []struct {
		name     string
		got      []byte
		expected []byte
	}{
		{"byteLF", byteLF, []byte("\n")},
		{"byteLFLF", byteLFLF, []byte("\n\n")},
		{"byteCR", byteCR, []byte("\r")},
		{"byteEscapedCR", byteEscapedCR, []byte("\\r")},
		{"utf8BomSequence", utf8BomSequence, []byte("\xEF\xBB\xBF")},
		{"fieldPrefixID", fieldPrefixID, []byte("id: ")},
		{"fieldPrefixEvent", fieldPrefixEvent, []byte("event: ")},
		{"fieldPrefixData", fieldPrefixData, []byte("data: ")},
		{"fieldPrefixRetry", fieldPrefixRetry, []byte("retry: ")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.got) != string(tt.expected) {
				t.Errorf("%s = %q, want %q", tt.name, tt.got, tt.expected)
			}
		})
	}
}

// TestErrors tests error definitions
func TestErrors(t *testing.T) {
	t.Run("ErrMessageNoContent", func(t *testing.T) {
		if ErrMessageNoContent == nil {
			t.Error("ErrMessageNoContent should not be nil")
		}
		expectedMsg := "message has no content"
		if ErrMessageNoContent.Error() != expectedMsg {
			t.Errorf("ErrMessageNoContent.Error() = %q, want %q",
				ErrMessageNoContent.Error(), expectedMsg)
		}
	})

	t.Run("ErrMessageInvalidEventName", func(t *testing.T) {
		if ErrMessageInvalidEventName == nil {
			t.Error("ErrMessageInvalidEventName should not be nil")
		}
		expectedMsg := "message event name is invalid"
		if ErrMessageInvalidEventName.Error() != expectedMsg {
			t.Errorf("ErrMessageInvalidEventName.Error() = %q, want %q",
				ErrMessageInvalidEventName.Error(), expectedMsg)
		}
	})

	t.Run("errors are distinct", func(t *testing.T) {
		if ErrMessageNoContent == ErrMessageInvalidEventName {
			t.Error("ErrMessageNoContent and ErrMessageInvalidEventName should be different")
		}
	})
}

// TestLineBreakReplacer tests the line break replacer
func TestLineBreakReplacer(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "no line breaks",
			input: "hello world",
			want:  "hello world",
		},
		{
			name:  "single LF",
			input: "hello\nworld",
			want:  "hello\\nworld",
		},
		{
			name:  "single CR",
			input: "hello\rworld",
			want:  "hello\\rworld",
		},
		{
			name:  "CRLF",
			input: "hello\r\nworld",
			want:  "hello\\r\\nworld",
		},
		{
			name:  "multiple line breaks",
			input: "line1\nline2\rline3\r\nline4",
			want:  "line1\\nline2\\rline3\\r\\nline4",
		},
		{
			name:  "only line breaks",
			input: "\n\r\n\r",
			want:  "\\n\\r\\n\\r",
		},
		{
			name:  "empty string",
			input: "",
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := lineBreakReplacer.Replace(tt.input)
			if got != tt.want {
				t.Errorf("lineBreakReplacer.Replace(%q) = %q, want %q",
					tt.input, got, tt.want)
			}
		})
	}
}

// BenchmarkIsValidDOMEventName benchmarks the event name validation
func BenchmarkIsValidDOMEventName(b *testing.B) {
	benchmarks := []struct {
		name      string
		eventName string
	}{
		{"short valid", "update"},
		{"long valid", "system.user.authentication.success"},
		{"invalid double dot", "user..profile"},
		{"invalid special char", "event!name"},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_ = isValidDOMEventName(bm.eventName)
			}
		})
	}
}

// BenchmarkIsValidSSEEventName benchmarks SSE event name validation
func BenchmarkIsValidSSEEventName(b *testing.B) {
	benchmarks := []struct {
		name      string
		eventName string
	}{
		{"empty", ""},
		{"valid", "user.created"},
		{"invalid", ".invalid"},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_ = isValidSSEEventName(bm.eventName)
			}
		})
	}
}

// BenchmarkLineBreakReplacer benchmarks the line break replacer
func BenchmarkLineBreakReplacer(b *testing.B) {
	benchmarks := []struct {
		name  string
		input string
	}{
		{"no breaks", "hello world this is a test"},
		{"single LF", "hello\nworld"},
		{"multiple breaks", "line1\nline2\rline3\r\nline4"},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_ = lineBreakReplacer.Replace(bm.input)
			}
		})
	}
}

// TestMessageZeroValue tests zero value behavior
func TestMessageZeroValue(t *testing.T) {
	var msg Message

	if msg.ID != "" {
		t.Errorf("zero value ID = %q, want empty string", msg.ID)
	}
	if msg.Event != "" {
		t.Errorf("zero value Event = %q, want empty string", msg.Event)
	}
	if msg.Data != nil {
		t.Errorf("zero value Data = %v, want nil", msg.Data)
	}
	if msg.Retry != 0 {
		t.Errorf("zero value Retry = %d, want 0", msg.Retry)
	}
}

// TestUnicodeEventNames tests Unicode handling in event names
func TestUnicodeEventNames(t *testing.T) {
	tests := []struct {
		name      string
		eventName string
		want      bool
	}{
		{
			name:      "Chinese characters",
			eventName: "用户登录",
			want:      true,
		},
		{
			name:      "Japanese characters",
			eventName: "ユーザー",
			want:      true,
		},
		{
			name:      "Arabic characters",
			eventName: "مستخدم",
			want:      true,
		},
		{
			name:      "Emoji",
			eventName: "event😀",
			want:      false, // Emoji is not a letter
		},
		{
			name:      "Mixed ASCII and Unicode",
			eventName: "user用户",
			want:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isValidDOMEventName(tt.eventName)
			if got != tt.want {
				t.Errorf("isValidDOMEventName(%q) = %v, want %v",
					tt.eventName, got, tt.want)
			}
		})
	}
}

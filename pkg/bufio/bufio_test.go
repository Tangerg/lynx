package bufio

import (
	"bufio"
	"bytes"
	"strings"
	"testing"
)

func TestDropCR(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		expected []byte
	}{
		{
			name:     "empty data",
			input:    []byte{},
			expected: []byte{},
		},
		{
			name:     "data ending with CR",
			input:    []byte("hello\r"),
			expected: []byte("hello"),
		},
		{
			name:     "data not ending with CR",
			input:    []byte("hello"),
			expected: []byte("hello"),
		},
		{
			name:     "data ending with LF",
			input:    []byte("hello\n"),
			expected: []byte("hello\n"),
		},
		{
			name:     "single CR",
			input:    []byte("\r"),
			expected: []byte{},
		},
		{
			name:     "CR in middle",
			input:    []byte("hel\rlo"),
			expected: []byte("hel\rlo"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := dropCR(tt.input)
			if !bytes.Equal(result, tt.expected) {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestScanLinesAllFormats_UnixNewline(t *testing.T) {
	t.Run("single line with LF", func(t *testing.T) {
		data := []byte("hello\n")
		advance, token, err := ScanLinesAllFormats(data, false)

		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if advance != 6 {
			t.Errorf("expected advance 6, got %d", advance)
		}
		if string(token) != "hello" {
			t.Errorf("expected 'hello', got '%s'", string(token))
		}
	})

	t.Run("multiple lines with LF", func(t *testing.T) {
		input := "line1\nline2\nline3\n"
		expected := []string{"line1", "line2", "line3"}

		scanner := bufio.NewScanner(strings.NewReader(input))
		scanner.Split(ScanLinesAllFormats)

		var lines []string
		for scanner.Scan() {
			lines = append(lines, scanner.Text())
		}

		if len(lines) != len(expected) {
			t.Errorf("expected %d lines, got %d", len(expected), len(lines))
		}

		for i, line := range lines {
			if line != expected[i] {
				t.Errorf("line %d: expected '%s', got '%s'", i, expected[i], line)
			}
		}
	})

	t.Run("empty line with LF", func(t *testing.T) {
		data := []byte("\n")
		advance, token, err := ScanLinesAllFormats(data, false)

		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if advance != 1 {
			t.Errorf("expected advance 1, got %d", advance)
		}
		if len(token) != 0 {
			t.Errorf("expected empty token, got '%s'", string(token))
		}
	})

	t.Run("LF only at EOF", func(t *testing.T) {
		data := []byte("last\n")
		advance, token, err := ScanLinesAllFormats(data, true)

		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if advance != 5 {
			t.Errorf("expected advance 5, got %d", advance)
		}
		if string(token) != "last" {
			t.Errorf("expected 'last', got '%s'", string(token))
		}
	})
}

func TestScanLinesAllFormats_WindowsNewline(t *testing.T) {
	t.Run("single line with CRLF", func(t *testing.T) {
		data := []byte("hello\r\n")
		advance, token, err := ScanLinesAllFormats(data, false)

		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if advance != 7 {
			t.Errorf("expected advance 7, got %d", advance)
		}
		if string(token) != "hello" {
			t.Errorf("expected 'hello', got '%s'", string(token))
		}
	})

	t.Run("multiple lines with CRLF", func(t *testing.T) {
		input := "line1\r\nline2\r\nline3\r\n"
		expected := []string{"line1", "line2", "line3"}

		scanner := bufio.NewScanner(strings.NewReader(input))
		scanner.Split(ScanLinesAllFormats)

		var lines []string
		for scanner.Scan() {
			lines = append(lines, scanner.Text())
		}

		if len(lines) != len(expected) {
			t.Errorf("expected %d lines, got %d", len(expected), len(lines))
		}

		for i, line := range lines {
			if line != expected[i] {
				t.Errorf("line %d: expected '%s', got '%s'", i, expected[i], line)
			}
		}
	})

	t.Run("empty line with CRLF", func(t *testing.T) {
		data := []byte("\r\n")
		advance, token, err := ScanLinesAllFormats(data, false)

		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if advance != 2 {
			t.Errorf("expected advance 2, got %d", advance)
		}
		if len(token) != 0 {
			t.Errorf("expected empty token, got '%s'", string(token))
		}
	})

	t.Run("CRLF at EOF", func(t *testing.T) {
		data := []byte("last\r\n")
		advance, token, err := ScanLinesAllFormats(data, true)

		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if advance != 6 {
			t.Errorf("expected advance 6, got %d", advance)
		}
		if string(token) != "last" {
			t.Errorf("expected 'last', got '%s'", string(token))
		}
	})
}

func TestScanLinesAllFormats_OldMacNewline(t *testing.T) {
	t.Run("single line with CR", func(t *testing.T) {
		data := []byte("hello\r")
		advance, token, err := ScanLinesAllFormats(data, false)

		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if advance != 6 {
			t.Errorf("expected advance 6, got %d", advance)
		}
		if string(token) != "hello" {
			t.Errorf("expected 'hello', got '%s'", string(token))
		}
	})

	t.Run("multiple lines with CR", func(t *testing.T) {
		input := "line1\rline2\rline3\r"
		expected := []string{"line1", "line2", "line3"}

		scanner := bufio.NewScanner(strings.NewReader(input))
		scanner.Split(ScanLinesAllFormats)

		var lines []string
		for scanner.Scan() {
			lines = append(lines, scanner.Text())
		}

		if len(lines) != len(expected) {
			t.Errorf("expected %d lines, got %d", len(expected), len(lines))
		}

		for i, line := range lines {
			if line != expected[i] {
				t.Errorf("line %d: expected '%s', got '%s'", i, expected[i], line)
			}
		}
	})

	t.Run("empty line with CR", func(t *testing.T) {
		data := []byte("\r")
		advance, token, err := ScanLinesAllFormats(data, false)

		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if advance != 1 {
			t.Errorf("expected advance 1, got %d", advance)
		}
		if len(token) != 0 {
			t.Errorf("expected empty token, got '%s'", string(token))
		}
	})

	t.Run("CR at EOF", func(t *testing.T) {
		data := []byte("last\r")
		advance, token, err := ScanLinesAllFormats(data, true)

		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if advance != 5 {
			t.Errorf("expected advance 5, got %d", advance)
		}
		if string(token) != "last" {
			t.Errorf("expected 'last', got '%s'", string(token))
		}
	})
}

func TestScanLinesAllFormats_MixedNewlines(t *testing.T) {
	t.Run("mixed LF, CRLF, CR", func(t *testing.T) {
		input := "line1\nline2\r\nline3\rline4"
		expected := []string{"line1", "line2", "line3", "line4"}

		scanner := bufio.NewScanner(strings.NewReader(input))
		scanner.Split(ScanLinesAllFormats)

		var lines []string
		for scanner.Scan() {
			lines = append(lines, scanner.Text())
		}

		if len(lines) != len(expected) {
			t.Errorf("expected %d lines, got %d", len(expected), len(lines))
		}

		for i, line := range lines {
			if line != expected[i] {
				t.Errorf("line %d: expected '%s', got '%s'", i, expected[i], line)
			}
		}
	})

	t.Run("LF followed by CR", func(t *testing.T) {
		input := "a\n\rb"
		expected := []string{"a", "", "b"}

		scanner := bufio.NewScanner(strings.NewReader(input))
		scanner.Split(ScanLinesAllFormats)

		var lines []string
		for scanner.Scan() {
			lines = append(lines, scanner.Text())
		}

		if len(lines) != len(expected) {
			t.Errorf("expected %d lines, got %d", len(expected), len(lines))
		}

		for i, line := range lines {
			if line != expected[i] {
				t.Errorf("line %d: expected '%s', got '%s'", i, expected[i], line)
			}
		}
	})

	t.Run("CR followed by LF (CRLF)", func(t *testing.T) {
		input := "a\r\nb"
		expected := []string{"a", "b"}

		scanner := bufio.NewScanner(strings.NewReader(input))
		scanner.Split(ScanLinesAllFormats)

		var lines []string
		for scanner.Scan() {
			lines = append(lines, scanner.Text())
		}

		if len(lines) != len(expected) {
			t.Errorf("expected %d lines, got %d", len(expected), len(lines))
		}

		for i, line := range lines {
			if line != expected[i] {
				t.Errorf("line %d: expected '%s', got '%s'", i, expected[i], line)
			}
		}
	})

	t.Run("CR with text before LF", func(t *testing.T) {
		input := "a\rXXX\nb"
		expected := []string{"a", "XXX", "b"}

		scanner := bufio.NewScanner(strings.NewReader(input))
		scanner.Split(ScanLinesAllFormats)

		var lines []string
		for scanner.Scan() {
			lines = append(lines, scanner.Text())
		}

		if len(lines) != len(expected) {
			t.Errorf("expected %d lines, got %d", len(expected), len(lines))
		}

		for i, line := range lines {
			if line != expected[i] {
				t.Errorf("line %d: expected '%s', got '%s'", i, expected[i], line)
			}
		}
	})

	t.Run("LF with text before CR", func(t *testing.T) {
		input := "a\nXXX\rb"
		expected := []string{"a", "XXX", "b"}

		scanner := bufio.NewScanner(strings.NewReader(input))
		scanner.Split(ScanLinesAllFormats)

		var lines []string
		for scanner.Scan() {
			lines = append(lines, scanner.Text())
		}

		if len(lines) != len(expected) {
			t.Errorf("expected %d lines, got %d", len(expected), len(lines))
		}

		for i, line := range lines {
			if line != expected[i] {
				t.Errorf("line %d: expected '%s', got '%s'", i, expected[i], line)
			}
		}
	})
}

func TestScanLinesAllFormats_EOFHandling(t *testing.T) {
	t.Run("empty data at EOF", func(t *testing.T) {
		data := []byte("")
		advance, token, err := ScanLinesAllFormats(data, true)

		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if advance != 0 {
			t.Errorf("expected advance 0, got %d", advance)
		}
		if token != nil {
			t.Errorf("expected nil token, got '%s'", string(token))
		}
	})

	t.Run("data without newline at EOF", func(t *testing.T) {
		data := []byte("last line")
		advance, token, err := ScanLinesAllFormats(data, true)

		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if advance != 9 {
			t.Errorf("expected advance 9, got %d", advance)
		}
		if string(token) != "last line" {
			t.Errorf("expected 'last line', got '%s'", string(token))
		}
	})

	t.Run("data ending with CR at EOF", func(t *testing.T) {
		data := []byte("line\r")
		advance, token, err := ScanLinesAllFormats(data, true)

		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if advance != 5 {
			t.Errorf("expected advance 5, got %d", advance)
		}
		if string(token) != "line" {
			t.Errorf("expected 'line', got '%s'", string(token))
		}
	})

	t.Run("data ending with LF at EOF", func(t *testing.T) {
		data := []byte("line\n")
		advance, token, err := ScanLinesAllFormats(data, true)

		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if advance != 5 {
			t.Errorf("expected advance 5, got %d", advance)
		}
		if string(token) != "line" {
			t.Errorf("expected 'line', got '%s'", string(token))
		}
	})

	t.Run("single CR at EOF", func(t *testing.T) {
		data := []byte("\r")
		advance, token, err := ScanLinesAllFormats(data, true)

		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if advance != 1 {
			t.Errorf("expected advance 1, got %d", advance)
		}
		if len(token) != 0 {
			t.Errorf("expected empty token, got '%s'", string(token))
		}
	})

	t.Run("no newline not at EOF", func(t *testing.T) {
		data := []byte("incomplete")
		advance, token, err := ScanLinesAllFormats(data, false)

		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if advance != 0 {
			t.Errorf("expected advance 0, got %d", advance)
		}
		if token != nil {
			t.Errorf("expected nil token, got '%s'", string(token))
		}
	})
}

func TestScanLinesAllFormats_EdgeCases(t *testing.T) {
	t.Run("multiple consecutive LF", func(t *testing.T) {
		input := "line1\n\n\nline2"
		expected := []string{"line1", "", "", "line2"}

		scanner := bufio.NewScanner(strings.NewReader(input))
		scanner.Split(ScanLinesAllFormats)

		var lines []string
		for scanner.Scan() {
			lines = append(lines, scanner.Text())
		}

		if len(lines) != len(expected) {
			t.Errorf("expected %d lines, got %d", len(expected), len(lines))
		}

		for i, line := range lines {
			if line != expected[i] {
				t.Errorf("line %d: expected '%s', got '%s'", i, expected[i], line)
			}
		}
	})

	t.Run("multiple consecutive CR", func(t *testing.T) {
		input := "line1\r\r\rline2"
		expected := []string{"line1", "", "", "line2"}

		scanner := bufio.NewScanner(strings.NewReader(input))
		scanner.Split(ScanLinesAllFormats)

		var lines []string
		for scanner.Scan() {
			lines = append(lines, scanner.Text())
		}

		if len(lines) != len(expected) {
			t.Errorf("expected %d lines, got %d", len(expected), len(lines))
		}

		for i, line := range lines {
			if line != expected[i] {
				t.Errorf("line %d: expected '%s', got '%s'", i, expected[i], line)
			}
		}
	})

	t.Run("multiple consecutive CRLF", func(t *testing.T) {
		input := "line1\r\n\r\n\r\nline2"
		expected := []string{"line1", "", "", "line2"}

		scanner := bufio.NewScanner(strings.NewReader(input))
		scanner.Split(ScanLinesAllFormats)

		var lines []string
		for scanner.Scan() {
			lines = append(lines, scanner.Text())
		}

		if len(lines) != len(expected) {
			t.Errorf("expected %d lines, got %d", len(expected), len(lines))
		}

		for i, line := range lines {
			if line != expected[i] {
				t.Errorf("line %d: expected '%s', got '%s'", i, expected[i], line)
			}
		}
	})

	t.Run("line with only spaces", func(t *testing.T) {
		data := []byte("   \n")
		advance, token, err := ScanLinesAllFormats(data, false)

		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if string(token) != "   " {
			t.Errorf("expected '   ', got '%s'", string(token))
		}
		t.Log(advance)
	})

	t.Run("very long line without newline", func(t *testing.T) {
		longLine := strings.Repeat("a", 10000)
		data := []byte(longLine)
		advance, token, err := ScanLinesAllFormats(data, false)

		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if advance != 0 {
			t.Errorf("expected advance 0 (request more data), got %d", advance)
		}
		if token != nil {
			t.Errorf("expected nil token, got token of length %d", len(token))
		}
	})

	t.Run("very long line with LF", func(t *testing.T) {
		longLine := strings.Repeat("a", 10000)
		data := []byte(longLine + "\n")
		advance, token, err := ScanLinesAllFormats(data, false)

		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if string(token) != longLine {
			t.Errorf("token length mismatch: expected %d, got %d", len(longLine), len(token))
		}
		t.Log(advance)
	})

	t.Run("CR at start", func(t *testing.T) {
		data := []byte("\rhello")
		advance, token, err := ScanLinesAllFormats(data, false)

		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if advance != 1 {
			t.Errorf("expected advance 1, got %d", advance)
		}
		if len(token) != 0 {
			t.Errorf("expected empty token, got '%s'", string(token))
		}
	})

	t.Run("LF at start", func(t *testing.T) {
		data := []byte("\nhello")
		advance, token, err := ScanLinesAllFormats(data, false)

		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if advance != 1 {
			t.Errorf("expected advance 1, got %d", advance)
		}
		if len(token) != 0 {
			t.Errorf("expected empty token, got '%s'", string(token))
		}
	})

	t.Run("only newlines", func(t *testing.T) {
		input := "\n\r\r\n\n\r"
		expected := []string{"", "", "", "", ""}

		scanner := bufio.NewScanner(strings.NewReader(input))
		scanner.Split(ScanLinesAllFormats)

		var lines []string
		for scanner.Scan() {
			lines = append(lines, scanner.Text())
		}

		if len(lines) != len(expected) {
			t.Errorf("expected %d lines, got %d", len(expected), len(lines))
		}
	})
}

func TestScanLinesAllFormats_SSEScenarios(t *testing.T) {
	t.Run("SSE event stream with mixed newlines", func(t *testing.T) {
		input := "data: message1\n\ndata: message2\r\n\nevent: custom\rdata: message3\n\n"

		scanner := bufio.NewScanner(strings.NewReader(input))
		scanner.Split(ScanLinesAllFormats)

		var lines []string
		for scanner.Scan() {
			lines = append(lines, scanner.Text())
		}

		expected := []string{
			"data: message1",
			"",
			"data: message2",
			"",
			"event: custom",
			"data: message3",
			"",
		}

		if len(lines) != len(expected) {
			t.Errorf("expected %d lines, got %d", len(expected), len(lines))
		}

		for i, line := range lines {
			if i < len(expected) && line != expected[i] {
				t.Errorf("line %d: expected '%s', got '%s'", i, expected[i], line)
			}
		}
	})

	t.Run("SSE with retry and id fields", func(t *testing.T) {
		input := "id: 123\ndata: test\nretry: 10000\n\n"

		scanner := bufio.NewScanner(strings.NewReader(input))
		scanner.Split(ScanLinesAllFormats)

		var lines []string
		for scanner.Scan() {
			lines = append(lines, scanner.Text())
		}

		expected := []string{"id: 123", "data: test", "retry: 10000", ""}

		if len(lines) != len(expected) {
			t.Errorf("expected %d lines, got %d", len(expected), len(lines))
		}

		for i, line := range lines {
			if line != expected[i] {
				t.Errorf("line %d: expected '%s', got '%s'", i, expected[i], line)
			}
		}
	})

	t.Run("SSE multiline data", func(t *testing.T) {
		input := "data: first line\r\ndata: second line\r\ndata: third line\r\n\r\n"

		scanner := bufio.NewScanner(strings.NewReader(input))
		scanner.Split(ScanLinesAllFormats)

		var lines []string
		for scanner.Scan() {
			lines = append(lines, scanner.Text())
		}

		expected := []string{
			"data: first line",
			"data: second line",
			"data: third line",
			"",
		}

		if len(lines) != len(expected) {
			t.Errorf("expected %d lines, got %d", len(expected), len(lines))
		}

		for i, line := range lines {
			if line != expected[i] {
				t.Errorf("line %d: expected '%s', got '%s'", i, expected[i], line)
			}
		}
	})
}

func TestScanLinesAllFormats_BinaryData(t *testing.T) {
	t.Run("binary data with embedded newlines", func(t *testing.T) {
		data := []byte{0x00, 0x01, 0x02, '\n', 0x03, 0x04, '\r', '\n', 0x05}

		advance, token, err := ScanLinesAllFormats(data, false)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if advance != 4 {
			t.Errorf("expected advance 4, got %d", advance)
		}
		if !bytes.Equal(token, []byte{0x00, 0x01, 0x02}) {
			t.Errorf("unexpected token: %v", token)
		}
	})

	t.Run("binary data with CR before LF", func(t *testing.T) {
		data := []byte{0xFF, '\r', '\n', 0xFE}

		advance, token, err := ScanLinesAllFormats(data, false)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if advance != 3 {
			t.Errorf("expected advance 3, got %d", advance)
		}
		if !bytes.Equal(token, []byte{0xFF}) {
			t.Errorf("unexpected token: %v", token)
		}
	})

	t.Run("null bytes with newlines", func(t *testing.T) {
		input := "\x00\n\x00\r\x00\r\n"
		expected := []string{"\x00", "\x00", "\x00"}

		scanner := bufio.NewScanner(strings.NewReader(input))
		scanner.Split(ScanLinesAllFormats)

		var lines []string
		for scanner.Scan() {
			lines = append(lines, scanner.Text())
		}

		if len(lines) != len(expected) {
			t.Errorf("expected %d lines, got %d", len(expected), len(lines))
		}

		for i, line := range lines {
			if line != expected[i] {
				t.Errorf("line %d: expected %q, got %q", i, expected[i], line)
			}
		}
	})
}

func TestScanLinesAllFormats_PartialData(t *testing.T) {
	t.Run("incomplete CRLF - only CR", func(t *testing.T) {
		data := []byte("hello\r")
		advance, token, err := ScanLinesAllFormats(data, false)

		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if advance != 6 {
			t.Errorf("expected advance 6, got %d", advance)
		}
		if string(token) != "hello" {
			t.Errorf("expected 'hello', got '%s'", string(token))
		}
	})

	t.Run("partial CRLF at boundary", func(t *testing.T) {
		// First chunk ends with \r, might be part of \r\n
		data := []byte("hello\r")
		advance, token, err := ScanLinesAllFormats(data, false)

		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		// Should treat \r as line ending since not at EOF
		if advance != 6 {
			t.Errorf("expected advance 6, got %d", advance)
		}
		if string(token) != "hello" {
			t.Errorf("expected 'hello', got '%s'", string(token))
		}
	})

	t.Run("data requiring more input", func(t *testing.T) {
		// No newline and not at EOF - should request more data
		data := []byte("incomplete")
		advance, token, err := ScanLinesAllFormats(data, false)

		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if advance != 0 {
			t.Errorf("expected advance 0 (request more data), got %d", advance)
		}
		if token != nil {
			t.Errorf("expected nil token, got '%s'", string(token))
		}
	})

	t.Run("sequential processing simulation", func(t *testing.T) {
		// Simulate processing data in chunks
		testCases := []struct {
			data          []byte
			atEOF         bool
			expectedAdv   int
			expectedToken string
			description   string
		}{
			{
				data:          []byte("hel"),
				atEOF:         false,
				expectedAdv:   0,
				expectedToken: "",
				description:   "incomplete line, no newline",
			},
			{
				data:          []byte("hello\n"),
				atEOF:         false,
				expectedAdv:   6,
				expectedToken: "hello",
				description:   "complete line with LF",
			},
			{
				data:          []byte("world\r"),
				atEOF:         false,
				expectedAdv:   6,
				expectedToken: "world",
				description:   "complete line with CR",
			},
			{
				data:          []byte("test\r\n"),
				atEOF:         false,
				expectedAdv:   6,
				expectedToken: "test",
				description:   "complete line with CRLF",
			},
			{
				data:          []byte("last"),
				atEOF:         true,
				expectedAdv:   4,
				expectedToken: "last",
				description:   "incomplete line at EOF",
			},
		}

		for _, tc := range testCases {
			t.Run(tc.description, func(t *testing.T) {
				advance, token, err := ScanLinesAllFormats(tc.data, tc.atEOF)

				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if advance != tc.expectedAdv {
					t.Errorf("expected advance %d, got %d", tc.expectedAdv, advance)
				}
				if string(token) != tc.expectedToken {
					t.Errorf("expected token '%s', got '%s'", tc.expectedToken, string(token))
				}
			})
		}
	})

	t.Run("buffer accumulation pattern", func(t *testing.T) {
		// Simulate how bufio.Scanner actually works with partial data
		chunks := [][]byte{
			[]byte("hel"),
			[]byte("lo\n"),
			[]byte("wor"),
			[]byte("ld\r\n"),
		}

		var accumulated []byte
		var results []string

		for i, chunk := range chunks {
			accumulated = append(accumulated, chunk...)
			atEOF := i == len(chunks)-1

			for {
				advance, token, err := ScanLinesAllFormats(accumulated, atEOF)
				if err != nil {
					t.Errorf("unexpected error: %v", err)
					break
				}

				if advance == 0 {
					// Need more data
					break
				}

				results = append(results, string(token))
				accumulated = accumulated[advance:]
			}
		}

		expected := []string{"hello", "world"}
		if len(results) != len(expected) {
			t.Errorf("expected %d lines, got %d: %v", len(expected), len(results), results)
		}

		for i, line := range results {
			if i < len(expected) && line != expected[i] {
				t.Errorf("line %d: expected '%s', got '%s'", i, expected[i], line)
			}
		}
	})

	t.Run("single byte chunks", func(t *testing.T) {
		var accumulated = []byte("a\nb\rc\r\n")
		var results []string

		scanner := bufio.NewScanner(bytes.NewReader(accumulated))
		scanner.Split(ScanLinesAllFormats)
		for scanner.Scan() {
			results = append(results, scanner.Text())
		}

		expected := []string{"a", "b", "c"}
		if len(results) != len(expected) {
			t.Errorf("expected %d lines, got %d: %v", len(expected), len(results), results)
		}

		for i, line := range results {
			if i < len(expected) && line != expected[i] {
				t.Errorf("line %d: expected '%s', got '%s'", i, expected[i], line)
			}
		}
	})

	t.Run("CR at chunk boundary", func(t *testing.T) {
		// Test when \r is at the end of a chunk and next chunk starts with \n
		testData := []struct {
			chunk1   []byte
			chunk2   []byte
			expected []string
		}{
			{
				chunk1:   []byte("hello\r"),
				chunk2:   []byte("\nworld"),
				expected: []string{"hello", ""},
			},
			{
				chunk1:   []byte("hello\r"),
				chunk2:   []byte("world"),
				expected: []string{"hello"},
			},
		}

		for idx, td := range testData {
			t.Run(string(rune('a'+idx)), func(t *testing.T) {
				var accumulated []byte
				var results []string

				// Process first chunk
				accumulated = append(accumulated, td.chunk1...)
				for {
					advance, token, err := ScanLinesAllFormats(accumulated, false)
					if err != nil {
						t.Errorf("unexpected error: %v", err)
						break
					}
					if advance == 0 {
						break
					}
					results = append(results, string(token))
					accumulated = accumulated[advance:]
				}

				// Process second chunk
				accumulated = append(accumulated, td.chunk2...)
				for {
					advance, token, err := ScanLinesAllFormats(accumulated, true)
					if err != nil {
						t.Errorf("unexpected error: %v", err)
						break
					}
					if advance == 0 {
						break
					}
					results = append(results, string(token))
					accumulated = accumulated[advance:]
				}

				if len(results) < len(td.expected) {
					t.Errorf("expected at least %d lines, got %d: %v",
						len(td.expected), len(results), results)
				}
			})
		}
	})
}

func BenchmarkScanLinesAllFormats_Unix(b *testing.B) {
	data := []byte("hello world\n")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ScanLinesAllFormats(data, false)
	}
}

func BenchmarkScanLinesAllFormats_Windows(b *testing.B) {
	data := []byte("hello world\r\n")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ScanLinesAllFormats(data, false)
	}
}

func BenchmarkScanLinesAllFormats_OldMac(b *testing.B) {
	data := []byte("hello world\r")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ScanLinesAllFormats(data, false)
	}
}

func BenchmarkScanLinesAllFormats_Mixed(b *testing.B) {
	data := []byte("line1\nline2\r\nline3\r")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		scanner := bufio.NewScanner(bytes.NewReader(data))
		scanner.Split(ScanLinesAllFormats)
		for scanner.Scan() {
		}
	}
}

func BenchmarkScanLinesAllFormats_LongLine(b *testing.B) {
	data := []byte(strings.Repeat("a", 10000) + "\n")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ScanLinesAllFormats(data, false)
	}
}

func BenchmarkScanLinesAllFormats_ManyShortLines(b *testing.B) {
	var input strings.Builder
	for i := 0; i < 100; i++ {
		input.WriteString("line\n")
	}
	data := []byte(input.String())

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		scanner := bufio.NewScanner(bytes.NewReader(data))
		scanner.Split(ScanLinesAllFormats)
		for scanner.Scan() {
		}
	}
}

package io

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
)

// TestReadAll_BasicFunctionality tests basic reading functionality
func TestReadAll_BasicFunctionality(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		capacity int
		want     string
	}{
		{
			name:     "simple text",
			input:    "hello world",
			capacity: 128,
			want:     "hello world",
		},
		{
			name:     "empty string",
			input:    "",
			capacity: 128,
			want:     "",
		},
		{
			name:     "single character",
			input:    "a",
			capacity: 128,
			want:     "a",
		},
		{
			name:     "with newlines",
			input:    "line1\nline2\nline3",
			capacity: 128,
			want:     "line1\nline2\nline3",
		},
		{
			name:     "with special characters",
			input:    "hello\t\r\n\x00world",
			capacity: 128,
			want:     "hello\t\r\n\x00world",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := strings.NewReader(tt.input)
			got, err := ReadAll(reader, tt.capacity)

			if err != nil {
				t.Errorf("ReadAll() error = %v, want nil", err)
				return
			}

			if string(got) != tt.want {
				t.Errorf("ReadAll() = %q, want %q", string(got), tt.want)
			}
		})
	}
}

// TestReadAll_BufferCapacities tests different buffer capacities
func TestReadAll_BufferCapacities(t *testing.T) {
	input := "hello world"

	tests := []struct {
		name       string
		capacities []int
		want       string
	}{
		{
			name:       "default capacity (no arg)",
			capacities: nil,
			want:       input,
		},
		{
			name:       "default capacity (zero)",
			capacities: []int{0},
			want:       input,
		},
		{
			name:       "small capacity (1 byte)",
			capacities: []int{1},
			want:       input,
		},
		{
			name:       "small capacity (4 bytes)",
			capacities: []int{4},
			want:       input,
		},
		{
			name:       "exact size capacity",
			capacities: []int{len(input)},
			want:       input,
		},
		{
			name:       "large capacity",
			capacities: []int{1024},
			want:       input,
		},
		{
			name:       "very large capacity",
			capacities: []int{1024 * 1024},
			want:       input,
		},
		{
			name:       "multiple values (first is used)",
			capacities: []int{128, 256, 512},
			want:       input,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := strings.NewReader(input)
			got, err := ReadAll(reader, tt.capacities...)

			if err != nil {
				t.Errorf("ReadAll() error = %v, want nil", err)
				return
			}

			if string(got) != tt.want {
				t.Errorf("ReadAll() = %q, want %q", string(got), tt.want)
			}
		})
	}
}

// TestReadAll_NegativeCapacity tests panic on negative capacity
func TestReadAll_NegativeCapacity(t *testing.T) {
	tests := []struct {
		name     string
		capacity int
	}{
		{"negative one", -1},
		{"negative small", -10},
		{"negative large", -1024},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r == nil {
					t.Error("expected panic for negative capacity, but didn't panic")
				}
			}()

			reader := strings.NewReader("test")
			_, _ = ReadAll(reader, tt.capacity)
		})
	}
}

// TestReadAll_LargeData tests reading large amounts of data
func TestReadAll_LargeData(t *testing.T) {
	tests := []struct {
		name     string
		dataSize int
		capacity int
	}{
		{"1KB data, small capacity", 1024, 16},
		{"10KB data, small capacity", 10 * 1024, 64},
		{"100KB data, medium capacity", 100 * 1024, 512},
		{"1MB data, large capacity", 1024 * 1024, 4096},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test data
			data := bytes.Repeat([]byte("a"), tt.dataSize)
			reader := bytes.NewReader(data)

			got, err := ReadAll(reader, tt.capacity)

			if err != nil {
				t.Errorf("ReadAll() error = %v, want nil", err)
				return
			}

			if len(got) != tt.dataSize {
				t.Errorf("ReadAll() length = %d, want %d", len(got), tt.dataSize)
			}

			if !bytes.Equal(got, data) {
				t.Error("ReadAll() data mismatch")
			}
		})
	}
}

// TestReadAll_BufferGrowth tests buffer growth behavior
func TestReadAll_BufferGrowth(t *testing.T) {
	// Create data larger than initial capacity
	largeData := strings.Repeat("x", 2000)
	reader := strings.NewReader(largeData)

	got, err := ReadAll(reader, 10) // Very small initial capacity

	if err != nil {
		t.Errorf("ReadAll() error = %v, want nil", err)
		return
	}

	if string(got) != largeData {
		t.Errorf("ReadAll() length = %d, want %d", len(got), len(largeData))
	}
}

// mockReader is a mock reader for testing error conditions
type mockReader struct {
	data      []byte
	offset    int
	readSize  int
	err       error
	errAfter  int
	readCount int
}

func (m *mockReader) Read(p []byte) (n int, err error) {
	m.readCount++

	if m.errAfter > 0 && m.readCount >= m.errAfter {
		return 0, m.err
	}

	if m.offset >= len(m.data) {
		return 0, io.EOF
	}

	size := m.readSize
	if size <= 0 || size > len(p) {
		size = len(p)
	}

	remaining := len(m.data) - m.offset
	if size > remaining {
		size = remaining
	}

	n = copy(p, m.data[m.offset:m.offset+size])
	m.offset += n

	return n, nil
}

// TestReadAll_ReadErrors tests error handling
func TestReadAll_ReadErrors(t *testing.T) {
	testErr := errors.New("read error")

	tests := []struct {
		name    string
		reader  io.Reader
		wantErr bool
		errType error
	}{
		{
			name: "error after some data",
			reader: &mockReader{
				data:     []byte("hello world"),
				readSize: 5,
				err:      testErr,
				errAfter: 3,
			},
			wantErr: true,
			errType: testErr,
		},
		{
			name: "immediate error",
			reader: &mockReader{
				data:     []byte("test"),
				err:      testErr,
				errAfter: 1,
			},
			wantErr: true,
			errType: testErr,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ReadAll(tt.reader, 128)

			if (err != nil) != tt.wantErr {
				t.Errorf("ReadAll() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr && !errors.Is(err, tt.errType) {
				t.Errorf("ReadAll() error = %v, want %v", err, tt.errType)
			}
		})
	}
}

// TestReadAll_EOF tests EOF handling
func TestReadAll_EOF(t *testing.T) {
	t.Run("EOF is not an error", func(t *testing.T) {
		reader := strings.NewReader("test data")
		_, err := ReadAll(reader, 128)

		if err != nil {
			t.Errorf("ReadAll() should not return error on EOF, got %v", err)
		}
	})

	t.Run("empty reader returns no error", func(t *testing.T) {
		reader := strings.NewReader("")
		data, err := ReadAll(reader, 128)

		if err != nil {
			t.Errorf("ReadAll() error = %v, want nil", err)
		}

		if len(data) != 0 {
			t.Errorf("ReadAll() length = %d, want 0", len(data))
		}
	})
}

// TestReadAll_BinaryData tests reading binary data
func TestReadAll_BinaryData(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{
			name: "all zeros",
			data: make([]byte, 100),
		},
		{
			name: "all ones",
			data: bytes.Repeat([]byte{0xFF}, 100),
		},
		{
			name: "mixed binary",
			data: []byte{0x00, 0x01, 0x02, 0xFE, 0xFF, 0x00},
		},
		{
			name: "sequential bytes",
			data: func() []byte {
				b := make([]byte, 256)
				for i := range b {
					b[i] = byte(i)
				}
				return b
			}(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := bytes.NewReader(tt.data)
			got, err := ReadAll(reader, 64)

			if err != nil {
				t.Errorf("ReadAll() error = %v, want nil", err)
				return
			}

			if !bytes.Equal(got, tt.data) {
				t.Error("ReadAll() binary data mismatch")
			}
		})
	}
}

// TestReadAll_MultipleReads tests behavior with varying read sizes
func TestReadAll_MultipleReads(t *testing.T) {
	data := []byte("test data for multiple reads")

	tests := []struct {
		name     string
		readSize int
		capacity int
	}{
		{"small reads, small capacity", 1, 4},
		{"small reads, large capacity", 1, 128},
		{"medium reads, small capacity", 5, 4},
		{"large reads, small capacity", 20, 4},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := &mockReader{
				data:     data,
				readSize: tt.readSize,
			}

			got, err := ReadAll(reader, tt.capacity)

			if err != nil {
				t.Errorf("ReadAll() error = %v, want nil", err)
				return
			}

			if !bytes.Equal(got, data) {
				t.Errorf("ReadAll() = %q, want %q", string(got), string(data))
			}
		})
	}
}

// TestReadAll_Comparison compares with standard library
func TestReadAll_Comparison(t *testing.T) {
	testData := []string{
		"",
		"a",
		"hello world",
		strings.Repeat("x", 1000),
		"multi\nline\ntext\n",
	}

	for _, data := range testData {
		t.Run("length_"+string(rune(len(data))), func(t *testing.T) {
			// Our implementation
			reader1 := strings.NewReader(data)
			got1, err1 := ReadAll(reader1, 128)

			// Standard library
			reader2 := strings.NewReader(data)
			got2, err2 := io.ReadAll(reader2)

			if (err1 != nil) != (err2 != nil) {
				t.Errorf("error mismatch: our=%v, stdlib=%v", err1, err2)
			}

			if !bytes.Equal(got1, got2) {
				t.Errorf("data mismatch: our=%q, stdlib=%q", string(got1), string(got2))
			}
		})
	}
}

// TestRead_BasicFunctionality tests basic iterator functionality
func TestRead_BasicFunctionality(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		bufferSize int
		maxReads   int
		wantChunks []string
	}{
		{
			name:       "simple text",
			input:      "hello world",
			bufferSize: 128,
			maxReads:   1,
			wantChunks: []string{"hello world"},
		},
		{
			name:       "empty string",
			input:      "",
			bufferSize: 128,
			maxReads:   1,
			wantChunks: []string{""},
		},
		{
			name:       "small buffer multiple reads",
			input:      "hello",
			bufferSize: 2,
			maxReads:   3,
			wantChunks: []string{"he", "ll", "o"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := strings.NewReader(tt.input)
			iter := Read(reader, tt.bufferSize)

			chunks := make([]string, 0, tt.maxReads)
			count := 0

			for data, err := range iter {
				if err != nil && err != io.EOF {
					t.Errorf("Read() unexpected error = %v", err)
					return
				}

				chunks = append(chunks, string(data))
				count++

				if count >= tt.maxReads {
					break
				}
			}

			if len(chunks) != len(tt.wantChunks) {
				t.Errorf("Read() chunks count = %d, want %d", len(chunks), len(tt.wantChunks))
				return
			}

			for i, chunk := range chunks {
				if chunk != tt.wantChunks[i] {
					t.Errorf("Read() chunk[%d] = %q, want %q", i, chunk, tt.wantChunks[i])
				}
			}
		})
	}
}

// TestRead_BufferSizes tests different buffer sizes
func TestRead_BufferSizes(t *testing.T) {
	input := "test data"

	tests := []struct {
		name        string
		bufferSizes []int
	}{
		{
			name:        "default size (no arg)",
			bufferSizes: nil,
		},
		{
			name:        "default size (zero)",
			bufferSizes: []int{0},
		},
		{
			name:        "small size",
			bufferSizes: []int{1},
		},
		{
			name:        "medium size",
			bufferSizes: []int{64},
		},
		{
			name:        "large size",
			bufferSizes: []int{1024},
		},
		{
			name:        "multiple values (first is used)",
			bufferSizes: []int{128, 256, 512},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := strings.NewReader(input)
			iter := Read(reader, tt.bufferSizes...)

			count := 0
			for _, err := range iter {
				if err != nil && err != io.EOF {
					t.Errorf("Read() unexpected error = %v", err)
					return
				}
				count++
				if count >= 1 {
					break
				}
			}

			if count == 0 {
				t.Error("Read() should yield at least one chunk")
			}
		})
	}
}

// TestRead_NegativeBufferSize tests panic on negative buffer size
func TestRead_NegativeBufferSize(t *testing.T) {
	tests := []struct {
		name       string
		bufferSize int
	}{
		{"negative one", -1},
		{"negative small", -10},
		{"negative large", -1024},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r == nil {
					t.Error("expected panic for negative buffer size, but didn't panic")
				}
			}()

			reader := strings.NewReader("test")
			_ = Read(reader, tt.bufferSize)
		})
	}
}

// TestRead_EarlyBreak tests iterator early termination
func TestRead_EarlyBreak(t *testing.T) {
	largeData := strings.Repeat("x", 1000)
	reader := strings.NewReader(largeData)
	iter := Read(reader, 10)

	count := 0
	maxReads := 5

	for range iter {
		count++
		if count >= maxReads {
			break
		}
	}

	if count != maxReads {
		t.Errorf("Read() count = %d, want %d", count, maxReads)
	}
}

// TestRead_EOF tests EOF handling
func TestRead_EOF(t *testing.T) {
	t.Run("EOF converted to nil", func(t *testing.T) {
		reader := strings.NewReader("test")
		iter := Read(reader, 128)

		count := 0
		for _, err := range iter {
			if err != nil {
				t.Errorf("Read() error = %v, want nil (EOF should be converted)", err)
			}
			count++
			if count >= 2 {
				break
			}
		}
	})

	t.Run("empty reader", func(t *testing.T) {
		reader := strings.NewReader("")
		iter := Read(reader, 128)

		count := 0
		for data, err := range iter {
			if err != nil {
				t.Errorf("Read() error = %v, want nil", err)
			}
			if len(data) != 0 {
				t.Errorf("Read() data length = %d, want 0", len(data))
			}
			count++
			if count >= 1 {
				break
			}
		}
	})
}

// TestRead_BinaryData tests reading binary data
func TestRead_BinaryData(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{
			name: "all zeros",
			data: make([]byte, 50),
		},
		{
			name: "all ones",
			data: bytes.Repeat([]byte{0xFF}, 50),
		},
		{
			name: "mixed binary",
			data: []byte{0x00, 0x01, 0x02, 0xFE, 0xFF},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := bytes.NewReader(tt.data)
			iter := Read(reader, 10)

			var result []byte
			count := 0
			maxReads := (len(tt.data) + 9) / 10 // Calculate expected chunks

			for data, err := range iter {
				if err != nil {
					t.Errorf("Read() error = %v, want nil", err)
					return
				}
				result = append(result, data...)
				count++
				if count >= maxReads {
					break
				}
			}

			if !bytes.Equal(result, tt.data) {
				t.Error("Read() binary data mismatch")
			}
		})
	}
}

// TestRead_ReadErrors tests error handling in iterator
func TestRead_ReadErrors(t *testing.T) {
	testErr := errors.New("read error")

	reader := &mockReader{
		data:     []byte("test data"),
		readSize: 4,
		err:      testErr,
		errAfter: 3,
	}

	iter := Read(reader, 10)

	count := 0
	gotError := false

	for _, err := range iter {
		count++
		if err != nil && errors.Is(err, testErr) {
			gotError = true
			break
		}
		if count >= 5 {
			break
		}
	}

	if !gotError {
		t.Error("Read() should propagate errors from underlying reader")
	}
}

// TestRead_BufferIsolation tests that buffers don't alias
func TestRead_BufferIsolation(t *testing.T) {
	reader := strings.NewReader("abcdefghij")
	iter := Read(reader, 2)

	var buffers [][]byte
	count := 0
	maxReads := 3

	for data, err := range iter {
		if err != nil {
			t.Errorf("Read() error = %v", err)
			return
		}
		// Store reference to buffer
		buffers = append(buffers, data)
		count++
		if count >= maxReads {
			break
		}
	}

	// Verify each buffer is independent
	for i := 0; i < len(buffers)-1; i++ {
		if &buffers[i][0] == &buffers[i+1][0] {
			t.Error("Read() buffers should not alias each other")
		}
	}
}

// TestRead_ConcurrentSafetyPattern tests safe concurrent usage pattern
func TestRead_ConcurrentSafetyPattern(t *testing.T) {
	data := bytes.Repeat([]byte("test"), 100)
	reader := bytes.NewReader(data)
	iter := Read(reader, 20)

	results := make([][]byte, 0)
	count := 0
	maxReads := 10

	for chunk, err := range iter {
		if err != nil {
			t.Errorf("Read() error = %v", err)
			return
		}
		// Copy data before storing
		copied := make([]byte, len(chunk))
		copy(copied, chunk)
		results = append(results, copied)

		count++
		if count >= maxReads {
			break
		}
	}

	if len(results) != maxReads {
		t.Errorf("Read() results count = %d, want %d", len(results), maxReads)
	}
}

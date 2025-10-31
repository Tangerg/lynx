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
		name       string
		input      string
		bufferSize int
		want       string
	}{
		{
			name:       "simple text",
			input:      "hello world",
			bufferSize: 128,
			want:       "hello world",
		},
		{
			name:       "empty string",
			input:      "",
			bufferSize: 128,
			want:       "",
		},
		{
			name:       "single character",
			input:      "a",
			bufferSize: 128,
			want:       "a",
		},
		{
			name:       "with newlines",
			input:      "line1\nline2\nline3",
			bufferSize: 128,
			want:       "line1\nline2\nline3",
		},
		{
			name:       "with special characters",
			input:      "hello\t\r\n\x00world",
			bufferSize: 128,
			want:       "hello\t\r\n\x00world",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := strings.NewReader(tt.input)
			got, err := ReadAll(reader, tt.bufferSize)

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

// TestReadAll_BufferSizes tests different buffer sizes
func TestReadAll_BufferSizes(t *testing.T) {
	input := "hello world"

	tests := []struct {
		name       string
		bufferSize []int
		want       string
	}{
		{
			name:       "default buffer size (no arg)",
			bufferSize: nil,
			want:       input,
		},
		{
			name:       "default buffer size (zero)",
			bufferSize: []int{0},
			want:       input,
		},
		{
			name:       "small buffer (1 byte)",
			bufferSize: []int{1},
			want:       input,
		},
		{
			name:       "small buffer (4 bytes)",
			bufferSize: []int{4},
			want:       input,
		},
		{
			name:       "exact size buffer",
			bufferSize: []int{len(input)},
			want:       input,
		},
		{
			name:       "large buffer",
			bufferSize: []int{1024},
			want:       input,
		},
		{
			name:       "very large buffer",
			bufferSize: []int{1024 * 1024},
			want:       input,
		},
		{
			name:       "multiple values (first is used)",
			bufferSize: []int{128, 256, 512},
			want:       input,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := strings.NewReader(input)
			got, err := ReadAll(reader, tt.bufferSize...)

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

// TestReadAll_NegativeBufferSize tests panic on negative buffer size
func TestReadAll_NegativeBufferSize(t *testing.T) {
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
			_, _ = ReadAll(reader, tt.bufferSize)
		})
	}
}

// TestReadAll_LargeData tests reading large amounts of data
func TestReadAll_LargeData(t *testing.T) {
	tests := []struct {
		name       string
		dataSize   int
		bufferSize int
	}{
		{"1KB data, small buffer", 1024, 16},
		{"10KB data, small buffer", 10 * 1024, 64},
		{"100KB data, medium buffer", 100 * 1024, 512},
		{"1MB data, large buffer", 1024 * 1024, 4096},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test data
			data := bytes.Repeat([]byte("a"), tt.dataSize)
			reader := bytes.NewReader(data)

			got, err := ReadAll(reader, tt.bufferSize)

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
	// Create data larger than initial buffer
	largeData := strings.Repeat("x", 2000)
	reader := strings.NewReader(largeData)

	got, err := ReadAll(reader, 10) // Very small initial buffer

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
		name       string
		readSize   int
		bufferSize int
	}{
		{"small reads, small buffer", 1, 4},
		{"small reads, large buffer", 1, 128},
		{"medium reads, small buffer", 5, 4},
		{"large reads, small buffer", 20, 4},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := &mockReader{
				data:     data,
				readSize: tt.readSize,
			}

			got, err := ReadAll(reader, tt.bufferSize)

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

// BenchmarkReadAll benchmarks ReadAll with different buffer sizes
func BenchmarkReadAll(b *testing.B) {
	data := bytes.Repeat([]byte("x"), 10*1024) // 10KB

	sizes := []int{16, 64, 128, 512, 1024, 4096}

	for _, size := range sizes {
		b.Run("buffer_"+string(rune(size)), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				reader := bytes.NewReader(data)
				_, _ = ReadAll(reader, size)
			}
		})
	}
}

// BenchmarkReadAll_vsStdLib compares performance with standard library
func BenchmarkReadAll_vsStdLib(b *testing.B) {
	data := bytes.Repeat([]byte("x"), 10*1024)

	b.Run("custom_implementation", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			reader := bytes.NewReader(data)
			_, _ = ReadAll(reader, 512)
		}
	})

	b.Run("standard_library", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			reader := bytes.NewReader(data)
			_, _ = io.ReadAll(reader)
		}
	})
}

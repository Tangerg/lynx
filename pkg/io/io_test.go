package io

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
)

// mockReader yields fixed-size chunks from data and can synthesize a
// read error after errAfter calls.
type mockReader struct {
	data      []byte
	off       int
	readSize  int
	err       error
	errAfter  int
	readCount int
}

func (m *mockReader) Read(p []byte) (int, error) {
	m.readCount++
	if m.errAfter > 0 && m.readCount >= m.errAfter {
		return 0, m.err
	}
	if m.off >= len(m.data) {
		return 0, io.EOF
	}
	sz := m.readSize
	if sz <= 0 || sz > len(p) {
		sz = len(p)
	}
	if r := len(m.data) - m.off; sz > r {
		sz = r
	}
	n := copy(p, m.data[m.off:m.off+sz])
	m.off += n
	return n, nil
}

func TestReadAll_Content(t *testing.T) {
	tests := []struct {
		name string
		in   string
	}{
		{"text", "hello world"},
		{"empty", ""},
		{"single", "a"},
		{"newlines", "line1\nline2\nline3"},
		{"specials", "hello\t\r\n\x00world"},
		{"larger than buf", strings.Repeat("x", 2000)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ReadAll(strings.NewReader(tt.in), 64)
			if err != nil {
				t.Fatalf("err = %v", err)
			}
			if string(got) != tt.in {
				t.Errorf("got %q, want %q", got, tt.in)
			}
		})
	}
}

func TestReadAll_BufferSize(t *testing.T) {
	t.Run("default", func(t *testing.T) {
		got, err := ReadAll(strings.NewReader("hi"))
		if err != nil || string(got) != "hi" {
			t.Errorf("got %q err %v", got, err)
		}
	})
	t.Run("zero uses default", func(t *testing.T) {
		got, err := ReadAll(strings.NewReader("hi"), 0)
		if err != nil || string(got) != "hi" {
			t.Errorf("got %q err %v", got, err)
		}
	})
	t.Run("negative panics", func(t *testing.T) {
		defer func() {
			if recover() == nil {
				t.Error("expected panic")
			}
		}()
		_, _ = ReadAll(strings.NewReader("hi"), -1)
	})
	t.Run("only first variadic used", func(t *testing.T) {
		// Negative as second arg must not panic.
		got, err := ReadAll(strings.NewReader("hi"), 8, -1)
		if err != nil || string(got) != "hi" {
			t.Errorf("got %q err %v", got, err)
		}
	})
}

func TestReadAll_PropagatesError(t *testing.T) {
	sentinel := errors.New("read error")
	r := &mockReader{
		data:     []byte("hello world"),
		readSize: 5,
		err:      sentinel,
		errAfter: 3,
	}
	_, err := ReadAll(r, 128)
	if !errors.Is(err, sentinel) {
		t.Errorf("err = %v, want %v", err, sentinel)
	}
}

func TestReadAll_BinaryData(t *testing.T) {
	tests := [][]byte{
		make([]byte, 100),
		bytes.Repeat([]byte{0xFF}, 100),
		{0x00, 0x01, 0x02, 0xFE, 0xFF, 0x00},
		func() []byte {
			b := make([]byte, 256)
			for i := range b {
				b[i] = byte(i)
			}
			return b
		}(),
	}
	for i, in := range tests {
		t.Run("", func(t *testing.T) {
			got, err := ReadAll(bytes.NewReader(in), 64)
			if err != nil {
				t.Fatalf("[%d] err = %v", i, err)
			}
			if !bytes.Equal(got, in) {
				t.Errorf("[%d] mismatch", i)
			}
		})
	}
}

func TestRead_StreamsAllChunks(t *testing.T) {
	in := strings.Repeat("abcdefghij", 10) // 100 bytes
	var got []byte
	for chunk, err := range Read(strings.NewReader(in), 16) {
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		got = append(got, chunk...)
	}
	if string(got) != in {
		t.Errorf("got %q", got)
	}
}

func TestRead_EmptyReader(t *testing.T) {
	count := 0
	for chunk, err := range Read(strings.NewReader(""), 16) {
		count++
		if err != nil {
			t.Errorf("err = %v", err)
		}
		if len(chunk) != 0 {
			t.Errorf("chunk len = %d", len(chunk))
		}
	}
	if count > 1 {
		t.Errorf("iterated %d times on empty reader", count)
	}
}

func TestRead_PropagatesError(t *testing.T) {
	sentinel := errors.New("read error")
	r := &mockReader{
		data:     []byte("hello"),
		readSize: 2,
		err:      sentinel,
		errAfter: 2,
	}
	var lastErr error
	for _, err := range Read(r, 4) {
		if err != nil {
			lastErr = err
		}
	}
	if !errors.Is(lastErr, sentinel) {
		t.Errorf("err = %v, want %v", lastErr, sentinel)
	}
}

func TestRead_EarlyBreak(t *testing.T) {
	in := strings.Repeat("xxxx", 100)
	count := 0
	for range Read(strings.NewReader(in), 4) {
		count++
		if count == 3 {
			break
		}
	}
	if count != 3 {
		t.Errorf("count = %d, want 3", count)
	}
}

func TestRead_NegativeBufSizePanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("expected panic")
		}
	}()
	for range Read(strings.NewReader("x"), -1) {
	}
}

func BenchmarkReadAll(b *testing.B) {
	data := bytes.Repeat([]byte("x"), 4096)
	for b.Loop() {
		_, _ = ReadAll(bytes.NewReader(data), 4096)
	}
}

func BenchmarkRead(b *testing.B) {
	data := bytes.Repeat([]byte("x"), 4096)
	for b.Loop() {
		for range Read(bytes.NewReader(data), 512) {
		}
	}
}

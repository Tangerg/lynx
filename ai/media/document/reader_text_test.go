package document

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewTextReader tests the constructor
func TestNewTextReader(t *testing.T) {
	t.Run("with default buffer size", func(t *testing.T) {
		reader := strings.NewReader("test")
		textReader, err := NewTextReader(reader)

		require.NoError(t, err)
		require.NotNil(t, textReader)
		assert.Equal(t, reader, textReader.reader)
		assert.Equal(t, 8192, textReader.bufferSize) // default
	})

	t.Run("with custom buffer size", func(t *testing.T) {
		reader := strings.NewReader("test")
		textReader, err := NewTextReader(reader, 4096)

		require.NoError(t, err)
		require.NotNil(t, textReader)
		assert.Equal(t, 4096, textReader.bufferSize)
	})

	t.Run("with zero buffer size uses default", func(t *testing.T) {
		reader := strings.NewReader("test")
		textReader, err := NewTextReader(reader, 0)

		require.NoError(t, err)
		require.NotNil(t, textReader)
		assert.Equal(t, 8192, textReader.bufferSize)
	})

	t.Run("with negative buffer size uses default", func(t *testing.T) {
		reader := strings.NewReader("test")
		textReader, err := NewTextReader(reader, -100)

		require.NoError(t, err)
		require.NotNil(t, textReader)
		assert.Equal(t, 8192, textReader.bufferSize)
	})

	t.Run("with multiple sizes uses first", func(t *testing.T) {
		reader := strings.NewReader("test")
		textReader, err := NewTextReader(reader, 2048, 4096, 8192)

		require.NoError(t, err)
		require.NotNil(t, textReader)
		assert.Equal(t, 2048, textReader.bufferSize)
	})

	t.Run("with no sizes uses default", func(t *testing.T) {
		reader := strings.NewReader("test")
		textReader, err := NewTextReader(reader)

		require.NoError(t, err)
		require.NotNil(t, textReader)
		assert.Equal(t, 8192, textReader.bufferSize)
	})

	t.Run("with nil reader returns error", func(t *testing.T) {
		textReader, err := NewTextReader(nil)

		require.Error(t, err)
		assert.Nil(t, textReader)
		assert.Equal(t, "reader is nil", err.Error())
	})

	t.Run("with nil reader and custom buffer size returns error", func(t *testing.T) {
		textReader, err := NewTextReader(nil, 4096)

		require.Error(t, err)
		assert.Nil(t, textReader)
		assert.Equal(t, "reader is nil", err.Error())
	})
}

// TestTextReader_Read tests the Read method
func TestTextReader_Read(t *testing.T) {
	ctx := context.Background()

	t.Run("read simple text", func(t *testing.T) {
		reader, err := NewTextReader(strings.NewReader("hello world"))
		require.NoError(t, err)

		docs, err := reader.Read(ctx)

		require.NoError(t, err)
		require.Len(t, docs, 1)
		assert.Equal(t, "hello world", docs[0].Text)
	})

	t.Run("read empty string", func(t *testing.T) {
		reader, err := NewTextReader(strings.NewReader(""))
		require.NoError(t, err)

		docs, err := reader.Read(ctx)

		require.Error(t, err)
		require.Len(t, docs, 0)
	})

	t.Run("read multiline text", func(t *testing.T) {
		text := "line 1\nline 2\nline 3"
		reader, err := NewTextReader(strings.NewReader(text))
		require.NoError(t, err)

		docs, err := reader.Read(ctx)

		require.NoError(t, err)
		require.Len(t, docs, 1)
		assert.Equal(t, text, docs[0].Text)
	})

	t.Run("read text with special characters", func(t *testing.T) {
		text := "Special chars: !@#$%^&*()_+-=[]{}|;':\",./<>?"
		reader, err := NewTextReader(strings.NewReader(text))
		require.NoError(t, err)

		docs, err := reader.Read(ctx)

		require.NoError(t, err)
		require.Len(t, docs, 1)
		assert.Equal(t, text, docs[0].Text)
	})

	t.Run("read text with unicode", func(t *testing.T) {
		text := "Unicode: 你好世界 🌍🎉 Ñoño"
		reader, err := NewTextReader(strings.NewReader(text))
		require.NoError(t, err)

		docs, err := reader.Read(ctx)

		require.NoError(t, err)
		require.Len(t, docs, 1)
		assert.Equal(t, text, docs[0].Text)
	})

	t.Run("read text with tabs and spaces", func(t *testing.T) {
		text := "Tab:\there\nSpace:  multiple  spaces"
		reader, err := NewTextReader(strings.NewReader(text))
		require.NoError(t, err)

		docs, err := reader.Read(ctx)

		require.NoError(t, err)
		require.Len(t, docs, 1)
		assert.Equal(t, text, docs[0].Text)
	})

	t.Run("read long text", func(t *testing.T) {
		text := strings.Repeat("Long text. ", 1000)
		reader, err := NewTextReader(strings.NewReader(text))
		require.NoError(t, err)

		docs, err := reader.Read(ctx)

		require.NoError(t, err)
		require.Len(t, docs, 1)
		assert.Equal(t, text, docs[0].Text)
	})

	t.Run("read with custom buffer size", func(t *testing.T) {
		text := "Custom buffer size test"
		reader, err := NewTextReader(strings.NewReader(text), 1024)
		require.NoError(t, err)

		docs, err := reader.Read(ctx)

		require.NoError(t, err)
		require.Len(t, docs, 1)
		assert.Equal(t, text, docs[0].Text)
	})

	t.Run("read with small buffer size", func(t *testing.T) {
		text := "This text is longer than the buffer"
		reader, err := NewTextReader(strings.NewReader(text), 10)
		require.NoError(t, err)

		docs, err := reader.Read(ctx)

		require.NoError(t, err)
		require.Len(t, docs, 1)
		assert.Equal(t, text, docs[0].Text)
	})

	t.Run("context cancellation does not affect read", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		reader, err := NewTextReader(strings.NewReader("test"))
		require.NoError(t, err)

		docs, err := reader.Read(ctx)

		require.NoError(t, err)
		require.Len(t, docs, 1)
		assert.Equal(t, "test", docs[0].Text)
	})

	t.Run("nil context", func(t *testing.T) {
		reader, err := NewTextReader(strings.NewReader("test"))
		require.NoError(t, err)

		docs, err := reader.Read(nil)

		require.NoError(t, err)
		require.Len(t, docs, 1)
		assert.Equal(t, "test", docs[0].Text)
	})
}

// TestTextReader_ReadErrors tests error handling
func TestTextReader_ReadErrors(t *testing.T) {
	ctx := context.Background()

	t.Run("reader returns error", func(t *testing.T) {
		expectedErr := errors.New("read error")
		errorReader := &errorReader{err: expectedErr}
		reader, err := NewTextReader(errorReader)
		require.NoError(t, err)

		docs, err := reader.Read(ctx)

		require.Error(t, err)
		assert.Nil(t, docs)
		assert.Equal(t, expectedErr, err)
	})

	t.Run("reader returns unexpected EOF", func(t *testing.T) {
		errorReader := &errorReader{err: io.ErrUnexpectedEOF}
		reader, err := NewTextReader(errorReader)
		require.NoError(t, err)

		docs, err := reader.Read(ctx)

		require.Error(t, err)
		assert.Nil(t, docs)
		assert.Equal(t, io.ErrUnexpectedEOF, err)
	})

	t.Run("constructor with nil reader returns error", func(t *testing.T) {
		reader, err := NewTextReader(nil)

		require.Error(t, err)
		assert.Nil(t, reader)
		assert.Equal(t, "reader is nil", err.Error())
	})
}

// TestTextReader_ReadEdgeCases tests edge cases
func TestTextReader_ReadEdgeCases(t *testing.T) {
	ctx := context.Background()

	t.Run("whitespace only", func(t *testing.T) {
		reader, err := NewTextReader(strings.NewReader("   \t\n   "))
		require.NoError(t, err)

		docs, err := reader.Read(ctx)

		require.NoError(t, err)
		require.Len(t, docs, 1)
		assert.Equal(t, "   \t\n   ", docs[0].Text)
	})

	t.Run("null bytes", func(t *testing.T) {
		text := "text\x00with\x00null\x00bytes"
		reader, err := NewTextReader(strings.NewReader(text))
		require.NoError(t, err)

		docs, err := reader.Read(ctx)

		require.NoError(t, err)
		require.Len(t, docs, 1)
		assert.Equal(t, text, docs[0].Text)
	})

	t.Run("very long line", func(t *testing.T) {
		text := strings.Repeat("a", 100000)
		reader, err := NewTextReader(strings.NewReader(text))
		require.NoError(t, err)

		docs, err := reader.Read(ctx)

		require.NoError(t, err)
		require.Len(t, docs, 1)
		assert.Equal(t, text, docs[0].Text)
	})

	t.Run("mixed line endings", func(t *testing.T) {
		text := "line1\nline2\rline3\r\nline4"
		reader, err := NewTextReader(strings.NewReader(text))
		require.NoError(t, err)

		docs, err := reader.Read(ctx)

		require.NoError(t, err)
		require.Len(t, docs, 1)
		assert.Equal(t, text, docs[0].Text)
	})

	t.Run("emoji and special unicode", func(t *testing.T) {
		text := "😀🎉🌍 Unicode test 中文 العربية עברית"
		reader, err := NewTextReader(strings.NewReader(text))
		require.NoError(t, err)

		docs, err := reader.Read(ctx)

		require.NoError(t, err)
		require.Len(t, docs, 1)
		assert.Equal(t, text, docs[0].Text)
	})

	t.Run("control characters", func(t *testing.T) {
		text := "text\x01\x02\x03control\x1f"
		reader, err := NewTextReader(strings.NewReader(text))
		require.NoError(t, err)

		docs, err := reader.Read(ctx)

		require.NoError(t, err)
		require.Len(t, docs, 1)
		assert.Equal(t, text, docs[0].Text)
	})

	t.Run("repeated reads exhaust reader", func(t *testing.T) {
		text := "test content"
		ioReader := strings.NewReader(text)
		reader, err := NewTextReader(ioReader)
		require.NoError(t, err)

		// First read
		docs1, err1 := reader.Read(ctx)
		require.NoError(t, err1)
		require.Len(t, docs1, 1)
		assert.Equal(t, text, docs1[0].Text)

		// Second read will return empty because reader is exhausted
		docs2, err2 := reader.Read(ctx)
		require.Error(t, err2) // Empty string error
		require.Len(t, docs2, 0)
	})
}

// TestTextReader_Document tests document properties
func TestTextReader_Document(t *testing.T) {
	ctx := context.Background()

	t.Run("document has correct text", func(t *testing.T) {
		text := "Document text content"
		reader, err := NewTextReader(strings.NewReader(text))
		require.NoError(t, err)

		docs, err := reader.Read(ctx)

		require.NoError(t, err)
		require.Len(t, docs, 1)
		assert.Equal(t, text, docs[0].Text)
	})

	t.Run("document metadata is initialized", func(t *testing.T) {
		reader, err := NewTextReader(strings.NewReader("test"))
		require.NoError(t, err)

		docs, err := reader.Read(ctx)

		require.NoError(t, err)
		require.Len(t, docs, 1)
		assert.NotNil(t, docs[0].Metadata)
	})

	t.Run("document text is nil", func(t *testing.T) {
		reader, err := NewTextReader(strings.NewReader("test"))
		require.NoError(t, err)

		docs, err := reader.Read(ctx)

		require.NoError(t, err)
		require.Len(t, docs, 1)
		// Text should be nil by default
		assert.Equal(t, docs[0].Text, "test")
	})
}

// TestTextReader_BufferSizes tests different buffer sizes
func TestTextReader_BufferSizes(t *testing.T) {
	ctx := context.Background()
	text := strings.Repeat("Test content. ", 1000) // ~14KB

	bufferSizes := []int{
		10,    // Very small
		128,   // Small
		1024,  // 1KB
		4096,  // 4KB
		8192,  // Default
		16384, // Large
		32768, // Very large
	}

	for _, size := range bufferSizes {
		t.Run(string(rune(size)), func(t *testing.T) {
			reader, err := NewTextReader(strings.NewReader(text), size)
			require.NoError(t, err)

			docs, err := reader.Read(ctx)

			require.NoError(t, err)
			require.Len(t, docs, 1)
			assert.Equal(t, text, docs[0].Text)
		})
	}
}

// TestTextReader_InterfaceCompliance verifies interface implementation
func TestTextReader_InterfaceCompliance(t *testing.T) {
	reader, err := NewTextReader(strings.NewReader("test"))
	require.NoError(t, err)
	var _ Reader = reader
}

// TestTextReader_Integration tests complete workflows
func TestTextReader_Integration(t *testing.T) {
	ctx := context.Background()

	t.Run("read markdown document", func(t *testing.T) {
		markdown := `# Title

## Subtitle

This is a paragraph with **bold** and *italic* text.

- List item 1
- List item 2

[Link](https://example.com)
`
		reader, err := NewTextReader(strings.NewReader(markdown))
		require.NoError(t, err)

		docs, err := reader.Read(ctx)

		require.NoError(t, err)
		require.Len(t, docs, 1)
		assert.Equal(t, markdown, docs[0].Text)
		assert.Contains(t, docs[0].Text, "# Title")
		assert.Contains(t, docs[0].Text, "**bold**")
	})

	t.Run("read code file", func(t *testing.T) {
		code := `package main

import "fmt"

func main() {
	fmt.Println("Hello, World!")
}
`
		reader, err := NewTextReader(strings.NewReader(code))
		require.NoError(t, err)

		docs, err := reader.Read(ctx)

		require.NoError(t, err)
		require.Len(t, docs, 1)
		assert.Equal(t, code, docs[0].Text)
		assert.Contains(t, docs[0].Text, "package main")
	})

	t.Run("read log file", func(t *testing.T) {
		log := `2025-01-01 10:00:00 INFO Starting application
2025-01-01 10:00:01 DEBUG Loading configuration
2025-01-01 10:00:02 ERROR Failed to connect
2025-01-01 10:00:03 INFO Retrying...
`
		reader, err := NewTextReader(strings.NewReader(log))
		require.NoError(t, err)

		docs, err := reader.Read(ctx)

		require.NoError(t, err)
		require.Len(t, docs, 1)
		assert.Equal(t, log, docs[0].Text)
		assert.Contains(t, docs[0].Text, "ERROR")
	})

	t.Run("read configuration file", func(t *testing.T) {
		config := `# Configuration
server:
  host: localhost
  port: 8080
database:
  url: postgres://localhost/db
  pool_size: 10
`
		reader, err := NewTextReader(strings.NewReader(config))
		require.NoError(t, err)

		docs, err := reader.Read(ctx)

		require.NoError(t, err)
		require.Len(t, docs, 1)
		assert.Equal(t, config, docs[0].Text)
	})

	t.Run("read CSV data", func(t *testing.T) {
		csv := `id,name,email
1,Alice,alice@example.com
2,Bob,bob@example.com
3,Charlie,charlie@example.com
`
		reader, err := NewTextReader(strings.NewReader(csv))
		require.NoError(t, err)

		docs, err := reader.Read(ctx)

		require.NoError(t, err)
		require.Len(t, docs, 1)
		assert.Equal(t, csv, docs[0].Text)
	})

	t.Run("read article with multiple paragraphs", func(t *testing.T) {
		article := `Lorem ipsum dolor sit amet, consectetur adipiscing elit.

Sed do eiusmod tempor incididunt ut labore et dolore magna aliqua.

Ut enim ad minim veniam, quis nostrud exercitation ullamco laboris.
`
		reader, err := NewTextReader(strings.NewReader(article))
		require.NoError(t, err)

		docs, err := reader.Read(ctx)

		require.NoError(t, err)
		require.Len(t, docs, 1)
		assert.Equal(t, article, docs[0].Text)
	})
}

// TestTextReader_Comparison tests comparison with JSONReader
func TestTextReader_Comparison(t *testing.T) {
	ctx := context.Background()

	t.Run("text reader vs json reader behavior", func(t *testing.T) {
		jsonData := `{"key":"value"}`

		// TextReader treats JSON as plain text
		textReader, err := NewTextReader(strings.NewReader(jsonData))
		require.NoError(t, err)

		textDocs, err := textReader.Read(ctx)
		require.NoError(t, err)
		require.Len(t, textDocs, 1)
		assert.Equal(t, jsonData, textDocs[0].Text)

		// JSONReader would parse it as JSON
		jsonReader, err := NewJSONReader(strings.NewReader(jsonData))
		require.NoError(t, err)

		jsonDocs, err := jsonReader.Read(ctx)
		require.NoError(t, err)
		require.Len(t, jsonDocs, 1)
		assert.Equal(t, jsonData, jsonDocs[0].Text)
	})

	t.Run("text reader handles invalid JSON gracefully", func(t *testing.T) {
		invalidJSON := `{invalid json}`

		// TextReader doesn't care about JSON validity
		textReader, err := NewTextReader(strings.NewReader(invalidJSON))
		require.NoError(t, err)

		docs, err := textReader.Read(ctx)
		require.NoError(t, err)
		require.Len(t, docs, 1)
		assert.Equal(t, invalidJSON, docs[0].Text)

		// JSONReader would fail
		jsonReader, err := NewJSONReader(strings.NewReader(invalidJSON))
		require.NoError(t, err)

		_, err = jsonReader.Read(ctx)
		require.Error(t, err)
	})
}

// BenchmarkTextReader benchmarks text reading
func BenchmarkTextReader_ReadSmall(b *testing.B) {
	text := "Small text content"
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		reader, _ := NewTextReader(strings.NewReader(text))
		_, _ = reader.Read(ctx)
	}
}

func BenchmarkTextReader_ReadMedium(b *testing.B) {
	text := strings.Repeat("Medium text content. ", 100)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		reader, _ := NewTextReader(strings.NewReader(text))
		_, _ = reader.Read(ctx)
	}
}

func BenchmarkTextReader_ReadLarge(b *testing.B) {
	text := strings.Repeat("Large text content. ", 10000)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		reader, _ := NewTextReader(strings.NewReader(text))
		_, _ = reader.Read(ctx)
	}
}

func BenchmarkTextReader_BufferSizes(b *testing.B) {
	text := strings.Repeat("Test content. ", 1000)
	ctx := context.Background()

	bufferSizes := []int{128, 1024, 4096, 8192, 16384}

	for _, size := range bufferSizes {
		b.Run(string(rune(size)), func(b *testing.B) {
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				reader, _ := NewTextReader(strings.NewReader(text), size)
				_, _ = reader.Read(ctx)
			}
		})
	}
}

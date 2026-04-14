package document

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewJSONReader tests the constructor
func TestNewJSONReader(t *testing.T) {
	t.Run("with default buffer size", func(t *testing.T) {
		reader := strings.NewReader("test")
		jsonReader, err := NewJSONReader(reader)

		require.NoError(t, err)
		require.NotNil(t, jsonReader)
		assert.Equal(t, reader, jsonReader.reader)
		assert.Equal(t, 8192, jsonReader.bufferSize) // default
	})

	t.Run("with custom buffer size", func(t *testing.T) {
		reader := strings.NewReader("test")
		jsonReader, err := NewJSONReader(reader, 4096)

		require.NoError(t, err)
		require.NotNil(t, jsonReader)
		assert.Equal(t, 4096, jsonReader.bufferSize)
	})

	t.Run("with zero buffer size uses default", func(t *testing.T) {
		reader := strings.NewReader("test")
		jsonReader, err := NewJSONReader(reader, 0)

		require.NoError(t, err)
		require.NotNil(t, jsonReader)
		assert.Equal(t, 8192, jsonReader.bufferSize)
	})

	t.Run("with negative buffer size uses default", func(t *testing.T) {
		reader := strings.NewReader("test")
		jsonReader, err := NewJSONReader(reader, -100)

		require.NoError(t, err)
		require.NotNil(t, jsonReader)
		assert.Equal(t, 8192, jsonReader.bufferSize)
	})

	t.Run("with multiple sizes uses first", func(t *testing.T) {
		reader := strings.NewReader("test")
		jsonReader, err := NewJSONReader(reader, 2048, 4096, 8192)

		require.NoError(t, err)
		require.NotNil(t, jsonReader)
		assert.Equal(t, 2048, jsonReader.bufferSize)
	})

	t.Run("with no sizes uses default", func(t *testing.T) {
		reader := strings.NewReader("test")
		jsonReader, err := NewJSONReader(reader)

		require.NoError(t, err)
		require.NotNil(t, jsonReader)
		assert.Equal(t, 8192, jsonReader.bufferSize)
	})

	t.Run("with nil reader returns error", func(t *testing.T) {
		jsonReader, err := NewJSONReader(nil)

		require.Error(t, err)
		assert.Nil(t, jsonReader)
		assert.Equal(t, "reader is nil", err.Error())
	})

	t.Run("with nil reader and custom buffer size returns error", func(t *testing.T) {
		jsonReader, err := NewJSONReader(nil, 4096)

		require.Error(t, err)
		assert.Nil(t, jsonReader)
		assert.Equal(t, "reader is nil", err.Error())
	})
}

// TestJSONReader_maybeJSONArray tests array detection
func TestJSONReader_maybeJSONArray(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{
			name:     "valid array",
			input:    "[1,2,3]",
			expected: true,
		},
		{
			name:     "empty array",
			input:    "[]",
			expected: true,
		},
		{
			name:     "array with spaces",
			input:    "  [1,2,3]  ",
			expected: true,
		},
		{
			name:     "array with newlines",
			input:    "\n[1,2,3]\n",
			expected: true,
		},
		{
			name:     "array with tabs",
			input:    "\t[1,2,3]\t",
			expected: true,
		},
		{
			name:     "object",
			input:    `{"key":"value"}`,
			expected: false,
		},
		{
			name:     "string",
			input:    `"test"`,
			expected: false,
		},
		{
			name:     "number",
			input:    "123",
			expected: false,
		},
		{
			name:     "boolean",
			input:    "true",
			expected: false,
		},
		{
			name:     "null",
			input:    "null",
			expected: false,
		},
		{
			name:     "empty string",
			input:    "",
			expected: false,
		},
		{
			name:     "single bracket",
			input:    "[",
			expected: false,
		},
		{
			name:     "malformed - starts with [",
			input:    "[1,2,3}",
			expected: false,
		},
		{
			name:     "malformed - ends with ]",
			input:    "{1,2,3]",
			expected: false,
		},
		{
			name:     "nested array",
			input:    "[[1,2],[3,4]]",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a valid reader to avoid nil error
			reader, err := NewJSONReader(strings.NewReader(""))
			require.NoError(t, err)

			result := reader.maybeJSONArray([]byte(tt.input))
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestJSONReader_parseAsArray tests array parsing
func TestJSONReader_parseAsArray(t *testing.T) {
	t.Run("parse number array", func(t *testing.T) {
		reader, err := NewJSONReader(strings.NewReader(""))
		require.NoError(t, err)
		data := []byte("[1,2,3]")

		docs, err := reader.parseAsArray(data)

		require.NoError(t, err)
		require.Len(t, docs, 3)
		assert.Equal(t, "1", docs[0].Text)
		assert.Equal(t, "2", docs[1].Text)
		assert.Equal(t, "3", docs[2].Text)
	})

	t.Run("parse string array", func(t *testing.T) {
		reader, err := NewJSONReader(strings.NewReader(""))
		require.NoError(t, err)
		data := []byte(`["string","test"]`)

		docs, err := reader.parseAsArray(data)

		require.NoError(t, err)
		require.Len(t, docs, 2)
		assert.Equal(t, `"string"`, docs[0].Text)
		assert.Equal(t, `"test"`, docs[1].Text)
	})

	t.Run("parse object array", func(t *testing.T) {
		reader, err := NewJSONReader(strings.NewReader(""))
		require.NoError(t, err)
		data := []byte(`[{"name":"Alice"},{"name":"Bob"}]`)

		docs, err := reader.parseAsArray(data)

		require.NoError(t, err)
		require.Len(t, docs, 2)
		assert.Contains(t, docs[0].Text, "Alice")
		assert.Contains(t, docs[1].Text, "Bob")
	})

	t.Run("parse mixed array", func(t *testing.T) {
		reader, err := NewJSONReader(strings.NewReader(""))
		require.NoError(t, err)
		data := []byte(`[1,"string",true,null,{"key":"value"}]`)

		docs, err := reader.parseAsArray(data)

		require.NoError(t, err)
		require.Len(t, docs, 5)
		assert.Equal(t, "1", docs[0].Text)
		assert.Equal(t, `"string"`, docs[1].Text)
		assert.Equal(t, "true", docs[2].Text)
		assert.Equal(t, "null", docs[3].Text)
		assert.Contains(t, docs[4].Text, "key")
	})

	t.Run("parse empty array", func(t *testing.T) {
		reader, err := NewJSONReader(strings.NewReader(""))
		require.NoError(t, err)
		data := []byte("[]")

		docs, err := reader.parseAsArray(data)

		require.NoError(t, err)
		assert.Empty(t, docs)
	})

	t.Run("parse nested array", func(t *testing.T) {
		reader, err := NewJSONReader(strings.NewReader(""))
		require.NoError(t, err)
		data := []byte(`[[1,2],[3,4]]`)

		docs, err := reader.parseAsArray(data)

		require.NoError(t, err)
		require.Len(t, docs, 2)
		assert.Contains(t, docs[0].Text, "[1,2]")
		assert.Contains(t, docs[1].Text, "[3,4]")
	})

	t.Run("invalid JSON array", func(t *testing.T) {
		reader, err := NewJSONReader(strings.NewReader(""))
		require.NoError(t, err)
		data := []byte("[1,2,")

		docs, err := reader.parseAsArray(data)

		require.Error(t, err)
		assert.Nil(t, docs)
	})

	t.Run("not an array", func(t *testing.T) {
		reader, err := NewJSONReader(strings.NewReader(""))
		require.NoError(t, err)
		data := []byte(`{"key":"value"}`)

		docs, err := reader.parseAsArray(data)

		require.Error(t, err)
		assert.Nil(t, docs)
	})
}

// TestJSONReader_Read tests the main Read method
func TestJSONReader_Read(t *testing.T) {
	ctx := context.Background()

	t.Run("read number array", func(t *testing.T) {
		reader, err := NewJSONReader(strings.NewReader("[1,2,3]"))
		require.NoError(t, err)

		docs, err := reader.Read(ctx)

		require.NoError(t, err)
		require.Len(t, docs, 3)
		assert.Equal(t, "1", docs[0].Text)
		assert.Equal(t, "2", docs[1].Text)
		assert.Equal(t, "3", docs[2].Text)
	})

	t.Run("read string array", func(t *testing.T) {
		reader, err := NewJSONReader(strings.NewReader(`["string","test"]`))
		require.NoError(t, err)

		docs, err := reader.Read(ctx)

		require.NoError(t, err)
		require.Len(t, docs, 2)
		assert.Equal(t, `"string"`, docs[0].Text)
		assert.Equal(t, `"test"`, docs[1].Text)
	})

	t.Run("read single object", func(t *testing.T) {
		reader, err := NewJSONReader(strings.NewReader(`{"key":"value"}`))
		require.NoError(t, err)

		docs, err := reader.Read(ctx)

		require.NoError(t, err)
		require.Len(t, docs, 1)
		assert.Equal(t, `{"key":"value"}`, docs[0].Text)
	})

	t.Run("read single string", func(t *testing.T) {
		reader, err := NewJSONReader(strings.NewReader(`"hello"`))
		require.NoError(t, err)

		docs, err := reader.Read(ctx)

		require.NoError(t, err)
		require.Len(t, docs, 1)
		assert.Equal(t, `"hello"`, docs[0].Text)
	})

	t.Run("read single number", func(t *testing.T) {
		reader, err := NewJSONReader(strings.NewReader("42"))
		require.NoError(t, err)

		docs, err := reader.Read(ctx)

		require.NoError(t, err)
		require.Len(t, docs, 1)
		assert.Equal(t, "42", docs[0].Text)
	})

	t.Run("read boolean true", func(t *testing.T) {
		reader, err := NewJSONReader(strings.NewReader("true"))
		require.NoError(t, err)

		docs, err := reader.Read(ctx)

		require.NoError(t, err)
		require.Len(t, docs, 1)
		assert.Equal(t, "true", docs[0].Text)
	})

	t.Run("read boolean false", func(t *testing.T) {
		reader, err := NewJSONReader(strings.NewReader("false"))
		require.NoError(t, err)

		docs, err := reader.Read(ctx)

		require.NoError(t, err)
		require.Len(t, docs, 1)
		assert.Equal(t, "false", docs[0].Text)
	})

	t.Run("read null", func(t *testing.T) {
		reader, err := NewJSONReader(strings.NewReader("null"))
		require.NoError(t, err)

		docs, err := reader.Read(ctx)

		require.NoError(t, err)
		require.Len(t, docs, 1)
		assert.Equal(t, "null", docs[0].Text)
	})

	t.Run("read empty array", func(t *testing.T) {
		reader, err := NewJSONReader(strings.NewReader("[]"))
		require.NoError(t, err)

		docs, err := reader.Read(ctx)

		require.NoError(t, err)
		assert.Empty(t, docs)
	})

	t.Run("read array with spaces", func(t *testing.T) {
		reader, err := NewJSONReader(strings.NewReader("  [1,2,3]  "))
		require.NoError(t, err)

		docs, err := reader.Read(ctx)

		require.NoError(t, err)
		require.Len(t, docs, 3)
	})

	t.Run("read multiline JSON", func(t *testing.T) {
		json := `{
			"name": "Alice",
			"age": 30,
			"active": true
		}`
		reader, err := NewJSONReader(strings.NewReader(json))
		require.NoError(t, err)

		docs, err := reader.Read(ctx)

		require.NoError(t, err)
		require.Len(t, docs, 1)
		assert.Contains(t, docs[0].Text, "Alice")
		assert.Contains(t, docs[0].Text, "30")
	})

	t.Run("read nested structures", func(t *testing.T) {
		json := `{
			"user": {
				"name": "Bob",
				"address": {
					"city": "NYC"
				}
			}
		}`
		reader, err := NewJSONReader(strings.NewReader(json))
		require.NoError(t, err)

		docs, err := reader.Read(ctx)

		require.NoError(t, err)
		require.Len(t, docs, 1)
		assert.Contains(t, docs[0].Text, "Bob")
		assert.Contains(t, docs[0].Text, "NYC")
	})

	t.Run("read complex array", func(t *testing.T) {
		json := `[
			{"id":1,"name":"Alice"},
			{"id":2,"name":"Bob"},
			{"id":3,"name":"Charlie"}
		]`
		reader, err := NewJSONReader(strings.NewReader(json))
		require.NoError(t, err)

		docs, err := reader.Read(ctx)

		require.NoError(t, err)
		require.Len(t, docs, 3)
		assert.Contains(t, docs[0].Text, "Alice")
		assert.Contains(t, docs[1].Text, "Bob")
		assert.Contains(t, docs[2].Text, "Charlie")
	})

	t.Run("invalid JSON", func(t *testing.T) {
		reader, err := NewJSONReader(strings.NewReader("{invalid json}"))
		require.NoError(t, err)

		docs, err := reader.Read(ctx)

		require.Error(t, err)
		assert.Nil(t, docs)
	})

	t.Run("empty input", func(t *testing.T) {
		reader, err := NewJSONReader(strings.NewReader(""))
		require.NoError(t, err)

		docs, err := reader.Read(ctx)

		require.Error(t, err)
		assert.Nil(t, docs)
	})

	t.Run("malformed array - missing bracket", func(t *testing.T) {
		reader, err := NewJSONReader(strings.NewReader("[1,2,3"))
		require.NoError(t, err)

		docs, err := reader.Read(ctx)

		require.Error(t, err)
		assert.Nil(t, docs)
	})

	t.Run("looks like array but invalid - fallback to single", func(t *testing.T) {
		reader, err := NewJSONReader(strings.NewReader("[invalid]"))
		require.NoError(t, err)

		docs, err := reader.Read(ctx)

		require.Error(t, err)
		assert.Nil(t, docs)
	})

	t.Run("with custom buffer size", func(t *testing.T) {
		reader, err := NewJSONReader(strings.NewReader("[1,2,3]"), 1024)
		require.NoError(t, err)

		docs, err := reader.Read(ctx)

		require.NoError(t, err)
		require.Len(t, docs, 3)
	})

	t.Run("context cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		reader, err := NewJSONReader(strings.NewReader("[1,2,3]"))
		require.NoError(t, err)

		docs, err := reader.Read(ctx)

		require.NoError(t, err)
		require.Len(t, docs, 3)
	})
}

// TestJSONReader_ReadEdgeCases tests edge cases
func TestJSONReader_ReadEdgeCases(t *testing.T) {
	ctx := context.Background()

	t.Run("very large array", func(t *testing.T) {
		elements := make([]string, 1000)
		for i := range elements {
			elements[i] = `"item"`
		}
		json := "[" + strings.Join(elements, ",") + "]"

		reader, err := NewJSONReader(strings.NewReader(json))
		require.NoError(t, err)

		docs, err := reader.Read(ctx)

		require.NoError(t, err)
		assert.Len(t, docs, 1000)
	})

	t.Run("deeply nested object", func(t *testing.T) {
		json := `{
			"level1": {
				"level2": {
					"level3": {
						"level4": {
							"value": "deep"
						}
					}
				}
			}
		}`
		reader, err := NewJSONReader(strings.NewReader(json))
		require.NoError(t, err)

		docs, err := reader.Read(ctx)

		require.NoError(t, err)
		require.Len(t, docs, 1)
		assert.Contains(t, docs[0].Text, "deep")
	})

	t.Run("special characters in strings", func(t *testing.T) {
		json := `["hello\nworld","tab\there","quote\"here"]`
		reader, err := NewJSONReader(strings.NewReader(json))
		require.NoError(t, err)

		docs, err := reader.Read(ctx)

		require.NoError(t, err)
		require.Len(t, docs, 3)
	})

	t.Run("unicode characters", func(t *testing.T) {
		json := `["你好","🎉","Ñoño"]`
		reader, err := NewJSONReader(strings.NewReader(json))
		require.NoError(t, err)

		docs, err := reader.Read(ctx)

		require.NoError(t, err)
		require.Len(t, docs, 3)
		assert.Contains(t, docs[0].Text, "你好")
	})

	t.Run("numeric edge cases", func(t *testing.T) {
		json := `[0,-1,1.5,-2.5,1e10,1.23e-10]`
		reader, err := NewJSONReader(strings.NewReader(json))
		require.NoError(t, err)

		docs, err := reader.Read(ctx)

		require.NoError(t, err)
		assert.Len(t, docs, 6)
	})
}

// TestJSONReader_InterfaceCompliance verifies interface implementation
func TestJSONReader_InterfaceCompliance(t *testing.T) {
	reader, err := NewJSONReader(strings.NewReader("[]"))
	require.NoError(t, err)
	var _ Reader = reader
}

// TestJSONReader_Integration tests complete workflows
func TestJSONReader_Integration(t *testing.T) {
	ctx := context.Background()

	t.Run("read document collection", func(t *testing.T) {
		json := `[
			{
				"id": "doc1",
				"title": "First Document",
				"content": "This is the first document"
			},
			{
				"id": "doc2",
				"title": "Second Document",
				"content": "This is the second document"
			}
		]`

		reader, err := NewJSONReader(strings.NewReader(json))
		require.NoError(t, err)

		docs, err := reader.Read(ctx)

		require.NoError(t, err)
		require.Len(t, docs, 2)

		assert.Contains(t, docs[0].Text, "doc1")
		assert.Contains(t, docs[0].Text, "First Document")
		assert.Contains(t, docs[1].Text, "doc2")
		assert.Contains(t, docs[1].Text, "Second Document")
	})

	t.Run("read configuration data", func(t *testing.T) {
		json := `{
			"app_name": "TestApp",
			"version": "1.0.0",
			"settings": {
				"debug": true,
				"timeout": 30
			}
		}`

		reader, err := NewJSONReader(strings.NewReader(json))
		require.NoError(t, err)

		docs, err := reader.Read(ctx)

		require.NoError(t, err)
		require.Len(t, docs, 1)

		config := docs[0].Text
		assert.Contains(t, config, "TestApp")
		assert.Contains(t, config, "1.0.0")
		assert.Contains(t, config, "debug")
	})

	t.Run("read API response array", func(t *testing.T) {
		json := `[
			{"status":"success","code":200},
			{"status":"error","code":404},
			{"status":"success","code":201}
		]`

		reader, err := NewJSONReader(strings.NewReader(json))
		require.NoError(t, err)

		docs, err := reader.Read(ctx)

		require.NoError(t, err)
		require.Len(t, docs, 3)

		successCount := 0
		for _, doc := range docs {
			if strings.Contains(doc.Text, "success") {
				successCount++
			}
		}
		assert.Equal(t, 2, successCount)
	})

	t.Run("read from different buffer sizes", func(t *testing.T) {
		json := `[1,2,3,4,5]`

		bufferSizes := []int{128, 1024, 4096, 8192}

		for _, size := range bufferSizes {
			t.Run(string(rune(size)), func(t *testing.T) {
				reader, err := NewJSONReader(strings.NewReader(json), size)
				require.NoError(t, err)

				docs, err := reader.Read(ctx)

				require.NoError(t, err)
				assert.Len(t, docs, 5)
			})
		}
	})
}

// TestJSONReader_ErrorHandling tests error scenarios
func TestJSONReader_ErrorHandling(t *testing.T) {
	ctx := context.Background()

	t.Run("reader error", func(t *testing.T) {
		errorReader := &errorReader{err: io.ErrUnexpectedEOF}
		reader, err := NewJSONReader(errorReader)
		require.NoError(t, err)

		docs, err := reader.Read(ctx)

		require.Error(t, err)
		assert.Nil(t, docs)
	})

	t.Run("partial JSON", func(t *testing.T) {
		reader, err := NewJSONReader(strings.NewReader(`{"incomplete":`))
		require.NoError(t, err)

		docs, err := reader.Read(ctx)

		require.Error(t, err)
		assert.Nil(t, docs)
	})

	t.Run("mixed valid and invalid in array", func(t *testing.T) {
		reader, err := NewJSONReader(strings.NewReader(`[1,invalid,3]`))
		require.NoError(t, err)

		docs, err := reader.Read(ctx)

		require.Error(t, err)
		assert.Nil(t, docs)
	})
}

// errorReader is a helper for testing reader errors
type errorReader struct {
	err error
}

func (e *errorReader) Read(p []byte) (n int, err error) {
	return 0, e.err
}

// BenchmarkJSONReader benchmarks JSON reading
func BenchmarkJSONReader_ReadArray(b *testing.B) {
	json := `[1,2,3,4,5,6,7,8,9,10]`
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		reader, _ := NewJSONReader(strings.NewReader(json))
		_, _ = reader.Read(ctx)
	}
}

func BenchmarkJSONReader_ReadObject(b *testing.B) {
	json := `{"key1":"value1","key2":"value2","key3":"value3"}`
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		reader, _ := NewJSONReader(strings.NewReader(json))
		_, _ = reader.Read(ctx)
	}
}

func BenchmarkJSONReader_ReadLargeArray(b *testing.B) {
	elements := make([]string, 1000)
	for i := range elements {
		elements[i] = `"item"`
	}
	json := "[" + strings.Join(elements, ",") + "]"
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		reader, _ := NewJSONReader(strings.NewReader(json))
		_, _ = reader.Read(ctx)
	}
}

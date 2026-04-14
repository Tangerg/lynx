package id

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewSha256Generator tests the constructor
func TestNewSha256Generator(t *testing.T) {
	t.Run("with nil salt", func(t *testing.T) {
		generator := NewSha256Generator(nil)

		require.NotNil(t, generator)
		assert.Nil(t, generator.salt)
	})

	t.Run("with empty salt", func(t *testing.T) {
		generator := NewSha256Generator([]byte{})

		require.NotNil(t, generator)
		assert.Empty(t, generator.salt)
	})

	t.Run("with salt", func(t *testing.T) {
		salt := []byte("my-salt")
		generator := NewSha256Generator(salt)

		require.NotNil(t, generator)
		assert.Equal(t, salt, generator.salt)
	})
}

// TestSha256Generator_Generate tests the Generate method
func TestSha256Generator_Generate(t *testing.T) {
	t.Run("empty objects returns empty string", func(t *testing.T) {
		generator := NewSha256Generator(nil)
		ctx := context.Background()

		id, err := generator.Generate(ctx)

		require.NoError(t, err)
		assert.Empty(t, id)
	})

	t.Run("nil objects returns empty string", func(t *testing.T) {
		generator := NewSha256Generator(nil)
		ctx := context.Background()

		id, err := generator.Generate(ctx, nil)

		require.NoError(t, err)
		assert.NotEmpty(t, id) // nil is still marshaled as "null"
	})

	t.Run("single string object", func(t *testing.T) {
		generator := NewSha256Generator(nil)
		ctx := context.Background()

		id, err := generator.Generate(ctx, "test")

		require.NoError(t, err)
		assert.NotEmpty(t, id)
		assert.Len(t, id, 64) // SHA256 produces 32 bytes = 64 hex chars
	})

	t.Run("single integer object", func(t *testing.T) {
		generator := NewSha256Generator(nil)
		ctx := context.Background()

		id, err := generator.Generate(ctx, 42)

		require.NoError(t, err)
		assert.NotEmpty(t, id)
		assert.Len(t, id, 64)
	})

	t.Run("multiple objects", func(t *testing.T) {
		generator := NewSha256Generator(nil)
		ctx := context.Background()

		id, err := generator.Generate(ctx, "test", 123, true)

		require.NoError(t, err)
		assert.NotEmpty(t, id)
		assert.Len(t, id, 64)
	})

	t.Run("struct object", func(t *testing.T) {
		generator := NewSha256Generator(nil)
		ctx := context.Background()

		type TestStruct struct {
			Name   string
			Age    int
			Active bool
		}
		obj := TestStruct{Name: "Alice", Age: 30, Active: true}

		id, err := generator.Generate(ctx, obj)

		require.NoError(t, err)
		assert.NotEmpty(t, id)
		assert.Len(t, id, 64)
	})

	t.Run("map object", func(t *testing.T) {
		generator := NewSha256Generator(nil)
		ctx := context.Background()

		obj := map[string]any{
			"key1": "value1",
			"key2": 123,
		}

		id, err := generator.Generate(ctx, obj)

		require.NoError(t, err)
		assert.NotEmpty(t, id)
		assert.Len(t, id, 64)
	})

	t.Run("slice object", func(t *testing.T) {
		generator := NewSha256Generator(nil)
		ctx := context.Background()

		obj := []string{"a", "b", "c"}

		id, err := generator.Generate(ctx, obj)

		require.NoError(t, err)
		assert.NotEmpty(t, id)
		assert.Len(t, id, 64)
	})

	t.Run("with salt produces different hash", func(t *testing.T) {
		generator1 := NewSha256Generator(nil)
		generator2 := NewSha256Generator([]byte("salt"))
		ctx := context.Background()

		id1, err1 := generator1.Generate(ctx, "test")
		id2, err2 := generator2.Generate(ctx, "test")

		require.NoError(t, err1)
		require.NoError(t, err2)
		assert.NotEqual(t, id1, id2, "Salt should produce different hash")
	})

	t.Run("different salts produce different hashes", func(t *testing.T) {
		generator1 := NewSha256Generator([]byte("salt1"))
		generator2 := NewSha256Generator([]byte("salt2"))
		ctx := context.Background()

		id1, err1 := generator1.Generate(ctx, "test")
		id2, err2 := generator2.Generate(ctx, "test")

		require.NoError(t, err1)
		require.NoError(t, err2)
		assert.NotEqual(t, id1, id2, "Different salts should produce different hashes")
	})

	t.Run("same input produces same hash", func(t *testing.T) {
		generator := NewSha256Generator([]byte("salt"))
		ctx := context.Background()

		id1, err1 := generator.Generate(ctx, "test", 123)
		id2, err2 := generator.Generate(ctx, "test", 123)

		require.NoError(t, err1)
		require.NoError(t, err2)
		assert.Equal(t, id1, id2, "Same input should produce same hash")
	})

	t.Run("order matters", func(t *testing.T) {
		generator := NewSha256Generator(nil)
		ctx := context.Background()

		id1, err1 := generator.Generate(ctx, "a", "b")
		id2, err2 := generator.Generate(ctx, "b", "a")

		require.NoError(t, err1)
		require.NoError(t, err2)
		assert.NotEqual(t, id1, id2, "Different order should produce different hash")
	})

	t.Run("unmarshalable object is skipped", func(t *testing.T) {
		generator := NewSha256Generator(nil)
		ctx := context.Background()

		// Channel cannot be marshaled
		ch := make(chan int)
		id, err := generator.Generate(ctx, "valid", ch, "another")

		require.NoError(t, err)
		// Should still generate a hash from valid objects
		assert.NotEmpty(t, id)
		assert.Len(t, id, 64)
	})

	t.Run("only unmarshalable objects", func(t *testing.T) {
		generator := NewSha256Generator(nil)
		ctx := context.Background()

		// Channel cannot be marshaled
		ch := make(chan int)
		id, err := generator.Generate(ctx, ch)

		require.NoError(t, err)
		// Since no data was written to hasher, should return hash of salt only
		// With nil salt, this should be hash of empty data
		assert.NotEmpty(t, id)
		assert.Len(t, id, 64)
	})

	t.Run("context cancellation does not affect generation", func(t *testing.T) {
		generator := NewSha256Generator(nil)
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		id, err := generator.Generate(ctx, "test")

		require.NoError(t, err)
		assert.NotEmpty(t, id)
	})
}

// TestSha256Generator_Deterministic tests deterministic behavior
func TestSha256Generator_Deterministic(t *testing.T) {
	t.Run("deterministic for strings", func(t *testing.T) {
		generator := NewSha256Generator([]byte("test-salt"))
		ctx := context.Background()

		results := make([]string, 10)
		for i := 0; i < 10; i++ {
			id, err := generator.Generate(ctx, "same-string")
			require.NoError(t, err)
			results[i] = id
		}

		// All results should be identical
		for i := 1; i < len(results); i++ {
			assert.Equal(t, results[0], results[i])
		}
	})

	t.Run("deterministic for complex objects", func(t *testing.T) {
		generator := NewSha256Generator([]byte("salt"))
		ctx := context.Background()

		type ComplexObject struct {
			ID       int
			Name     string
			Tags     []string
			Metadata map[string]string
		}

		obj := ComplexObject{
			ID:   1,
			Name: "test",
			Tags: []string{"tag1", "tag2"},
			Metadata: map[string]string{
				"key1": "value1",
			},
		}

		results := make([]string, 5)
		for i := 0; i < 5; i++ {
			id, err := generator.Generate(ctx, obj)
			require.NoError(t, err)
			results[i] = id
		}

		// All results should be identical
		for i := 1; i < len(results); i++ {
			assert.Equal(t, results[0], results[i])
		}
	})
}

// TestSha256Generator_HashFormat tests hash output format
func TestSha256Generator_HashFormat(t *testing.T) {
	t.Run("output is valid hex", func(t *testing.T) {
		generator := NewSha256Generator(nil)
		ctx := context.Background()

		id, err := generator.Generate(ctx, "test")
		require.NoError(t, err)

		// Should be valid hex string
		_, hexErr := hex.DecodeString(id)
		assert.NoError(t, hexErr)
	})

	t.Run("output length is 64 characters", func(t *testing.T) {
		generator := NewSha256Generator(nil)
		ctx := context.Background()

		testCases := []any{
			"short",
			"a very long string that is much longer than the hash output",
			123,
			map[string]any{"key": "value"},
		}

		for _, tc := range testCases {
			id, err := generator.Generate(ctx, tc)
			require.NoError(t, err)
			assert.Len(t, id, 64, "SHA256 hash should always be 64 hex characters")
		}
	})
}

// TestSha256Generator_EdgeCases tests edge cases
func TestSha256Generator_EdgeCases(t *testing.T) {
	t.Run("empty string vs no input", func(t *testing.T) {
		generator := NewSha256Generator(nil)
		ctx := context.Background()

		idEmpty, err1 := generator.Generate(ctx, "")
		idNone, err2 := generator.Generate(ctx)

		require.NoError(t, err1)
		require.NoError(t, err2)
		assert.NotEqual(t, idEmpty, idNone)
		assert.Empty(t, idNone)
		assert.NotEmpty(t, idEmpty)
	})

	t.Run("zero values", func(t *testing.T) {
		generator := NewSha256Generator(nil)
		ctx := context.Background()

		testCases := []struct {
			name  string
			value any
		}{
			{"zero int", 0},
			{"empty string", ""},
			{"false bool", false},
			{"empty slice", []string{}},
			{"empty map", map[string]any{}},
			{"nil", nil},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				id, err := generator.Generate(ctx, tc.value)
				require.NoError(t, err)
				assert.NotEmpty(t, id)
				assert.Len(t, id, 64)
			})
		}
	})

	t.Run("large input", func(t *testing.T) {
		generator := NewSha256Generator(nil)
		ctx := context.Background()

		// Create a large slice
		largeSlice := make([]int, 10000)
		for i := range largeSlice {
			largeSlice[i] = i
		}

		id, err := generator.Generate(ctx, largeSlice)

		require.NoError(t, err)
		assert.NotEmpty(t, id)
		assert.Len(t, id, 64)
	})

	t.Run("special characters", func(t *testing.T) {
		generator := NewSha256Generator(nil)
		ctx := context.Background()

		specialStrings := []string{
			"hello\nworld",
			"tab\there",
			"unicode: 你好",
			"emoji: 😀🎉",
			"null\x00byte",
		}

		for _, str := range specialStrings {
			id, err := generator.Generate(ctx, str)
			require.NoError(t, err)
			assert.NotEmpty(t, id)
			assert.Len(t, id, 64)
		}
	})
}

// TestSha256Generator_InterfaceCompliance verifies interface implementation
func TestSha256Generator_InterfaceCompliance(t *testing.T) {
	generator := NewSha256Generator(nil)
	var _ Generator = generator
}

// TestSha256Generator_Integration tests complete workflows
func TestSha256Generator_Integration(t *testing.T) {
	t.Run("document-like structure hashing", func(t *testing.T) {
		generator := NewSha256Generator([]byte("doc-salt"))
		ctx := context.Background()

		type Document struct {
			Title    string
			Content  string
			Author   string
			Tags     []string
			Metadata map[string]any
		}

		doc := Document{
			Title:   "Test Document",
			Content: "This is the content of the document.",
			Author:  "Alice",
			Tags:    []string{"test", "example"},
			Metadata: map[string]any{
				"created": "2025-01-01",
				"version": 1,
			},
		}

		// Generate ID multiple times
		id1, err1 := generator.Generate(ctx, doc)
		id2, err2 := generator.Generate(ctx, doc)

		require.NoError(t, err1)
		require.NoError(t, err2)
		assert.Equal(t, id1, id2, "Same document should produce same ID")

		// Modify document
		doc.Content = "Modified content"
		id3, err3 := generator.Generate(ctx, doc)

		require.NoError(t, err3)
		assert.NotEqual(t, id1, id3, "Modified document should produce different ID")
	})

	t.Run("multi-field hashing", func(t *testing.T) {
		generator := NewSha256Generator(nil)
		ctx := context.Background()

		// Simulate hashing multiple fields together
		title := "My Title"
		content := "My Content"
		author := "John Doe"

		id, err := generator.Generate(ctx, title, content, author)

		require.NoError(t, err)
		assert.NotEmpty(t, id)

		// Changing one field should change hash
		id2, err2 := generator.Generate(ctx, title, "Different Content", author)
		require.NoError(t, err2)
		assert.NotEqual(t, id, id2)
	})

	t.Run("using as cache key generator", func(t *testing.T) {
		generator := NewSha256Generator([]byte("cache-salt"))
		ctx := context.Background()

		// Simulate cache key generation from query parameters
		params := map[string]any{
			"query":  "search term",
			"page":   1,
			"limit":  10,
			"filter": []string{"active", "verified"},
		}

		cacheKey, err := generator.Generate(ctx, params)
		require.NoError(t, err)

		// Same params should give same key
		cacheKey2, err2 := generator.Generate(ctx, params)
		require.NoError(t, err2)
		assert.Equal(t, cacheKey, cacheKey2)

		// Different params should give different key
		params["page"] = 2
		cacheKey3, err3 := generator.Generate(ctx, params)
		require.NoError(t, err3)
		assert.NotEqual(t, cacheKey, cacheKey3)
	})

	t.Run("deduplication use case", func(t *testing.T) {
		generator := NewSha256Generator(nil)
		ctx := context.Background()

		// Simulate checking for duplicate documents
		documents := []string{
			"Document A content",
			"Document B content",
			"Document A content", // Duplicate
			"Document C content",
		}

		seenIDs := make(map[string]bool)
		duplicates := 0

		for _, doc := range documents {
			id, err := generator.Generate(ctx, doc)
			require.NoError(t, err)

			if seenIDs[id] {
				duplicates++
			} else {
				seenIDs[id] = true
			}
		}

		assert.Equal(t, 1, duplicates, "Should detect one duplicate")
		assert.Len(t, seenIDs, 3, "Should have 3 unique documents")
	})
}

// TestSha256Generator_ExpectedHashes tests against known hash values
func TestSha256Generator_ExpectedHashes(t *testing.T) {
	t.Run("known hash without salt", func(t *testing.T) {
		generator := NewSha256Generator(nil)
		ctx := context.Background()

		// "test" marshals to JSON as "test" (with quotes)
		// We can compute expected hash
		data, _ := json.Marshal("test")
		hasher := sha256.New()
		hasher.Write(data)
		expectedHash := hex.EncodeToString(hasher.Sum(nil))

		id, err := generator.Generate(ctx, "test")
		require.NoError(t, err)
		assert.Equal(t, expectedHash, id)
	})

	t.Run("known hash with salt", func(t *testing.T) {
		salt := []byte("my-salt")
		generator := NewSha256Generator(salt)
		ctx := context.Background()

		data, _ := json.Marshal("test")
		hasher := sha256.New()
		hasher.Write(data)
		expectedHash := hex.EncodeToString(hasher.Sum(salt))

		id, err := generator.Generate(ctx, "test")
		require.NoError(t, err)
		assert.Equal(t, expectedHash, id)
	})
}

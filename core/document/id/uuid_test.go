package id

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewUUIDGenerator tests the constructor
func TestNewUUIDGenerator(t *testing.T) {
	t.Run("creates valid generator", func(t *testing.T) {
		generator := NewUUIDGenerator()

		require.NotNil(t, generator)
	})

	t.Run("multiple instances are independent", func(t *testing.T) {
		gen1 := NewUUIDGenerator()
		gen2 := NewUUIDGenerator()

		require.NotNil(t, gen1)
		require.NotNil(t, gen2)
		// They are different instances
		assert.NotSame(t, gen1, gen2)
	})
}

// TestUUIDGenerator_Generate tests the Generate method
func TestUUIDGenerator_Generate(t *testing.T) {
	t.Run("generates valid UUID", func(t *testing.T) {
		generator := NewUUIDGenerator()
		ctx := context.Background()

		id, err := generator.Generate(ctx)

		require.NoError(t, err)
		assert.NotEmpty(t, id)

		// Verify it's a valid UUID
		parsed, parseErr := uuid.Parse(id)
		assert.NoError(t, parseErr)
		assert.NotEqual(t, uuid.Nil, parsed)
	})

	t.Run("generates unique UUIDs", func(t *testing.T) {
		generator := NewUUIDGenerator()
		ctx := context.Background()

		// Generate multiple UUIDs
		ids := make(map[string]bool)
		const count = 1000

		for i := 0; i < count; i++ {
			id, err := generator.Generate(ctx)
			require.NoError(t, err)

			// Check for duplicates
			assert.False(t, ids[id], "Generated duplicate UUID: %s", id)
			ids[id] = true
		}

		assert.Len(t, ids, count, "Should generate %d unique UUIDs", count)
	})

	t.Run("ignores input objects", func(t *testing.T) {
		generator := NewUUIDGenerator()
		ctx := context.Background()

		// Generate with different inputs
		id1, err1 := generator.Generate(ctx)
		id2, err2 := generator.Generate(ctx, "test")
		id3, err3 := generator.Generate(ctx, "test", 123)
		id4, err4 := generator.Generate(ctx, nil)

		require.NoError(t, err1)
		require.NoError(t, err2)
		require.NoError(t, err3)
		require.NoError(t, err4)

		// All should be different (inputs don't matter)
		assert.NotEqual(t, id1, id2)
		assert.NotEqual(t, id1, id3)
		assert.NotEqual(t, id1, id4)
		assert.NotEqual(t, id2, id3)
		assert.NotEqual(t, id2, id4)
		assert.NotEqual(t, id3, id4)
	})

	t.Run("same input produces different UUIDs", func(t *testing.T) {
		generator := NewUUIDGenerator()
		ctx := context.Background()

		input := "same-input"
		id1, err1 := generator.Generate(ctx, input)
		id2, err2 := generator.Generate(ctx, input)

		require.NoError(t, err1)
		require.NoError(t, err2)

		// UUIDs should be different even with same input
		assert.NotEqual(t, id1, id2, "UUIDs should be random, not deterministic")
	})

	t.Run("context cancellation does not affect generation", func(t *testing.T) {
		generator := NewUUIDGenerator()
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		id, err := generator.Generate(ctx)

		require.NoError(t, err)
		assert.NotEmpty(t, id)

		// Verify it's a valid UUID
		_, parseErr := uuid.Parse(id)
		assert.NoError(t, parseErr)
	})

	t.Run("nil context", func(t *testing.T) {
		generator := NewUUIDGenerator()

		// Should not panic with nil context
		id, err := generator.Generate(nil)

		require.NoError(t, err)
		assert.NotEmpty(t, id)

		_, parseErr := uuid.Parse(id)
		assert.NoError(t, parseErr)
	})
}

// TestUUIDGenerator_Format tests UUID format
func TestUUIDGenerator_Format(t *testing.T) {
	t.Run("UUID format is correct", func(t *testing.T) {
		generator := NewUUIDGenerator()
		ctx := context.Background()

		id, err := generator.Generate(ctx)
		require.NoError(t, err)

		// UUID format: xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
		// Length should be 36 characters (32 hex + 4 hyphens)
		assert.Len(t, id, 36)

		// Check hyphen positions
		assert.Equal(t, "-", string(id[8]))
		assert.Equal(t, "-", string(id[13]))
		assert.Equal(t, "-", string(id[18]))
		assert.Equal(t, "-", string(id[23]))
	})

	t.Run("UUID is lowercase", func(t *testing.T) {
		generator := NewUUIDGenerator()
		ctx := context.Background()

		id, err := generator.Generate(ctx)
		require.NoError(t, err)

		// UUID from google/uuid is lowercase
		assert.Equal(t, id, id) // Just checking it doesn't panic

		// Parse and verify
		parsed, parseErr := uuid.Parse(id)
		require.NoError(t, parseErr)
		assert.Equal(t, id, parsed.String())
	})

	t.Run("UUID version 4", func(t *testing.T) {
		generator := NewUUIDGenerator()
		ctx := context.Background()

		id, err := generator.Generate(ctx)
		require.NoError(t, err)

		_, parseErr := uuid.Parse(id)
		require.NoError(t, parseErr)

		// Google's uuid.New() generates version 4 UUIDs
		// Version 4 UUIDs have '4' in the version field
		// Position 14 (0-indexed) should be '4'
		assert.Equal(t, byte('4'), id[14], "Should be UUID version 4")
	})
}

// TestUUIDGenerator_Concurrency tests concurrent usage
func TestUUIDGenerator_Concurrency(t *testing.T) {
	t.Run("concurrent generation", func(t *testing.T) {
		generator := NewUUIDGenerator()
		ctx := context.Background()

		const goroutines = 100
		const idsPerGoroutine = 100

		idsChan := make(chan string, goroutines*idsPerGoroutine)
		errChan := make(chan error, goroutines)

		// Generate UUIDs concurrently
		for i := 0; i < goroutines; i++ {
			go func() {
				for j := 0; j < idsPerGoroutine; j++ {
					id, err := generator.Generate(ctx)
					if err != nil {
						errChan <- err
						return
					}
					idsChan <- id
				}
			}()
		}

		// Collect results
		ids := make(map[string]bool)
		for i := 0; i < goroutines*idsPerGoroutine; i++ {
			select {
			case id := <-idsChan:
				assert.False(t, ids[id], "Duplicate UUID in concurrent generation")
				ids[id] = true
			case err := <-errChan:
				t.Fatalf("Error in concurrent generation: %v", err)
			}
		}

		assert.Len(t, ids, goroutines*idsPerGoroutine, "Should generate all unique UUIDs")
	})

	t.Run("multiple generators concurrent", func(t *testing.T) {
		const generators = 10
		const idsPerGenerator = 100

		ctx := context.Background()
		idsChan := make(chan string, generators*idsPerGenerator)

		for i := 0; i < generators; i++ {
			go func() {
				gen := NewUUIDGenerator()
				for j := 0; j < idsPerGenerator; j++ {
					id, err := gen.Generate(ctx)
					if err != nil {
						t.Errorf("Error generating UUID: %v", err)
						return
					}
					idsChan <- id
				}
			}()
		}

		// Collect and verify uniqueness
		ids := make(map[string]bool)
		for i := 0; i < generators*idsPerGenerator; i++ {
			id := <-idsChan
			assert.False(t, ids[id], "Duplicate UUID across generators")
			ids[id] = true
		}

		assert.Len(t, ids, generators*idsPerGenerator)
	})
}

// TestUUIDGenerator_InterfaceCompliance verifies interface implementation
func TestUUIDGenerator_InterfaceCompliance(t *testing.T) {
	generator := NewUUIDGenerator()
	var _ Generator = generator
}

// TestUUIDGenerator_EdgeCases tests edge cases
func TestUUIDGenerator_EdgeCases(t *testing.T) {
	t.Run("generate with various object types", func(t *testing.T) {
		generator := NewUUIDGenerator()
		ctx := context.Background()

		testCases := []struct {
			name    string
			objects []any
		}{
			{"nil", []any{nil}},
			{"string", []any{"test"}},
			{"int", []any{123}},
			{"struct", []any{struct{ Name string }{Name: "test"}}},
			{"map", []any{map[string]any{"key": "value"}}},
			{"slice", []any{[]int{1, 2, 3}}},
			{"multiple", []any{"test", 123, true}},
			{"empty", []any{}},
			{"channel", []any{make(chan int)}},
		}

		ids := make(map[string]bool)
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				id, err := generator.Generate(ctx, tc.objects...)
				require.NoError(t, err)
				assert.NotEmpty(t, id)

				// Verify valid UUID
				_, parseErr := uuid.Parse(id)
				assert.NoError(t, parseErr)

				// Verify uniqueness
				assert.False(t, ids[id], "Generated duplicate UUID for case: %s", tc.name)
				ids[id] = true
			})
		}
	})

	t.Run("rapid generation", func(t *testing.T) {
		generator := NewUUIDGenerator()
		ctx := context.Background()

		ids := make(map[string]bool)
		const count = 10000

		for i := 0; i < count; i++ {
			id, err := generator.Generate(ctx)
			require.NoError(t, err)
			assert.False(t, ids[id], "Duplicate in rapid generation")
			ids[id] = true
		}

		assert.Len(t, ids, count)
	})
}

// TestUUIDGenerator_Integration tests complete workflows
func TestUUIDGenerator_Integration(t *testing.T) {
	t.Run("document ID generation", func(t *testing.T) {
		generator := NewUUIDGenerator()
		ctx := context.Background()

		// Simulate generating IDs for documents
		type Document struct {
			ID      string
			Title   string
			Content string
		}

		documents := make([]Document, 100)
		usedIDs := make(map[string]bool)

		for i := range documents {
			id, err := generator.Generate(ctx)
			require.NoError(t, err)

			documents[i] = Document{
				ID:      id,
				Title:   "Document " + id,
				Content: "Content",
			}

			// Verify uniqueness
			assert.False(t, usedIDs[id])
			usedIDs[id] = true

			// Verify valid UUID
			_, parseErr := uuid.Parse(id)
			assert.NoError(t, parseErr)
		}

		assert.Len(t, usedIDs, 100)
	})

	t.Run("comparison with SHA256Generator", func(t *testing.T) {
		uuidGen := NewUUIDGenerator()
		sha256Gen := NewSha256Generator(nil)
		ctx := context.Background()

		input := "same-input"

		// UUID should be different each time
		uuid1, err1 := uuidGen.Generate(ctx, input)
		uuid2, err2 := uuidGen.Generate(ctx, input)
		require.NoError(t, err1)
		require.NoError(t, err2)
		assert.NotEqual(t, uuid1, uuid2, "UUID is random")

		// SHA256 should be same each time
		sha1, err3 := sha256Gen.Generate(ctx, input)
		sha2, err4 := sha256Gen.Generate(ctx, input)
		require.NoError(t, err3)
		require.NoError(t, err4)
		assert.Equal(t, sha1, sha2, "SHA256 is deterministic")

		// Formats are different
		assert.Len(t, uuid1, 36) // UUID format
		assert.Len(t, sha1, 64)  // SHA256 hex format
	})

	t.Run("use as tracking ID", func(t *testing.T) {
		generator := NewUUIDGenerator()
		ctx := context.Background()

		// Simulate tracking IDs for operations
		type Operation struct {
			TrackingID string
			Action     string
			Timestamp  string
		}

		operations := make([]Operation, 50)
		trackingIDs := make(map[string]bool)

		for i := range operations {
			trackingID, err := generator.Generate(ctx)
			require.NoError(t, err)

			operations[i] = Operation{
				TrackingID: trackingID,
				Action:     "action",
				Timestamp:  "2025-01-01",
			}

			// Each operation should have unique tracking ID
			assert.False(t, trackingIDs[trackingID])
			trackingIDs[trackingID] = true
		}

		assert.Len(t, trackingIDs, 50)
	})

	t.Run("session ID generation", func(t *testing.T) {
		generator := NewUUIDGenerator()
		ctx := context.Background()

		// Simulate generating session IDs
		sessions := make(map[string]map[string]any)

		for i := 0; i < 100; i++ {
			sessionID, err := generator.Generate(ctx)
			require.NoError(t, err)

			sessions[sessionID] = map[string]any{
				"user":      "user" + string(rune(i)),
				"createdAt": "2025-01-01",
			}
		}

		assert.Len(t, sessions, 100, "All session IDs should be unique")
	})
}

// TestUUIDGenerator_Performance tests performance characteristics
func TestUUIDGenerator_Performance(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance test in short mode")
	}

	t.Run("generation speed", func(t *testing.T) {
		generator := NewUUIDGenerator()
		ctx := context.Background()

		const iterations = 100000

		for i := 0; i < iterations; i++ {
			_, err := generator.Generate(ctx)
			require.NoError(t, err)
		}

		// If we get here without timeout, performance is acceptable
	})

	t.Run("memory efficiency", func(t *testing.T) {
		generator := NewUUIDGenerator()
		ctx := context.Background()

		// Generate many UUIDs without storing them
		// Should not cause memory issues
		for i := 0; i < 1000000; i++ {
			_, err := generator.Generate(ctx)
			require.NoError(t, err)
		}
	})
}

// BenchmarkUUIDGenerator benchmarks UUID generation
func BenchmarkUUIDGenerator_Generate(b *testing.B) {
	generator := NewUUIDGenerator()
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = generator.Generate(ctx)
	}
}

func BenchmarkUUIDGenerator_GenerateWithObjects(b *testing.B) {
	generator := NewUUIDGenerator()
	ctx := context.Background()
	obj := map[string]any{"key": "value"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = generator.Generate(ctx, obj)
	}
}

func BenchmarkUUIDGenerator_Concurrent(b *testing.B) {
	generator := NewUUIDGenerator()
	ctx := context.Background()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _ = generator.Generate(ctx)
		}
	})
}

package rag

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewQuery(t *testing.T) {
	t.Run("creates query with valid text", func(t *testing.T) {
		text := "test query"

		query, err := NewQuery(text)

		require.NoError(t, err)
		require.NotNil(t, query)
		assert.Equal(t, text, query.Text)
		assert.Nil(t, query.Extra)
	})

	t.Run("returns error for empty text", func(t *testing.T) {
		query, err := NewQuery("")

		assert.Error(t, err)
		assert.Nil(t, query)
		assert.Equal(t, "text is empty", err.Error())
	})

	t.Run("creates query with whitespace text", func(t *testing.T) {
		text := "   "

		query, err := NewQuery(text)

		require.NoError(t, err)
		require.NotNil(t, query)
		assert.Equal(t, text, query.Text)
	})

	t.Run("creates query with special characters", func(t *testing.T) {
		text := "test @#$% query 中文 🚀"

		query, err := NewQuery(text)

		require.NoError(t, err)
		require.NotNil(t, query)
		assert.Equal(t, text, query.Text)
	})
}

func TestQuery_ensureExtra(t *testing.T) {
	t.Run("initializes Extra when nil", func(t *testing.T) {
		query := &Query{Text: "test"}
		assert.Nil(t, query.Extra)

		query.ensureExtra()

		assert.NotNil(t, query.Extra)
		assert.Empty(t, query.Extra)
	})

	t.Run("does not reinitialize existing Extra", func(t *testing.T) {
		existingExtra := map[string]any{"key": "value"}
		query := &Query{
			Text:  "test",
			Extra: existingExtra,
		}

		query.ensureExtra()

		assert.True(t, reflect.DeepEqual(existingExtra, query.Extra))
		assert.Equal(t, "value", query.Extra["key"])
	})

	t.Run("preserves existing data in Extra", func(t *testing.T) {
		query := &Query{
			Text:  "test",
			Extra: map[string]any{"existing": "data"},
		}

		query.ensureExtra()

		assert.Equal(t, "data", query.Extra["existing"])
	})
}

func TestQuery_Get(t *testing.T) {
	t.Run("returns value and true for existing key", func(t *testing.T) {
		query := &Query{
			Text:  "test",
			Extra: map[string]any{"key": "value"},
		}

		value, exists := query.Get("key")

		assert.True(t, exists)
		assert.Equal(t, "value", value)
	})

	t.Run("returns nil and false for non-existing key", func(t *testing.T) {
		query := &Query{
			Text:  "test",
			Extra: map[string]any{"key": "value"},
		}

		value, exists := query.Get("nonexistent")

		assert.False(t, exists)
		assert.Nil(t, value)
	})

	t.Run("initializes Extra if nil", func(t *testing.T) {
		query := &Query{Text: "test"}
		assert.Nil(t, query.Extra)

		value, exists := query.Get("key")

		assert.False(t, exists)
		assert.Nil(t, value)
		assert.NotNil(t, query.Extra)
	})

	t.Run("handles different value types", func(t *testing.T) {
		query := &Query{
			Text: "test",
			Extra: map[string]any{
				"string": "value",
				"int":    42,
				"float":  3.14,
				"bool":   true,
				"slice":  []string{"a", "b"},
				"map":    map[string]int{"x": 1},
				"nil":    nil,
			},
		}

		testCases := []struct {
			key           string
			expectedValue any
			expectedType  string
		}{
			{"string", "value", "string"},
			{"int", 42, "int"},
			{"float", 3.14, "float64"},
			{"bool", true, "bool"},
			{"slice", []string{"a", "b"}, "slice"},
			{"map", map[string]int{"x": 1}, "map"},
			{"nil", nil, "nil"},
		}

		for _, tc := range testCases {
			t.Run(tc.key, func(t *testing.T) {
				value, exists := query.Get(tc.key)
				assert.True(t, exists)
				assert.Equal(t, tc.expectedValue, value)
			})
		}
	})
}

func TestQuery_Set(t *testing.T) {
	t.Run("sets value for new key", func(t *testing.T) {
		query := &Query{Text: "test"}

		query.Set("key", "value")

		assert.NotNil(t, query.Extra)
		assert.Equal(t, "value", query.Extra["key"])
	})

	t.Run("overwrites existing key", func(t *testing.T) {
		query := &Query{
			Text:  "test",
			Extra: map[string]any{"key": "old"},
		}

		query.Set("key", "new")

		assert.Equal(t, "new", query.Extra["key"])
	})

	t.Run("initializes Extra if nil", func(t *testing.T) {
		query := &Query{Text: "test"}
		assert.Nil(t, query.Extra)

		query.Set("key", "value")

		assert.NotNil(t, query.Extra)
		assert.Equal(t, "value", query.Extra["key"])
	})

	t.Run("handles multiple sets", func(t *testing.T) {
		query := &Query{Text: "test"}

		query.Set("key1", "value1")
		query.Set("key2", 42)
		query.Set("key3", true)

		assert.Len(t, query.Extra, 3)
		assert.Equal(t, "value1", query.Extra["key1"])
		assert.Equal(t, 42, query.Extra["key2"])
		assert.Equal(t, true, query.Extra["key3"])
	})

	t.Run("handles different value types", func(t *testing.T) {
		query := &Query{Text: "test"}

		testCases := []struct {
			key   string
			value any
		}{
			{"string", "value"},
			{"int", 42},
			{"float", 3.14},
			{"bool", true},
			{"slice", []string{"a", "b"}},
			{"map", map[string]int{"x": 1}},
			{"nil", nil},
			{"struct", struct{ Name string }{"test"}},
		}

		for _, tc := range testCases {
			t.Run(tc.key, func(t *testing.T) {
				query.Set(tc.key, tc.value)
				assert.Equal(t, tc.value, query.Extra[tc.key])
			})
		}
	})

	t.Run("handles empty string key", func(t *testing.T) {
		query := &Query{Text: "test"}

		query.Set("", "value")

		assert.Equal(t, "value", query.Extra[""])
	})
}

func TestQuery_Clone(t *testing.T) {
	t.Run("clones query with text only", func(t *testing.T) {
		original := &Query{Text: "test query"}

		cloned := original.Clone()

		require.NotNil(t, cloned)
		assert.NotSame(t, original, cloned)
		assert.Equal(t, original.Text, cloned.Text)
		assert.Nil(t, cloned.Extra)
	})

	t.Run("clones query with Extra", func(t *testing.T) {
		original := &Query{
			Text: "test query",
			Extra: map[string]any{
				"key1": "value1",
				"key2": 42,
				"key3": true,
			},
		}

		cloned := original.Clone()

		require.NotNil(t, cloned)
		assert.NotSame(t, original, cloned)
		assert.Equal(t, original.Text, cloned.Text)
		assert.NotSame(t, &original.Extra, &cloned.Extra)
		assert.Equal(t, original.Extra, cloned.Extra)
	})

	t.Run("modifications to clone do not affect original", func(t *testing.T) {
		original := &Query{
			Text: "test query",
			Extra: map[string]any{
				"key": "original",
			},
		}

		cloned := original.Clone()
		cloned.Text = "modified query"
		cloned.Set("key", "modified")
		cloned.Set("newkey", "new")

		assert.Equal(t, "test query", original.Text)
		assert.Equal(t, "original", original.Extra["key"])
		_, exists := original.Get("newkey")
		assert.False(t, exists)

		assert.Equal(t, "modified query", cloned.Text)
		assert.Equal(t, "modified", cloned.Extra["key"])
		assert.Equal(t, "new", cloned.Extra["newkey"])
	})

	t.Run("clones query with nil Extra", func(t *testing.T) {
		original := &Query{
			Text:  "test query",
			Extra: nil,
		}

		cloned := original.Clone()

		assert.NotSame(t, original, cloned)
		assert.Equal(t, original.Text, cloned.Text)
		assert.Nil(t, cloned.Extra)
	})

	t.Run("clones query with empty Extra", func(t *testing.T) {
		original := &Query{
			Text:  "test query",
			Extra: map[string]any{},
		}

		cloned := original.Clone()

		assert.NotSame(t, original, cloned)
		assert.NotSame(t, &original.Extra, &cloned.Extra)
		assert.Empty(t, cloned.Extra)
	})

	t.Run("clones query with complex Extra values", func(t *testing.T) {
		original := &Query{
			Text: "test query",
			Extra: map[string]any{
				"slice": []string{"a", "b", "c"},
				"map":   map[string]int{"x": 1, "y": 2},
				"nested": map[string]any{
					"inner": "value",
				},
			},
		}

		cloned := original.Clone()

		assert.NotSame(t, original, cloned)
		assert.NotSame(t, &original.Extra, &cloned.Extra)

		// Note: maps.Clone performs shallow copy
		// Modifying nested structures will affect both
		assert.Equal(t, original.Extra["slice"], cloned.Extra["slice"])
		assert.Equal(t, original.Extra["map"], cloned.Extra["map"])
		assert.Equal(t, original.Extra["nested"], cloned.Extra["nested"])
	})
}

func TestQuery_Integration(t *testing.T) {
	t.Run("complete workflow", func(t *testing.T) {
		// Create query
		query, err := NewQuery("What is RAG?")
		require.NoError(t, err)

		// Set metadata
		query.Set("source", "user")
		query.Set("timestamp", "2024-01-01")
		query.Set("priority", 1)

		// Get metadata
		source, exists := query.Get("source")
		assert.True(t, exists)
		assert.Equal(t, "user", source)

		// Clone and modify
		cloned := query.Clone()
		cloned.Set("source", "system")

		// Verify original is unchanged
		originalSource, _ := query.Get("source")
		assert.Equal(t, "user", originalSource)

		clonedSource, _ := cloned.Get("source")
		assert.Equal(t, "system", clonedSource)
	})

	t.Run("chained operations", func(t *testing.T) {
		query, _ := NewQuery("test")

		query.Set("a", 1)
		query.Set("b", 2)
		query.Set("c", 3)

		clone1 := query.Clone()
		clone1.Set("a", 10)

		clone2 := clone1.Clone()
		clone2.Set("b", 20)

		// Verify each clone is independent
		assert.Equal(t, 1, query.Extra["a"])
		assert.Equal(t, 10, clone1.Extra["a"])
		assert.Equal(t, 2, clone1.Extra["b"])
		assert.Equal(t, 10, clone2.Extra["a"])
		assert.Equal(t, 20, clone2.Extra["b"])
	})
}

func TestQuery_EdgeCases(t *testing.T) {
	t.Run("query with very long text", func(t *testing.T) {
		longText := string(make([]byte, 10000))
		query, err := NewQuery(longText)

		require.NoError(t, err)
		assert.Len(t, query.Text, 10000)
	})

	t.Run("Extra with same key set multiple times", func(t *testing.T) {
		query := &Query{Text: "test"}

		query.Set("key", "value1")
		query.Set("key", "value2")
		query.Set("key", "value3")

		value, _ := query.Get("key")
		assert.Equal(t, "value3", value)
	})

	t.Run("Get on query with empty Extra map", func(t *testing.T) {
		query := &Query{
			Text:  "test",
			Extra: map[string]any{},
		}

		value, exists := query.Get("key")

		assert.False(t, exists)
		assert.Nil(t, value)
	})

	t.Run("Set nil value", func(t *testing.T) {
		query := &Query{Text: "test"}

		query.Set("key", nil)

		value, exists := query.Get("key")
		assert.True(t, exists)
		assert.Nil(t, value)
	})
}

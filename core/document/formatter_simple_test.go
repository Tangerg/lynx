package document

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSimpleFormatterConfig tests the configuration structure
func TestSimpleFormatterConfig(t *testing.T) {
	t.Run("config with all fields", func(t *testing.T) {
		config := &SimpleFormatterConfig{
			ExcludedInferenceMetadataKeys: []string{"key1", "key2"},
			ExcludedEmbedMetadataKeys:     []string{"key3", "key4"},
		}

		assert.Len(t, config.ExcludedInferenceMetadataKeys, 2)
		assert.Len(t, config.ExcludedEmbedMetadataKeys, 2)
	})

	t.Run("empty config", func(t *testing.T) {
		config := &SimpleFormatterConfig{}

		assert.Nil(t, config.ExcludedInferenceMetadataKeys)
		assert.Nil(t, config.ExcludedEmbedMetadataKeys)
	})
}

// TestNewSimpleFormatter tests the constructor
func TestNewSimpleFormatter(t *testing.T) {
	t.Run("with nil config", func(t *testing.T) {
		formatter := NewSimpleFormatter(nil)

		require.NotNil(t, formatter)
		assert.NotNil(t, formatter.excludedInferenceMetadataKeys)
		assert.NotNil(t, formatter.excludedEmbedMetadataKeys)
		assert.Empty(t, formatter.excludedInferenceMetadataKeys)
		assert.Empty(t, formatter.excludedEmbedMetadataKeys)
	})

	t.Run("with empty config", func(t *testing.T) {
		config := &SimpleFormatterConfig{}
		formatter := NewSimpleFormatter(config)

		require.NotNil(t, formatter)
		assert.NotNil(t, formatter.excludedInferenceMetadataKeys)
		assert.NotNil(t, formatter.excludedEmbedMetadataKeys)
		assert.Empty(t, formatter.excludedInferenceMetadataKeys)
		assert.Empty(t, formatter.excludedEmbedMetadataKeys)
	})

	t.Run("with full config", func(t *testing.T) {
		config := &SimpleFormatterConfig{
			ExcludedInferenceMetadataKeys: []string{"inference_key1", "inference_key2"},
			ExcludedEmbedMetadataKeys:     []string{"embed_key1", "embed_key2"},
		}
		formatter := NewSimpleFormatter(config)

		require.NotNil(t, formatter)
		assert.Equal(t, config.ExcludedInferenceMetadataKeys, formatter.excludedInferenceMetadataKeys)
		assert.Equal(t, config.ExcludedEmbedMetadataKeys, formatter.excludedEmbedMetadataKeys)
	})

	t.Run("with nil slices in config", func(t *testing.T) {
		config := &SimpleFormatterConfig{
			ExcludedInferenceMetadataKeys: nil,
			ExcludedEmbedMetadataKeys:     nil,
		}
		formatter := NewSimpleFormatter(config)

		require.NotNil(t, formatter)
		assert.NotNil(t, formatter.excludedInferenceMetadataKeys)
		assert.NotNil(t, formatter.excludedEmbedMetadataKeys)
	})
}

// TestNewDefaultSimpleFormatter tests the default constructor
func TestNewDefaultSimpleFormatter(t *testing.T) {
	t.Run("creates formatter with default config", func(t *testing.T) {
		formatter := NewDefaultSimpleFormatter()

		require.NotNil(t, formatter)
		assert.NotNil(t, formatter.excludedInferenceMetadataKeys)
		assert.NotNil(t, formatter.excludedEmbedMetadataKeys)
		assert.Empty(t, formatter.excludedInferenceMetadataKeys)
		assert.Empty(t, formatter.excludedEmbedMetadataKeys)
	})
}

// TestSimpleFormatter_Format tests the Format method
func TestSimpleFormatter_Format(t *testing.T) {
	t.Run("format with no metadata - mode all", func(t *testing.T) {
		formatter := NewDefaultSimpleFormatter()
		doc := mustCreateDocument(t, "Test content", nil)

		result := formatter.Format(doc, MetadataModeAll)

		assert.Contains(t, result, "Test content")
		// Should have empty metadata section
		assert.Contains(t, result, "\n\n")
	})

	t.Run("format with metadata - mode all", func(t *testing.T) {
		formatter := NewDefaultSimpleFormatter()
		doc := mustCreateDocument(t, "Test content", nil)
		doc.Metadata["author"] = "John Doe"
		doc.Metadata["date"] = "2025-01-01"

		result := formatter.Format(doc, MetadataModeAll)

		assert.Contains(t, result, "Test content")
		assert.Contains(t, result, "author: John Doe")
		assert.Contains(t, result, "date: 2025-01-01")
	})

	t.Run("format with metadata - mode none", func(t *testing.T) {
		formatter := NewDefaultSimpleFormatter()
		doc := mustCreateDocument(t, "Test content", nil)
		doc.Metadata["author"] = "John Doe"
		doc.Metadata["date"] = "2025-01-01"

		result := formatter.Format(doc, MetadataModeNone)

		assert.Contains(t, result, "Test content")
		assert.NotContains(t, result, "author")
		assert.NotContains(t, result, "date")
		assert.NotContains(t, result, "John Doe")
	})

	t.Run("format with excluded inference metadata", func(t *testing.T) {
		config := &SimpleFormatterConfig{
			ExcludedInferenceMetadataKeys: []string{"internal_id", "debug_info"},
		}
		formatter := NewSimpleFormatter(config)
		doc := mustCreateDocument(t, "Test content", nil)
		doc.Metadata["author"] = "John Doe"
		doc.Metadata["internal_id"] = "12345"
		doc.Metadata["debug_info"] = "some debug data"

		result := formatter.Format(doc, MetadataModeInference)

		assert.Contains(t, result, "Test content")
		assert.Contains(t, result, "author: John Doe")
		assert.NotContains(t, result, "internal_id")
		assert.NotContains(t, result, "debug_info")
	})

	t.Run("format with excluded embed metadata", func(t *testing.T) {
		config := &SimpleFormatterConfig{
			ExcludedEmbedMetadataKeys: []string{"timestamp", "source_url"},
		}
		formatter := NewSimpleFormatter(config)
		doc := mustCreateDocument(t, "Test content", nil)
		doc.Metadata["author"] = "John Doe"
		doc.Metadata["timestamp"] = "2025-01-01T00:00:00Z"
		doc.Metadata["source_url"] = "https://example.com"

		result := formatter.Format(doc, MetadataModeEmbed)

		assert.Contains(t, result, "Test content")
		assert.Contains(t, result, "author: John Doe")
		assert.NotContains(t, result, "timestamp")
		assert.NotContains(t, result, "source_url")
	})

	t.Run("format with multiple metadata types", func(t *testing.T) {
		formatter := NewDefaultSimpleFormatter()
		doc := mustCreateDocument(t, "Test content", nil)
		doc.Metadata["string_key"] = "string value"
		doc.Metadata["int_key"] = 42
		doc.Metadata["float_key"] = 3.14
		doc.Metadata["bool_key"] = true

		result := formatter.Format(doc, MetadataModeAll)

		assert.Contains(t, result, "Test content")
		assert.Contains(t, result, "string_key: string value")
		assert.Contains(t, result, "int_key: 42")
		assert.Contains(t, result, "float_key: 3.14")
		assert.Contains(t, result, "bool_key: true")
	})

	t.Run("format preserves newlines in metadata separator", func(t *testing.T) {
		formatter := NewDefaultSimpleFormatter()
		doc := mustCreateDocument(t, "Content", nil)
		doc.Metadata["key1"] = "value1"
		doc.Metadata["key2"] = "value2"

		result := formatter.Format(doc, MetadataModeAll)

		// Each metadata entry should be on a separate line
		assert.Contains(t, result, "key1: value1")
		assert.Contains(t, result, "key2: value2")
		lines := strings.Split(result, "\n")
		assert.True(t, len(lines) >= 3) // at least 2 metadata lines + content
	})

	t.Run("format template structure", func(t *testing.T) {
		formatter := NewDefaultSimpleFormatter()
		doc := mustCreateDocument(t, "Test content", nil)
		doc.Metadata["author"] = "John"

		result := formatter.Format(doc, MetadataModeAll)

		// Verify template structure: metadata\n\ncontent
		parts := strings.Split(result, "\n\n")
		assert.Len(t, parts, 2, "Should have metadata and content separated by double newline")
		assert.Contains(t, parts[0], "author: John")
		assert.Equal(t, "Test content", parts[1])
	})
}

// TestSimpleFormatter_filterMetadataByMode tests the metadata filtering logic
func TestSimpleFormatter_filterMetadataByMode(t *testing.T) {
	t.Run("mode all returns all metadata", func(t *testing.T) {
		formatter := NewDefaultSimpleFormatter()
		metadata := map[string]any{
			"key1": "value1",
			"key2": "value2",
			"key3": "value3",
		}

		result := formatter.filterMetadataByMode(metadata, MetadataModeAll)

		assert.Len(t, result, 3)
		assert.Equal(t, "value1", result["key1"])
		assert.Equal(t, "value2", result["key2"])
		assert.Equal(t, "value3", result["key3"])
	})

	t.Run("mode none returns empty map", func(t *testing.T) {
		formatter := NewDefaultSimpleFormatter()
		metadata := map[string]any{
			"key1": "value1",
			"key2": "value2",
		}

		result := formatter.filterMetadataByMode(metadata, MetadataModeNone)

		assert.NotNil(t, result)
		assert.Empty(t, result)
	})

	t.Run("mode inference excludes configured keys", func(t *testing.T) {
		config := &SimpleFormatterConfig{
			ExcludedInferenceMetadataKeys: []string{"exclude1", "exclude2"},
		}
		formatter := NewSimpleFormatter(config)
		metadata := map[string]any{
			"exclude1": "should be excluded",
			"exclude2": "should be excluded",
			"include1": "should be included",
		}

		result := formatter.filterMetadataByMode(metadata, MetadataModeInference)

		assert.Len(t, result, 1)
		assert.Equal(t, "should be included", result["include1"])
		assert.NotContains(t, result, "exclude1")
		assert.NotContains(t, result, "exclude2")
	})

	t.Run("mode embed excludes configured keys", func(t *testing.T) {
		config := &SimpleFormatterConfig{
			ExcludedEmbedMetadataKeys: []string{"exclude1", "exclude2"},
		}
		formatter := NewSimpleFormatter(config)
		metadata := map[string]any{
			"exclude1": "should be excluded",
			"exclude2": "should be excluded",
			"include1": "should be included",
		}

		result := formatter.filterMetadataByMode(metadata, MetadataModeEmbed)

		assert.Len(t, result, 1)
		assert.Equal(t, "should be included", result["include1"])
		assert.NotContains(t, result, "exclude1")
		assert.NotContains(t, result, "exclude2")
	})

	t.Run("filtering does not modify original metadata", func(t *testing.T) {
		config := &SimpleFormatterConfig{
			ExcludedInferenceMetadataKeys: []string{"exclude1"},
		}
		formatter := NewSimpleFormatter(config)
		metadata := map[string]any{
			"exclude1": "value1",
			"include1": "value2",
		}

		result := formatter.filterMetadataByMode(metadata, MetadataModeInference)

		// Original should be unchanged
		assert.Len(t, metadata, 2)
		assert.Contains(t, metadata, "exclude1")
		assert.Contains(t, metadata, "include1")

		// Result should be filtered
		assert.Len(t, result, 1)
		assert.Contains(t, result, "include1")
		assert.NotContains(t, result, "exclude1")
	})

	t.Run("empty metadata returns empty map", func(t *testing.T) {
		formatter := NewDefaultSimpleFormatter()
		metadata := map[string]any{}

		result := formatter.filterMetadataByMode(metadata, MetadataModeAll)

		assert.NotNil(t, result)
		assert.Empty(t, result)
	})

	t.Run("nil metadata handling", func(t *testing.T) {
		formatter := NewDefaultSimpleFormatter()
		var metadata map[string]any

		// Should not panic
		result := formatter.filterMetadataByMode(metadata, MetadataModeAll)

		assert.Nil(t, result)
	})

	t.Run("inference mode with no exclusions", func(t *testing.T) {
		config := &SimpleFormatterConfig{
			ExcludedInferenceMetadataKeys: []string{},
		}
		formatter := NewSimpleFormatter(config)
		metadata := map[string]any{
			"key1": "value1",
			"key2": "value2",
		}

		result := formatter.filterMetadataByMode(metadata, MetadataModeInference)

		assert.Len(t, result, 2)
		assert.Equal(t, "value1", result["key1"])
		assert.Equal(t, "value2", result["key2"])
	})
}

// TestSimpleFormatter_InterfaceCompliance verifies interface implementation
func TestSimpleFormatter_InterfaceCompliance(t *testing.T) {
	formatter := NewDefaultSimpleFormatter()
	var _ Formatter = formatter
}

// TestSimpleFormatter_Integration tests complete workflows
func TestSimpleFormatter_Integration(t *testing.T) {
	t.Run("complete formatting workflow", func(t *testing.T) {
		config := &SimpleFormatterConfig{
			ExcludedInferenceMetadataKeys: []string{"internal_id", "debug"},
			ExcludedEmbedMetadataKeys:     []string{"timestamp", "version"},
		}
		formatter := NewSimpleFormatter(config)

		doc := mustCreateDocument(t, "This is a test document.", nil)
		doc.Metadata["author"] = "Alice"
		doc.Metadata["internal_id"] = "12345"
		doc.Metadata["timestamp"] = "2025-01-01"
		doc.Metadata["category"] = "test"
		doc.Metadata["debug"] = "debug info"
		doc.Metadata["version"] = "1.0"

		// Test all mode
		resultAll := formatter.Format(doc, MetadataModeAll)
		assert.Contains(t, resultAll, "author: Alice")
		assert.Contains(t, resultAll, "internal_id: 12345")
		assert.Contains(t, resultAll, "timestamp: 2025-01-01")
		assert.Contains(t, resultAll, "category: test")
		assert.Contains(t, resultAll, "This is a test document.")

		// Test inference mode
		resultInference := formatter.Format(doc, MetadataModeInference)
		assert.Contains(t, resultInference, "author: Alice")
		assert.Contains(t, resultInference, "category: test")
		assert.NotContains(t, resultInference, "internal_id")
		assert.NotContains(t, resultInference, "debug")
		assert.Contains(t, resultInference, "timestamp: 2025-01-01") // not excluded in inference
		assert.Contains(t, resultInference, "This is a test document.")

		// Test embed mode
		resultEmbed := formatter.Format(doc, MetadataModeEmbed)
		assert.Contains(t, resultEmbed, "author: Alice")
		assert.Contains(t, resultEmbed, "category: test")
		assert.NotContains(t, resultEmbed, "timestamp")
		assert.NotContains(t, resultEmbed, "version")
		assert.Contains(t, resultEmbed, "internal_id: 12345") // not excluded in embed
		assert.Contains(t, resultEmbed, "This is a test document.")

		// Test none mode
		resultNone := formatter.Format(doc, MetadataModeNone)
		assert.NotContains(t, resultNone, "author")
		assert.NotContains(t, resultNone, "internal_id")
		assert.NotContains(t, resultNone, "timestamp")
		assert.Contains(t, resultNone, "This is a test document.")
	})

	t.Run("complex metadata values", func(t *testing.T) {
		formatter := NewDefaultSimpleFormatter()
		doc := mustCreateDocument(t, "Complex document", nil)
		doc.Metadata["string"] = "text value"
		doc.Metadata["integer"] = 123
		doc.Metadata["float"] = 45.67
		doc.Metadata["boolean"] = true
		doc.Metadata["negative"] = -999

		result := formatter.Format(doc, MetadataModeAll)

		assert.Contains(t, result, "string: text value")
		assert.Contains(t, result, "integer: 123")
		assert.Contains(t, result, "float: 45.67")
		assert.Contains(t, result, "boolean: true")
		assert.Contains(t, result, "negative: -999")
		assert.Contains(t, result, "Complex document")
	})

	t.Run("document with special characters", func(t *testing.T) {
		formatter := NewDefaultSimpleFormatter()
		doc := mustCreateDocument(t, "Content with: special, characters!", nil)
		doc.Metadata["key"] = "value: with: colons"
		doc.Metadata["another"] = "value, with, commas"

		result := formatter.Format(doc, MetadataModeAll)

		assert.Contains(t, result, "key: value: with: colons")
		assert.Contains(t, result, "another: value, with, commas")
		assert.Contains(t, result, "Content with: special, characters!")
	})

	t.Run("multiple formatters with same document", func(t *testing.T) {
		doc := mustCreateDocument(t, "Test content", nil)
		doc.Metadata["key1"] = "value1"
		doc.Metadata["key2"] = "value2"

		formatter1 := NewDefaultSimpleFormatter()
		formatter2 := NewSimpleFormatter(&SimpleFormatterConfig{
			ExcludedInferenceMetadataKeys: []string{"key1"},
		})

		result1 := formatter1.Format(doc, MetadataModeInference)
		result2 := formatter2.Format(doc, MetadataModeInference)

		// formatter1 should include both keys
		assert.Contains(t, result1, "key1: value1")
		assert.Contains(t, result1, "key2: value2")

		// formatter2 should exclude key1
		assert.NotContains(t, result2, "key1")
		assert.Contains(t, result2, "key2: value2")
	})
}

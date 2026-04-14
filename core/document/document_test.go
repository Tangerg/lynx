package document

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Tangerg/lynx/ai/media"
	"github.com/Tangerg/lynx/pkg/mime"
)

// TestNewDocument tests the NewDocument constructor
func TestNewDocument(t *testing.T) {
	tests := []struct {
		name        string
		text        string
		media       *media.Media
		wantErr     bool
		errContains string
		checkFn     func(*testing.T, *Document)
	}{
		{
			name:    "valid document with text only",
			text:    "hello world",
			media:   nil,
			wantErr: false,
			checkFn: func(t *testing.T, doc *Document) {
				assert.Equal(t, "hello world", doc.Text)
				assert.Nil(t, doc.Media)
				assert.NotNil(t, doc.Metadata)
				assert.Empty(t, doc.Metadata)
				assert.NotNil(t, doc.Formatter)
				assert.Empty(t, doc.ID)
				assert.Equal(t, float64(0), doc.Score)
			},
		},
		{
			name:    "valid document with media only",
			text:    "",
			media:   mustCreateMedia(t, "image/png", []byte{0x89, 0x50, 0x4E, 0x47}),
			wantErr: false,
			checkFn: func(t *testing.T, doc *Document) {
				assert.Empty(t, doc.Text)
				assert.NotNil(t, doc.Media)
				assert.NotNil(t, doc.Metadata)
				assert.Empty(t, doc.Metadata)
				assert.NotNil(t, doc.Formatter)
			},
		},
		{
			name:    "valid document with both text and media",
			text:    "image description",
			media:   mustCreateMedia(t, "image/jpeg", []byte{0xFF, 0xD8, 0xFF}),
			wantErr: false,
			checkFn: func(t *testing.T, doc *Document) {
				assert.Equal(t, "image description", doc.Text)
				assert.NotNil(t, doc.Media)
				assert.NotNil(t, doc.Metadata)
				assert.NotNil(t, doc.Formatter)
			},
		},
		{
			name:        "empty text and nil media",
			text:        "",
			media:       nil,
			wantErr:     true,
			errContains: "document requires either text content or media attachment",
		},
		{
			name:    "whitespace text only (valid)",
			text:    "   ",
			media:   nil,
			wantErr: false,
			checkFn: func(t *testing.T, doc *Document) {
				assert.Equal(t, "   ", doc.Text)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc, err := NewDocument(tt.text, tt.media)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
				assert.Nil(t, doc)
			} else {
				require.NoError(t, err)
				require.NotNil(t, doc)
				if tt.checkFn != nil {
					tt.checkFn(t, doc)
				}
			}
		})
	}
}

// TestDocument_Format tests the Format method
func TestDocument_Format(t *testing.T) {
	t.Run("format with default Nop formatter", func(t *testing.T) {
		doc, err := NewDocument("test content", nil)
		require.NoError(t, err)

		result := doc.Format()

		// Nop formatter returns only text
		assert.Equal(t, "test content", result)
	})

	t.Run("format with custom formatter", func(t *testing.T) {
		doc, err := NewDocument("test content", nil)
		require.NoError(t, err)

		customFormatter := mockFormatterFn(
			func(d *Document, mode MetadataMode) string {
				return "custom: " + d.Text
			})
		doc.Formatter = customFormatter

		result := doc.Format()
		assert.Equal(t, "custom: test content", result)
	})

	t.Run("format with metadata", func(t *testing.T) {
		doc, err := NewDocument("content", nil)
		require.NoError(t, err)
		doc.Metadata["author"] = "test"
		doc.Metadata["date"] = "2025-01-01"

		result := doc.Format()

		// Nop formatter ignores metadata
		assert.Equal(t, "content", result)
	})

	t.Run("format empty text document", func(t *testing.T) {
		media := mustCreateMedia(t, "image/png", []byte{1, 2, 3})
		doc, err := NewDocument("", media)
		require.NoError(t, err)

		result := doc.Format()
		assert.Equal(t, "", result)
	})
}

// TestDocument_FormatByMetadataMode tests the FormatByMetadataMode method
func TestDocument_FormatByMetadataMode(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		mode     MetadataMode
		expected string
	}{
		{
			name:     "mode all",
			text:     "test content",
			mode:     MetadataModeAll,
			expected: "test content",
		},
		{
			name:     "mode embed",
			text:     "embedding content",
			mode:     MetadataModeEmbed,
			expected: "embedding content",
		},
		{
			name:     "mode inference",
			text:     "inference content",
			mode:     MetadataModeInference,
			expected: "inference content",
		},
		{
			name:     "mode none",
			text:     "plain content",
			mode:     MetadataModeNone,
			expected: "plain content",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc, err := NewDocument(tt.text, nil)
			require.NoError(t, err)

			result := doc.FormatByMetadataMode(tt.mode)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestDocument_FormatByMetadataMode_CustomFormatter tests with custom formatter
func TestDocument_FormatByMetadataMode_CustomFormatter(t *testing.T) {
	t.Run("custom formatter respects metadata mode", func(t *testing.T) {
		doc, err := NewDocument("content", nil)
		require.NoError(t, err)
		doc.Metadata["key"] = "value"

		customFormatter := mockFormatterFn(
			func(d *Document, mode MetadataMode) string {
				switch mode {
				case MetadataModeAll:
					return d.Text + " [all metadata]"
				case MetadataModeEmbed:
					return d.Text + " [embed]"
				case MetadataModeNone:
					return d.Text
				default:
					return d.Text
				}
			})
		doc.Formatter = customFormatter

		t.Run("all mode", func(t *testing.T) {
			result := doc.FormatByMetadataMode(MetadataModeAll)
			assert.Equal(t, "content [all metadata]", result)
		})

		t.Run("embed mode", func(t *testing.T) {
			result := doc.FormatByMetadataMode(MetadataModeEmbed)
			assert.Equal(t, "content [embed]", result)
		})

		t.Run("none mode", func(t *testing.T) {
			result := doc.FormatByMetadataMode(MetadataModeNone)
			assert.Equal(t, "content", result)
		})
	})
}

// TestDocument_FormatByMetadataModeWithFormatter tests the FormatByMetadataModeWithFormatter method
func TestDocument_FormatByMetadataModeWithFormatter(t *testing.T) {
	t.Run("use provided formatter", func(t *testing.T) {
		doc, err := NewDocument("test", nil)
		require.NoError(t, err)

		customFormatter := mockFormatterFn(func(d *Document, mode MetadataMode) string {
			return "custom: " + d.Text + " mode: " + string(mode)
		})

		result := doc.FormatByMetadataModeWithFormatter(MetadataModeAll, customFormatter)
		assert.Equal(t, "custom: test mode: all", result)
	})

	t.Run("fallback to Nop when formatter is nil", func(t *testing.T) {
		doc, err := NewDocument("test content", nil)
		require.NoError(t, err)

		result := doc.FormatByMetadataModeWithFormatter(MetadataModeAll, nil)

		// Should use Nop formatter
		assert.Equal(t, "test content", result)
	})

	t.Run("override document's default formatter", func(t *testing.T) {
		doc, err := NewDocument("content", nil)
		require.NoError(t, err)

		doc.Formatter = mockFormatterFn(func(d *Document, mode MetadataMode) string {
			return "default: " + d.Text
		})

		overrideFormatter := mockFormatterFn(func(d *Document, mode MetadataMode) string {
			return "override: " + d.Text
		})
		result := doc.FormatByMetadataModeWithFormatter(MetadataModeAll, overrideFormatter)
		assert.Equal(t, "override: content", result)
	})

	t.Run("different modes with same formatter", func(t *testing.T) {
		doc, err := NewDocument("test", nil)
		require.NoError(t, err)

		formatter := mockFormatterFn(func(d *Document, mode MetadataMode) string {
			return d.Text + ":" + string(mode)
		})

		resultAll := doc.FormatByMetadataModeWithFormatter(MetadataModeAll, formatter)
		resultEmbed := doc.FormatByMetadataModeWithFormatter(MetadataModeEmbed, formatter)
		resultNone := doc.FormatByMetadataModeWithFormatter(MetadataModeNone, formatter)

		assert.Equal(t, "test:all", resultAll)
		assert.Equal(t, "test:embed", resultEmbed)
		assert.Equal(t, "test:none", resultNone)
	})
}

// TestDocument_Fields tests all document fields
func TestDocument_Fields(t *testing.T) {
	t.Run("all fields can be set and retrieved", func(t *testing.T) {
		m := mustCreateMedia(t, "video/mp4", []byte{0x00, 0x00, 0x00, 0x18})
		doc, err := NewDocument("test content", m)
		require.NoError(t, err)

		// Set all fields
		doc.ID = "doc-123"
		doc.Score = 0.95
		doc.Metadata["author"] = "John Doe"
		doc.Metadata["tags"] = []string{"test", "document"}
		doc.Metadata["priority"] = 1

		customFormatter := mockFormatterFn(func(d *Document, mode MetadataMode) string {
			return "formatted"
		})
		doc.Formatter = customFormatter

		// Verify all fields
		assert.Equal(t, "doc-123", doc.ID)
		assert.Equal(t, 0.95, doc.Score)
		assert.Equal(t, "test content", doc.Text)
		assert.Same(t, m, doc.Media)
		assert.Equal(t, "John Doe", doc.Metadata["author"])
		assert.Equal(t, []string{"test", "document"}, doc.Metadata["tags"])
		assert.Equal(t, 1, doc.Metadata["priority"])
		assert.Len(t, doc.Metadata, 3)
	})

	t.Run("metadata operations", func(t *testing.T) {
		doc, err := NewDocument("test", nil)
		require.NoError(t, err)

		// Add metadata
		doc.Metadata["key1"] = "value1"
		doc.Metadata["key2"] = 123
		assert.Len(t, doc.Metadata, 2)

		// Update metadata
		doc.Metadata["key1"] = "updated"
		assert.Equal(t, "updated", doc.Metadata["key1"])

		// Delete metadata
		delete(doc.Metadata, "key2")
		assert.Len(t, doc.Metadata, 1)
		_, exists := doc.Metadata["key2"]
		assert.False(t, exists)
	})
}

// TestDocument_WithMedia tests document with media
func TestDocument_WithMedia(t *testing.T) {
	t.Run("document with image media", func(t *testing.T) {
		imageData := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}
		m := mustCreateMedia(t, "image/png", imageData)

		doc, err := NewDocument("Image description", m)
		require.NoError(t, err)

		assert.Equal(t, "Image description", doc.Text)
		assert.NotNil(t, doc.Media)
		assert.Equal(t, "image/png", doc.Media.MimeType.String())

		data, err := doc.Media.DataAsBytes()
		require.NoError(t, err)
		assert.Equal(t, imageData, data)
	})

	t.Run("document with audio media", func(t *testing.T) {
		audioData := []byte{0x49, 0x44, 0x33} // ID3 tag
		m := mustCreateMedia(t, "audio/mpeg", audioData)

		doc, err := NewDocument("", m)
		require.NoError(t, err)

		assert.Empty(t, doc.Text)
		assert.NotNil(t, doc.Media)
		assert.Equal(t, "audio/mpeg", doc.Media.MimeType.String())
	})
}

// TestDocument_Integration tests complete document workflows
func TestDocument_Integration(t *testing.T) {
	t.Run("complete document lifecycle", func(t *testing.T) {
		// Create document with text
		doc, err := NewDocument("Initial content", nil)
		require.NoError(t, err)

		// Set properties
		doc.ID = "doc-001"
		doc.Score = 0.88
		doc.Metadata["author"] = "Alice"
		doc.Metadata["version"] = 1

		// Format with default formatter
		result1 := doc.Format()
		assert.Equal(t, "Initial content", result1)

		// Format with specific mode
		result2 := doc.FormatByMetadataMode(MetadataModeEmbed)
		assert.Equal(t, "Initial content", result2)

		// Format with custom formatter
		customFormatter := mockFormatterFn(func(d *Document, mode MetadataMode) string {
			return "[" + d.ID + "] " + d.Text
		})
		result3 := doc.FormatByMetadataModeWithFormatter(MetadataModeAll, customFormatter)
		assert.Equal(t, "[doc-001] Initial content", result3)

		// Update content
		doc.Text = "Updated content"
		doc.Score = 0.92
		doc.Metadata["version"] = 2

		// Verify updates
		assert.Equal(t, "Updated content", doc.Text)
		assert.Equal(t, 0.92, doc.Score)
		assert.Equal(t, 2, doc.Metadata["version"])
	})

	t.Run("document with media and custom formatting", func(t *testing.T) {
		m := mustCreateMedia(t, "video/mp4", []byte{0x00, 0x00, 0x00, 0x20})
		doc, err := NewDocument("Video tutorial", m)
		require.NoError(t, err)

		doc.ID = "vid-001"
		doc.Metadata["duration"] = 120
		doc.Metadata["resolution"] = "1920x1080"

		customFormatter := mockFormatterFn(func(d *Document, mode MetadataMode) string {
			if d.Media != nil {
				return d.Text + " [" + d.Media.MimeType.String() + "]"
			}
			return d.Text
		})

		result := doc.FormatByMetadataModeWithFormatter(MetadataModeAll, customFormatter)
		assert.Equal(t, "Video tutorial [video/mp4]", result)
	})
}

// mockFormatter is a test helper for custom formatter testing
type mockFormatterFn func(*Document, MetadataMode) string

func (m mockFormatterFn) Format(doc *Document, mode MetadataMode) string {
	if m != nil {
		return m(doc, mode)
	}
	return doc.Text
}

// mustCreateMedia is a test helper to create media or fail
func mustCreateMedia(t *testing.T, mimeType string, data []byte) *media.Media {
	t.Helper()
	split := strings.Split(mimeType, "/")
	m, err := media.NewMedia(mime.MustNew(split[0], split[1]), data)
	require.NoError(t, err)
	return m
}

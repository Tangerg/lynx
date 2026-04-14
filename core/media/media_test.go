package media

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Tangerg/lynx/pkg/mime"
)

func TestNewMedia(t *testing.T) {
	tests := []struct {
		name        string
		mimeType    *mime.MIME
		data        any
		wantErr     bool
		errContains string
	}{
		{
			name:     "valid media with string data",
			mimeType: mime.MustNew("text", "plain"),
			data:     "test content",
			wantErr:  false,
		},
		{
			name:     "valid media with byte data",
			mimeType: mime.MustNew("application", "octet-stream"),
			data:     []byte{1, 2, 3},
			wantErr:  false,
		},
		{
			name:        "nil mime type",
			mimeType:    nil,
			data:        "test",
			wantErr:     true,
			errContains: "mime type is required",
		},
		{
			name:        "nil data",
			mimeType:    mime.MustNew("text", "plain"),
			data:        nil,
			wantErr:     true,
			errContains: "data is required",
		},
		{
			name:        "both nil",
			mimeType:    nil,
			data:        nil,
			wantErr:     true,
			errContains: "mime type is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			media, err := NewMedia(tt.mimeType, tt.data)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
				assert.Nil(t, media)
			} else {
				require.NoError(t, err)
				require.NotNil(t, media)
				assert.Equal(t, tt.mimeType, media.MimeType)
				assert.Equal(t, tt.data, media.Data)
				assert.NotNil(t, media.Metadata)
				assert.Empty(t, media.Metadata)
				assert.Empty(t, media.ID)
				assert.Empty(t, media.Name)
			}
		})
	}
}

func TestMedia_DataAsBytes(t *testing.T) {
	tests := []struct {
		name        string
		data        any
		expected    []byte
		wantErr     bool
		errContains string
	}{
		{
			name:     "valid byte slice",
			data:     []byte{1, 2, 3, 4, 5},
			expected: []byte{1, 2, 3, 4, 5},
			wantErr:  false,
		},
		{
			name:     "empty byte slice",
			data:     []byte{},
			expected: []byte{},
			wantErr:  false,
		},
		{
			name:        "string data",
			data:        "test string",
			wantErr:     true,
			errContains: "expected []byte, got string",
		},
		{
			name:        "int data",
			data:        123,
			wantErr:     true,
			errContains: "expected []byte, got int",
		},
		{
			name:        "nil data",
			data:        nil,
			wantErr:     true,
			errContains: "data is nil",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			media := &Media{
				MimeType: mime.MustNew("application", "octet-stream"),
				Data:     tt.data,
				Metadata: make(map[string]any),
			}

			result, err := media.DataAsBytes()

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
				assert.Nil(t, result)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestMedia_DataAsString(t *testing.T) {
	tests := []struct {
		name        string
		data        any
		expected    string
		wantErr     bool
		errContains string
	}{
		{
			name:     "valid string",
			data:     "test content",
			expected: "test content",
			wantErr:  false,
		},
		{
			name:     "empty string",
			data:     "",
			expected: "",
			wantErr:  false,
		},
		{
			name:        "byte slice data",
			data:        []byte{1, 2, 3},
			wantErr:     true,
			errContains: "expected string, got []uint8",
		},
		{
			name:        "int data",
			data:        456,
			wantErr:     true,
			errContains: "expected string, got int",
		},
		{
			name:        "nil data",
			data:        nil,
			wantErr:     true,
			errContains: "data is nil",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			media := &Media{
				MimeType: mime.MustNew("text", "plain"),
				Data:     tt.data,
				Metadata: make(map[string]any),
			}

			result, err := media.DataAsString()

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
				assert.Empty(t, result)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestMedia_Fields(t *testing.T) {
	t.Run("test all fields", func(t *testing.T) {
		mimeType := mime.MustNew("image", "png")
		data := []byte{137, 80, 78, 71}

		media, err := NewMedia(mimeType, data)
		require.NoError(t, err)

		// Test setting fields
		media.ID = "test-id-123"
		media.Name = "test-image.png"
		media.Metadata["width"] = 800
		media.Metadata["height"] = 600
		media.Metadata["author"] = "test user"

		// Verify fields
		assert.Equal(t, "test-id-123", media.ID)
		assert.Equal(t, "test-image.png", media.Name)
		assert.Equal(t, mimeType, media.MimeType)
		assert.Equal(t, data, media.Data)
		assert.Equal(t, 800, media.Metadata["width"])
		assert.Equal(t, 600, media.Metadata["height"])
		assert.Equal(t, "test user", media.Metadata["author"])
		assert.Len(t, media.Metadata, 3)
	})
}

func TestMedia_MetadataOperations(t *testing.T) {
	t.Run("metadata modifications", func(t *testing.T) {
		media, err := NewMedia(
			mime.MustNew("video", "mp4"),
			[]byte{0x00, 0x00, 0x00, 0x18},
		)
		require.NoError(t, err)

		// Add metadata
		media.Metadata["duration"] = 120
		media.Metadata["codec"] = "h264"
		assert.Len(t, media.Metadata, 2)

		// Update metadata
		media.Metadata["duration"] = 150
		assert.Equal(t, 150, media.Metadata["duration"])

		// Delete metadata
		delete(media.Metadata, "codec")
		assert.Len(t, media.Metadata, 1)
		_, exists := media.Metadata["codec"]
		assert.False(t, exists)
	})
}

func TestMedia_Integration(t *testing.T) {
	t.Run("complete workflow with string data", func(t *testing.T) {
		// Create media
		content := "Hello, World!"
		media, err := NewMedia(
			mime.MustNew("text", "plain"),
			content,
		)
		require.NoError(t, err)

		// Set properties
		media.ID = "doc-001"
		media.Name = "greeting.txt"
		media.Metadata["encoding"] = "utf-8"
		media.Metadata["language"] = "en"

		// Retrieve as string
		retrieved, err := media.DataAsString()
		require.NoError(t, err)
		assert.Equal(t, content, retrieved)

		// Should fail as bytes
		_, err = media.DataAsBytes()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "expected []byte, got string")
	})

	t.Run("complete workflow with byte data", func(t *testing.T) {
		// Create media
		data := []byte{0x89, 0x50, 0x4E, 0x47}
		media, err := NewMedia(
			mime.MustNew("image", "png"),
			data,
		)
		require.NoError(t, err)

		// Set properties
		media.ID = "img-001"
		media.Name = "logo.png"
		media.Metadata["size"] = len(data)

		// Retrieve as bytes
		retrieved, err := media.DataAsBytes()
		require.NoError(t, err)
		assert.Equal(t, data, retrieved)

		// Should fail as string
		_, err = media.DataAsString()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "expected string, got []uint8")
	})
}

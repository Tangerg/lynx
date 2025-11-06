package writers

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Tangerg/lynx/ai/media/document"
)

// TestFileWriterConfig_validate tests the validation logic
func TestFileWriterConfig_validate(t *testing.T) {
	tests := []struct {
		name        string
		config      *FileWriterConfig
		wantErr     bool
		errContains string
	}{
		{
			name: "valid config with all fields",
			config: &FileWriterConfig{
				Path:                "/tmp/test.txt",
				WithDocumentMarkers: true,
				AppendMode:          true,
			},
			wantErr: false,
		},
		{
			name: "valid config with minimal fields",
			config: &FileWriterConfig{
				Path: "/tmp/test.txt",
			},
			wantErr: false,
		},
		{
			name:        "nil config",
			config:      nil,
			wantErr:     true,
			errContains: "config is required",
		},
		{
			name: "empty path",
			config: &FileWriterConfig{
				Path:       "",
				AppendMode: true,
			},
			wantErr:     true,
			errContains: "file path is required",
		},
		{
			name: "whitespace path",
			config: &FileWriterConfig{
				Path: "   ",
			},
			wantErr: false, // whitespace is valid path technically
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.validate()

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestNewFileWriter tests the constructor
func TestNewFileWriter(t *testing.T) {
	tests := []struct {
		name        string
		config      *FileWriterConfig
		wantErr     bool
		errContains string
		checkFn     func(*testing.T, *FileWriter)
	}{
		{
			name: "valid writer with all options",
			config: &FileWriterConfig{
				Path:                "/tmp/test.txt",
				WithDocumentMarkers: true,
				AppendMode:          true,
			},
			wantErr: false,
			checkFn: func(t *testing.T, fw *FileWriter) {
				assert.Equal(t, "/tmp/test.txt", fw.path)
				assert.True(t, fw.withDocumentMarkers)
				assert.True(t, fw.appendMode)
			},
		},
		{
			name: "valid writer with minimal config",
			config: &FileWriterConfig{
				Path: "/tmp/minimal.txt",
			},
			wantErr: false,
			checkFn: func(t *testing.T, fw *FileWriter) {
				assert.Equal(t, "/tmp/minimal.txt", fw.path)
				assert.False(t, fw.withDocumentMarkers)
				assert.False(t, fw.appendMode)
			},
		},
		{
			name:        "nil config",
			config:      nil,
			wantErr:     true,
			errContains: "config is required",
		},
		{
			name: "empty path",
			config: &FileWriterConfig{
				Path: "",
			},
			wantErr:     true,
			errContains: "file path is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			writer, err := NewFileWriter(tt.config)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
				assert.Nil(t, writer)
			} else {
				require.NoError(t, err)
				require.NotNil(t, writer)
				if tt.checkFn != nil {
					tt.checkFn(t, writer)
				}
			}
		})
	}
}

// TestFileWriter_Write tests the Write method
func TestFileWriter_Write(t *testing.T) {
	t.Run("write single document without markers", func(t *testing.T) {
		tmpFile := createTempFile(t)
		defer os.Remove(tmpFile)

		config := &FileWriterConfig{
			Path:                tmpFile,
			WithDocumentMarkers: false,
			AppendMode:          false,
		}
		writer, err := NewFileWriter(config)
		require.NoError(t, err)

		doc := mustCreateDocument(t, "Hello World", nil)
		err = writer.Write(context.Background(), []*document.Document{doc})
		require.NoError(t, err)

		content := readFile(t, tmpFile)
		assert.Contains(t, content, "Hello World")
		assert.NotContains(t, content, "### Index:")
	})

	t.Run("write single document with markers", func(t *testing.T) {
		tmpFile := createTempFile(t)
		defer os.Remove(tmpFile)

		config := &FileWriterConfig{
			Path:                tmpFile,
			WithDocumentMarkers: true,
			AppendMode:          false,
		}
		writer, err := NewFileWriter(config)
		require.NoError(t, err)

		doc := mustCreateDocument(t, "Test content", nil)
		err = writer.Write(context.Background(), []*document.Document{doc})
		require.NoError(t, err)

		content := readFile(t, tmpFile)
		assert.Contains(t, content, "### Index: 0")
		assert.Contains(t, content, "Test content")
	})

	t.Run("write multiple documents", func(t *testing.T) {
		tmpFile := createTempFile(t)
		defer os.Remove(tmpFile)

		config := &FileWriterConfig{
			Path:                tmpFile,
			WithDocumentMarkers: true,
			AppendMode:          false,
		}
		writer, err := NewFileWriter(config)
		require.NoError(t, err)

		docs := []*document.Document{
			mustCreateDocument(t, "First document", nil),
			mustCreateDocument(t, "Second document", nil),
			mustCreateDocument(t, "Third document", nil),
		}
		err = writer.Write(context.Background(), docs)
		require.NoError(t, err)

		content := readFile(t, tmpFile)
		assert.Contains(t, content, "### Index: 0")
		assert.Contains(t, content, "### Index: 1")
		assert.Contains(t, content, "### Index: 2")
		assert.Contains(t, content, "First document")
		assert.Contains(t, content, "Second document")
		assert.Contains(t, content, "Third document")
	})

	t.Run("write with page metadata", func(t *testing.T) {
		tmpFile := createTempFile(t)
		defer os.Remove(tmpFile)

		config := &FileWriterConfig{
			Path:                tmpFile,
			WithDocumentMarkers: true,
			AppendMode:          false,
		}
		writer, err := NewFileWriter(config)
		require.NoError(t, err)

		doc := mustCreateDocument(t, "Document with pages", nil)
		doc.Metadata[MetadataStartPageNumber] = 1
		doc.Metadata[MetadataEndPageNumber] = 5

		err = writer.Write(context.Background(), []*document.Document{doc})
		require.NoError(t, err)

		content := readFile(t, tmpFile)
		assert.Contains(t, content, "### Index: 0, Pages:[1,5]")
		assert.Contains(t, content, "Document with pages")
	})

	t.Run("write in append mode", func(t *testing.T) {
		tmpFile := createTempFile(t)
		defer os.Remove(tmpFile)

		// Write initial content
		config1 := &FileWriterConfig{
			Path:       tmpFile,
			AppendMode: false,
		}
		writer1, err := NewFileWriter(config1)
		require.NoError(t, err)

		doc1 := mustCreateDocument(t, "Initial content", nil)
		err = writer1.Write(context.Background(), []*document.Document{doc1})
		require.NoError(t, err)

		// Append more content
		config2 := &FileWriterConfig{
			Path:       tmpFile,
			AppendMode: true,
		}
		writer2, err := NewFileWriter(config2)
		require.NoError(t, err)

		doc2 := mustCreateDocument(t, "Appended content", nil)
		err = writer2.Write(context.Background(), []*document.Document{doc2})
		require.NoError(t, err)

		content := readFile(t, tmpFile)
		assert.Contains(t, content, "Initial content")
		assert.Contains(t, content, "Appended content")
	})

	t.Run("write in truncate mode", func(t *testing.T) {
		tmpFile := createTempFile(t)
		defer os.Remove(tmpFile)

		// Write initial content
		err := os.WriteFile(tmpFile, []byte("Old content that will be removed"), 0666)
		require.NoError(t, err)

		// Write new content (should truncate)
		config := &FileWriterConfig{
			Path:       tmpFile,
			AppendMode: false,
		}
		writer, err := NewFileWriter(config)
		require.NoError(t, err)

		doc := mustCreateDocument(t, "New content", nil)
		err = writer.Write(context.Background(), []*document.Document{doc})
		require.NoError(t, err)

		content := readFile(t, tmpFile)
		assert.Contains(t, content, "New content")
		assert.NotContains(t, content, "Old content")
	})

	t.Run("write empty document list", func(t *testing.T) {
		tmpFile := createTempFile(t)
		defer os.Remove(tmpFile)

		config := &FileWriterConfig{
			Path: tmpFile,
		}
		writer, err := NewFileWriter(config)
		require.NoError(t, err)

		err = writer.Write(context.Background(), []*document.Document{})
		require.NoError(t, err)

		content := readFile(t, tmpFile)
		assert.Empty(t, content)
	})

	t.Run("write nil document list", func(t *testing.T) {
		tmpFile := createTempFile(t)
		defer os.Remove(tmpFile)

		config := &FileWriterConfig{
			Path: tmpFile,
		}
		writer, err := NewFileWriter(config)
		require.NoError(t, err)

		err = writer.Write(context.Background(), nil)
		require.NoError(t, err)
	})

	t.Run("write large batch of documents", func(t *testing.T) {
		tmpFile := createTempFile(t)
		defer os.Remove(tmpFile)

		config := &FileWriterConfig{
			Path:                tmpFile,
			WithDocumentMarkers: true,
		}
		writer, err := NewFileWriter(config)
		require.NoError(t, err)

		// Create 20 documents (larger than batch size of 5)
		docs := make([]*document.Document, 20)
		for i := 0; i < 20; i++ {
			docs[i] = mustCreateDocument(t, "Document "+string(rune('A'+i)), nil)
		}

		err = writer.Write(context.Background(), docs)
		require.NoError(t, err)

		content := readFile(t, tmpFile)
		for i := 0; i < 20; i++ {
			assert.Contains(t, content, "Document "+string(rune('A'+i)))
		}
	})

	t.Run("write to invalid path", func(t *testing.T) {
		config := &FileWriterConfig{
			Path: "/invalid/nonexistent/path/test.txt",
		}
		writer, err := NewFileWriter(config)
		require.NoError(t, err)

		doc := mustCreateDocument(t, "Test", nil)
		err = writer.Write(context.Background(), []*document.Document{doc})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to open file")
	})
}

// TestFileWriter_determineFileFlags tests the file flag determination
func TestFileWriter_determineFileFlags(t *testing.T) {
	tests := []struct {
		name       string
		appendMode bool
		expected   int
	}{
		{
			name:       "truncate mode",
			appendMode: false,
			expected:   os.O_CREATE | os.O_WRONLY | os.O_TRUNC,
		},
		{
			name:       "append mode",
			appendMode: true,
			expected:   os.O_CREATE | os.O_WRONLY | os.O_APPEND,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			writer := &FileWriter{
				appendMode: tt.appendMode,
			}

			flags := writer.determineFileFlags()
			assert.Equal(t, tt.expected, flags)
		})
	}
}

// TestFileWriter_buildDocumentContent tests content building
func TestFileWriter_buildDocumentContent(t *testing.T) {
	t.Run("without markers", func(t *testing.T) {
		writer := &FileWriter{
			withDocumentMarkers: false,
		}

		doc := mustCreateDocument(t, "Test content", nil)
		content := writer.buildDocumentContent(0, doc)

		assert.Contains(t, content, "Test content")
		assert.NotContains(t, content, "### Index:")
		assert.True(t, strings.HasSuffix(content, "\n\n"))
	})

	t.Run("with markers but no page metadata", func(t *testing.T) {
		writer := &FileWriter{
			withDocumentMarkers: true,
		}

		doc := mustCreateDocument(t, "Test content", nil)
		content := writer.buildDocumentContent(5, doc)

		assert.Contains(t, content, "### Index: 5")
		assert.Contains(t, content, "Test content")
		assert.NotContains(t, content, "Pages:")
	})

	t.Run("with markers and page metadata", func(t *testing.T) {
		writer := &FileWriter{
			withDocumentMarkers: true,
		}

		doc := mustCreateDocument(t, "Test content", nil)
		doc.Metadata[MetadataStartPageNumber] = 10
		doc.Metadata[MetadataEndPageNumber] = 15

		content := writer.buildDocumentContent(3, doc)

		assert.Contains(t, content, "### Index: 3, Pages:[10,15]")
		assert.Contains(t, content, "Test content")
	})

	t.Run("with markers and partial page metadata", func(t *testing.T) {
		writer := &FileWriter{
			withDocumentMarkers: true,
		}

		doc := mustCreateDocument(t, "Test content", nil)
		doc.Metadata[MetadataStartPageNumber] = 5
		// Missing end page

		content := writer.buildDocumentContent(0, doc)

		assert.Contains(t, content, "### Index: 0")
		assert.NotContains(t, content, "Pages:")
		assert.Contains(t, content, "Test content")
	})

	t.Run("with nil metadata", func(t *testing.T) {
		writer := &FileWriter{
			withDocumentMarkers: true,
		}

		doc := mustCreateDocument(t, "Test content", nil)
		doc.Metadata = nil

		content := writer.buildDocumentContent(0, doc)

		assert.Contains(t, content, "### Index: 0")
		assert.NotContains(t, content, "Pages:")
		assert.Contains(t, content, "Test content")
	})
}

// TestFileWriter_InterfaceCompliance verifies interface implementation
func TestFileWriter_InterfaceCompliance(t *testing.T) {
	config := &FileWriterConfig{
		Path: "/tmp/test.txt",
	}
	writer, err := NewFileWriter(config)
	require.NoError(t, err)

	var _ document.Writer = writer
}

// TestFileWriter_Integration tests complete workflows
func TestFileWriter_Integration(t *testing.T) {
	t.Run("complete write workflow with all features", func(t *testing.T) {
		tmpFile := createTempFile(t)
		defer os.Remove(tmpFile)

		// Create writer with all features enabled
		config := &FileWriterConfig{
			Path:                tmpFile,
			WithDocumentMarkers: true,
			AppendMode:          false,
		}
		writer, err := NewFileWriter(config)
		require.NoError(t, err)

		// Create documents with various metadata
		doc1 := mustCreateDocument(t, "Chapter 1: Introduction", nil)
		doc1.Metadata[MetadataStartPageNumber] = 1
		doc1.Metadata[MetadataEndPageNumber] = 10
		doc1.Metadata["author"] = "John Doe"

		doc2 := mustCreateDocument(t, "Chapter 2: Methods", nil)
		doc2.Metadata[MetadataStartPageNumber] = 11
		doc2.Metadata[MetadataEndPageNumber] = 25

		doc3 := mustCreateDocument(t, "Chapter 3: Results", nil)
		// No page metadata

		// Write documents
		err = writer.Write(context.Background(), []*document.Document{doc1, doc2, doc3})
		require.NoError(t, err)

		// Verify content
		content := readFile(t, tmpFile)

		// Check all chapters are present
		assert.Contains(t, content, "Chapter 1: Introduction")
		assert.Contains(t, content, "Chapter 2: Methods")
		assert.Contains(t, content, "Chapter 3: Results")

		// Check markers
		assert.Contains(t, content, "### Index: 0, Pages:[1,10]")
		assert.Contains(t, content, "### Index: 1, Pages:[11,25]")
		assert.Contains(t, content, "### Index: 2")
		assert.NotContains(t, strings.Split(content, "Chapter 3")[1], "Pages:")
	})

	t.Run("multiple write operations with append", func(t *testing.T) {
		tmpFile := createTempFile(t)
		defer os.Remove(tmpFile)

		// First write
		config1 := &FileWriterConfig{
			Path:                tmpFile,
			WithDocumentMarkers: true,
			AppendMode:          false,
		}
		writer1, err := NewFileWriter(config1)
		require.NoError(t, err)

		batch1 := []*document.Document{
			mustCreateDocument(t, "Batch 1 Doc 1", nil),
			mustCreateDocument(t, "Batch 1 Doc 2", nil),
		}
		err = writer1.Write(context.Background(), batch1)
		require.NoError(t, err)

		// Second write (append)
		config2 := &FileWriterConfig{
			Path:                tmpFile,
			WithDocumentMarkers: true,
			AppendMode:          true,
		}
		writer2, err := NewFileWriter(config2)
		require.NoError(t, err)

		batch2 := []*document.Document{
			mustCreateDocument(t, "Batch 2 Doc 1", nil),
			mustCreateDocument(t, "Batch 2 Doc 2", nil),
		}
		err = writer2.Write(context.Background(), batch2)
		require.NoError(t, err)

		// Verify all content is present
		content := readFile(t, tmpFile)
		assert.Contains(t, content, "Batch 1 Doc 1")
		assert.Contains(t, content, "Batch 1 Doc 2")
		assert.Contains(t, content, "Batch 2 Doc 1")
		assert.Contains(t, content, "Batch 2 Doc 2")
	})
}

// Test helper functions

func createTempFile(t *testing.T) string {
	t.Helper()
	tmpDir := t.TempDir()
	return filepath.Join(tmpDir, "test.txt")
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(content)
}

func mustCreateDocument(t *testing.T, text string, media interface{}) *document.Document {
	t.Helper()
	doc, err := document.NewDocument(text, nil)
	require.NoError(t, err)
	return doc
}

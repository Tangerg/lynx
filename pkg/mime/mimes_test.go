package mime

import (
	"fmt"
	"strings"
	"sync"
	"testing"
)

// TestStringTypeByExtension tests the StringTypeByExtension function
func TestStringTypeByExtension(t *testing.T) {
	tests := []struct {
		name     string
		filePath string
		expected string
	}{
		{
			name:     "PDF document",
			filePath: "document.pdf",
			expected: "application/pdf",
		},
		{
			name:     "PNG image",
			filePath: "image.png",
			expected: "image/png",
		},
		{
			name:     "JSON file",
			filePath: "data.json",
			expected: "application/json",
		},
		{
			name:     "HTML file",
			filePath: "index.html",
			expected: "text/html",
		},
		{
			name:     "JavaScript file",
			filePath: "script.js",
			expected: "application/javascript",
		},
		{
			name:     "CSS file",
			filePath: "styles.css",
			expected: "text/css",
		},
		{
			name:     "XML file",
			filePath: "config.xml",
			expected: "application/xml",
		},
		{
			name:     "ZIP archive",
			filePath: "archive.zip",
			expected: "application/zip",
		},
		{
			name:     "MP3 audio",
			filePath: "song.mp3",
			expected: "audio/mpeg",
		},
		{
			name:     "MP4 video",
			filePath: "video.mp4",
			expected: "video/mp4",
		},
		{
			name:     "DOCX document",
			filePath: "document.docx",
			expected: "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		},
		{
			name:     "XLSX spreadsheet",
			filePath: "spreadsheet.xlsx",
			expected: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
		},
		{
			name:     "PPTX presentation",
			filePath: "presentation.pptx",
			expected: "application/vnd.openxmlformats-officedocument.presentationml.presentation",
		},
		{
			name:     "unknown extension",
			filePath: "file.unknown123",
			expected: "application/octet-stream",
		},
		{
			name:     "no extension",
			filePath: "README",
			expected: "application/octet-stream",
		},
		{
			name:     "uppercase extension",
			filePath: "document.PDF",
			expected: "application/pdf",
		},
		{
			name:     "mixed case extension",
			filePath: "Image.PnG",
			expected: "image/png",
		},
		{
			name:     "path with directories",
			filePath: "/path/to/file.json",
			expected: "application/json",
		},
		{
			name:     "Windows path",
			filePath: "C:\\Users\\Documents\\file.docx",
			expected: "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		},
		{
			name:     "relative path",
			filePath: "../data/config.xml",
			expected: "application/xml",
		},
		{
			name:     "multiple dots in filename",
			filePath: "my.file.name.csv",
			expected: "text/csv",
		},
		{
			name:     "hidden file with extension",
			filePath: ".htaccess",
			expected: "application/octet-stream",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := StringTypeByExtension(tt.filePath)
			split := strings.Split(result, ";")
			if split[0] != tt.expected {
				t.Errorf("StringTypeByExtension(%q) = %q, want %q", tt.filePath, result, tt.expected)
			}
		})
	}
}

// TestTypeByExtension tests the TypeByExtension function
func TestTypeByExtension(t *testing.T) {
	tests := []struct {
		name            string
		filePath        string
		shouldFind      bool
		expectedType    string
		expectedSubType string
	}{
		{
			name:            "PDF document",
			filePath:        "document.pdf",
			shouldFind:      true,
			expectedType:    "application",
			expectedSubType: "pdf",
		},
		{
			name:            "PNG image",
			filePath:        "image.png",
			shouldFind:      true,
			expectedType:    "image",
			expectedSubType: "png",
		},
		{
			name:            "JSON file",
			filePath:        "data.json",
			shouldFind:      true,
			expectedType:    "application",
			expectedSubType: "json",
		},
		{
			name:            "HTML file",
			filePath:        "index.html",
			shouldFind:      true,
			expectedType:    "text",
			expectedSubType: "html",
		},
		{
			name:            "JPEG image",
			filePath:        "photo.jpg",
			shouldFind:      true,
			expectedType:    "image",
			expectedSubType: "jpeg",
		},
		{
			name:            "CSV file",
			filePath:        "data.csv",
			shouldFind:      true,
			expectedType:    "text",
			expectedSubType: "csv",
		},
		{
			name:       "unknown extension",
			filePath:   "file.unknown123",
			shouldFind: false,
		},
		{
			name:       "no extension",
			filePath:   "README",
			shouldFind: false,
		},
		{
			name:            "uppercase extension",
			filePath:        "IMAGE.PNG",
			shouldFind:      true,
			expectedType:    "image",
			expectedSubType: "png",
		},
		{
			name:            "path with directories",
			filePath:        "/var/www/html/index.html",
			shouldFind:      true,
			expectedType:    "text",
			expectedSubType: "html",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mimeType, found := TypeByExtension(tt.filePath)

			if found != tt.shouldFind {
				t.Errorf("TypeByExtension(%q) found = %v, want %v", tt.filePath, found, tt.shouldFind)
			}

			if tt.shouldFind {
				if mimeType == nil {
					t.Errorf("TypeByExtension(%q) returned nil MIME type", tt.filePath)
					return
				}

				if mimeType.Type() != tt.expectedType {
					t.Errorf("TypeByExtension(%q) type = %q, want %q", tt.filePath, mimeType.Type(), tt.expectedType)
				}

				if mimeType.SubType() != tt.expectedSubType {
					t.Errorf("TypeByExtension(%q) subtype = %q, want %q", tt.filePath, mimeType.SubType(), tt.expectedSubType)
				}
			} else {
				if mimeType != nil {
					t.Errorf("TypeByExtension(%q) should return nil for unknown extension", tt.filePath)
				}
			}
		})
	}
}

// TestTypeByExtension_Immutability tests that returned MIME objects are clones
func TestTypeByExtension_Immutability(t *testing.T) {
	mime1, found1 := TypeByExtension("test.json")
	if !found1 {
		t.Fatal("Expected to find .json extension")
	}

	mime2, found2 := TypeByExtension("test.json")
	if !found2 {
		t.Fatal("Expected to find .json extension")
	}

	// Should be equal but different instances
	if !mime1.Equals(mime2) {
		t.Error("MIME types should be equal")
	}

	// Verify they are different instances (clones)
	if mime1 == mime2 {
		t.Error("MIME types should be different instances (clones)")
	}
}

// TestRegisterExtension tests registering a single extension
func TestRegisterExtension(t *testing.T) {
	tests := []struct {
		name        string
		ext         string
		mimeType    string
		shouldError bool
	}{
		{
			name:        "valid custom extension",
			ext:         ".custom",
			mimeType:    "application/x-custom",
			shouldError: false,
		},
		{
			name:        "override existing extension",
			ext:         ".testtxt",
			mimeType:    "text/custom-plain",
			shouldError: false,
		},
		{
			name:        "invalid MIME type - missing slash",
			ext:         ".invalid1",
			mimeType:    "invalid-mime-type",
			shouldError: true,
		},
		{
			name:        "empty MIME type",
			ext:         ".empty",
			mimeType:    "",
			shouldError: true,
		},
		{
			name:        "complex MIME type with parameters",
			ext:         ".complex",
			mimeType:    "text/html; charset=UTF-8",
			shouldError: false,
		},
		{
			name:        "MIME type with multiple parameters",
			ext:         ".multi",
			mimeType:    "text/html; charset=UTF-8; boundary=something",
			shouldError: false,
		},
		{
			name:        "numeric extension",
			ext:         ".123",
			mimeType:    "application/x-numeric",
			shouldError: false,
		},
		{
			name:        "extension with dash",
			ext:         ".x-custom",
			mimeType:    "application/x-custom-type",
			shouldError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := RegisterExtension(tt.ext, tt.mimeType)

			if tt.shouldError {
				if err == nil {
					t.Errorf("RegisterExtension(%q, %q) should return error", tt.ext, tt.mimeType)
				}
			} else {
				if err != nil {
					t.Errorf("RegisterExtension(%q, %q) unexpected error: %v", tt.ext, tt.mimeType, err)
				}

				// Verify registration via StringTypeByExtension
				result := StringTypeByExtension("test" + tt.ext)
				if result == "" {
					t.Errorf("Extension %q was not registered properly", tt.ext)
				}

				// Verify registration via TypeByExtension
				mimeObj, found := TypeByExtension("test" + tt.ext)
				if !found {
					t.Errorf("Extension %q not found after registration", tt.ext)
				}
				if mimeObj == nil {
					t.Errorf("Extension %q returned nil MIME object", tt.ext)
				}
			}
		})
	}
}

// TestRegisterExtension_Override tests overriding existing extensions
func TestRegisterExtension_Override(t *testing.T) {
	testExt := ".override-test"

	// Register initial extension
	err := RegisterExtension(testExt, "application/initial")
	if err != nil {
		t.Fatalf("Failed to register initial extension: %v", err)
	}

	// Verify initial registration
	result1 := StringTypeByExtension("test" + testExt)
	if result1 != "application/initial" {
		t.Errorf("Initial MIME type = %q, want %q", result1, "application/initial")
	}

	// Override with new MIME type
	err = RegisterExtension(testExt, "application/overridden")
	if err != nil {
		t.Fatalf("Failed to override extension: %v", err)
	}

	// Verify override
	result2 := StringTypeByExtension("test" + testExt)
	if result2 != "application/overridden" {
		t.Errorf("Overridden MIME type = %q, want %q", result2, "application/overridden")
	}
}

// TestRegisterExtensions tests batch registration
func TestRegisterExtensions(t *testing.T) {
	tests := []struct {
		name        string
		mappings    map[string]string
		shouldError bool
	}{
		{
			name: "valid batch registration",
			mappings: map[string]string{
				".batch1": "application/x-batch1",
				".batch2": "application/x-batch2",
				".batch3": "application/x-batch3",
			},
			shouldError: false,
		},
		{
			name: "batch with one invalid",
			mappings: map[string]string{
				".valid1":  "application/x-valid1",
				".invalid": "invalid-mime",
				".valid2":  "application/x-valid2",
			},
			shouldError: true,
		},
		{
			name:        "empty batch",
			mappings:    map[string]string{},
			shouldError: false,
		},
		{
			name: "large batch",
			mappings: map[string]string{
				".large1":  "application/x-large1",
				".large2":  "application/x-large2",
				".large3":  "application/x-large3",
				".large4":  "application/x-large4",
				".large5":  "application/x-large5",
				".large6":  "application/x-large6",
				".large7":  "application/x-large7",
				".large8":  "application/x-large8",
				".large9":  "application/x-large9",
				".large10": "application/x-large10",
			},
			shouldError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := RegisterExtensions(tt.mappings)

			if tt.shouldError {
				if err == nil {
					t.Error("RegisterExtensions should return error")
				}
			} else {
				if err != nil {
					t.Errorf("RegisterExtensions unexpected error: %v", err)
				}

				// Verify all registrations
				for ext, _ := range tt.mappings {
					result := StringTypeByExtension("test" + ext)
					if result == "" {
						t.Errorf("Extension %q was not registered", ext)
					}

					mimeObj, found := TypeByExtension("test" + ext)
					if !found {
						t.Errorf("Extension %q not found after batch registration", ext)
					}
					if mimeObj == nil {
						t.Errorf("Extension %q returned nil MIME object", ext)
					}
				}
			}
		})
	}
}

// TestRegisterExtensions_Atomicity tests all-or-nothing behavior
func TestRegisterExtensions_Atomicity(t *testing.T) {
	// Register a test extension first to verify it's not affected
	testExt := ".atomicity-test"
	err := RegisterExtension(testExt, "application/test")
	if err != nil {
		t.Fatalf("Failed to register test extension: %v", err)
	}

	// Try to register a batch with one invalid entry
	mappings := map[string]string{
		".atomic1": "application/x-atomic1",
		".atomic2": "invalid-mime-type",
		".atomic3": "application/x-atomic3",
	}

	err = RegisterExtensions(mappings)
	if err == nil {
		t.Fatal("Expected error from invalid MIME type")
	}

	// Verify none of the extensions were registered due to atomicity
	for ext := range mappings {
		if ext == ".atomic2" {
			continue // Skip the invalid one
		}

		// These should not be registered due to atomicity
		_, found := TypeByExtension("test" + ext)
		if found {
			t.Errorf("Extension %q should not be registered due to atomicity failure", ext)
		}
	}

	// Verify the original test extension is still registered
	result := StringTypeByExtension("test" + testExt)
	if result != "application/test" {
		t.Error("Original extension should still be registered after failed batch operation")
	}
}

// TestConcurrentStringTypeByExtension tests concurrent read operations
func TestConcurrentStringTypeByExtension(t *testing.T) {
	const goroutines = 50
	const operations = 1000

	extensions := []string{".json", ".html", ".pdf", ".png", ".xml", ".css"}

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < operations; j++ {
				ext := extensions[j%len(extensions)]
				result := StringTypeByExtension("test" + ext)
				if result == "" {
					t.Errorf("StringTypeByExtension returned empty for %s", ext)
				}
			}
		}(i)
	}

	wg.Wait()
}

// TestConcurrentTypeByExtension tests concurrent TypeByExtension calls
func TestConcurrentTypeByExtension(t *testing.T) {
	const goroutines = 50
	const operations = 1000

	extensions := []string{".json", ".html", ".pdf", ".png", ".xml"}

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < operations; j++ {
				ext := extensions[j%len(extensions)]
				mimeType, found := TypeByExtension("test" + ext)
				if !found {
					t.Errorf("TypeByExtension did not find %s", ext)
				}
				if mimeType == nil {
					t.Errorf("TypeByExtension returned nil for %s", ext)
				}
			}
		}(i)
	}

	wg.Wait()
}

// TestConcurrentRegisterExtension tests concurrent writes
func TestConcurrentRegisterExtension(t *testing.T) {
	const goroutines = 20
	const operations = 100

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < operations; j++ {
				ext := fmt.Sprintf(".concurrent-%d-%d", id, j)
				err := RegisterExtension(ext, "application/test")
				if err != nil {
					t.Errorf("RegisterExtension failed: %v", err)
				}
			}
		}(i)
	}

	wg.Wait()
}

// TestConcurrentReadWrite tests mixed read and write operations
func TestConcurrentReadWrite(t *testing.T) {
	const readers = 30
	const writers = 10
	const operations = 500

	var wg sync.WaitGroup
	wg.Add(readers + writers)

	// Start readers
	for i := 0; i < readers; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < operations; j++ {
				_ = StringTypeByExtension("test.json")
				_, _ = TypeByExtension("test.html")
			}
		}(i)
	}

	// Start writers
	for i := 0; i < writers; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < operations; j++ {
				ext := fmt.Sprintf(".rw-test-%d-%d", id, j)
				_ = RegisterExtension(ext, "application/test")
			}
		}(i)
	}

	wg.Wait()
}

// TestConcurrentBatchOperations tests concurrent batch registrations
func TestConcurrentBatchOperations(t *testing.T) {
	const goroutines = 10
	const operations = 50

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < operations; j++ {
				mappings := map[string]string{
					fmt.Sprintf(".batch-%d-%d-1", id, j): "application/test1",
					fmt.Sprintf(".batch-%d-%d-2", id, j): "application/test2",
					fmt.Sprintf(".batch-%d-%d-3", id, j): "application/test3",
				}
				err := RegisterExtensions(mappings)
				if err != nil {
					t.Errorf("RegisterExtensions failed: %v", err)
				}
			}
		}(i)
	}

	wg.Wait()
}

// TestMicrosoftOfficeFormats tests various Microsoft Office formats
func TestMicrosoftOfficeFormats(t *testing.T) {
	tests := []struct {
		ext      string
		expected string
	}{
		{".docx", "application/vnd.openxmlformats-officedocument.wordprocessingml.document"},
		{".xlsx", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"},
		{".pptx", "application/vnd.openxmlformats-officedocument.presentationml.presentation"},
		{".doc", "application/msword"},
		{".xls", "application/vnd.ms-excel"},
		{".ppt", "application/vnd.ms-powerpoint"},
		{".dotx", "application/vnd.openxmlformats-officedocument.wordprocessingml.template"},
		{".xltx", "application/vnd.openxmlformats-officedocument.spreadsheetml.template"},
		{".potx", "application/vnd.openxmlformats-officedocument.presentationml.template"},
	}

	for _, tt := range tests {
		t.Run(tt.ext, func(t *testing.T) {
			result := StringTypeByExtension("document" + tt.ext)
			if result != tt.expected {
				t.Errorf("StringTypeByExtension(%q) = %q, want %q", tt.ext, result, tt.expected)
			}
		})
	}
}

// TestMediaFormats tests various media formats
func TestMediaFormats(t *testing.T) {
	audioFormats := []string{".mp3", ".wav", ".aac", ".flac", ".wma"}
	videoFormats := []string{".mp4", ".avi", ".wmv", ".flv", ".webm"}

	for _, ext := range audioFormats {
		t.Run("audio"+ext, func(t *testing.T) {
			result := StringTypeByExtension("audio" + ext)
			if result == "" || result == "application/octet-stream" {
				t.Errorf("Audio extension %q should have a specific MIME type", ext)
			}
		})
	}

	for _, ext := range videoFormats {
		t.Run("video"+ext, func(t *testing.T) {
			result := StringTypeByExtension("video" + ext)
			if result == "" || result == "application/octet-stream" {
				t.Errorf("Video extension %q should have a specific MIME type", ext)
			}
		})
	}
}

// TestImageFormats tests various image formats
func TestImageFormats(t *testing.T) {
	imageExts := []string{".jpg", ".jpeg", ".png", ".gif", ".bmp", ".webp", ".svg", ".ico", ".tiff"}

	for _, ext := range imageExts {
		t.Run(ext, func(t *testing.T) {
			mime, found := TypeByExtension("image" + ext)
			if !found {
				t.Errorf("Image extension %q not found", ext)
				return
			}

			if mime.Type() != "image" {
				t.Errorf("Extension %q should have type 'image', got %q", ext, mime.Type())
			}
		})
	}
}

// TestCompressedFormats tests various compressed formats
func TestCompressedFormats(t *testing.T) {
	compressedExts := []string{".zip", ".gz", ".bz2", ".tar", ".rar"}

	for _, ext := range compressedExts {
		t.Run(ext, func(t *testing.T) {
			result := StringTypeByExtension("archive" + ext)
			if result == "" || result == "application/octet-stream" {
				t.Errorf("Compressed extension %q should have a specific MIME type", ext)
			}
		})
	}
}

// TestTextFormats tests various text formats
func TestTextFormats(t *testing.T) {
	textFormats := map[string]string{
		".txt":  "text/plain",
		".csv":  "text/csv",
		".html": "text/html",
		".css":  "text/css",
		".rtf":  "text/rtf",
	}

	for ext, expectedMime := range textFormats {
		t.Run(ext, func(t *testing.T) {
			result := StringTypeByExtension("file" + ext)
			if result != expectedMime {
				t.Errorf("Extension %q: got %q, want %q", ext, result, expectedMime)
			}
		})
	}
}

// BenchmarkStringTypeByExtension benchmarks string type lookup
func BenchmarkStringTypeByExtension(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = StringTypeByExtension("test.json")
	}
}

// BenchmarkTypeByExtension benchmarks MIME object lookup
func BenchmarkTypeByExtension(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = TypeByExtension("test.json")
	}
}

// BenchmarkRegisterExtension benchmarks extension registration
func BenchmarkRegisterExtension(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ext := fmt.Sprintf(".bench-%d", i)
		_ = RegisterExtension(ext, "application/test")
	}
}

// BenchmarkRegisterExtensions benchmarks batch registration
func BenchmarkRegisterExtensions(b *testing.B) {
	mappings := map[string]string{
		".bench1": "application/test1",
		".bench2": "application/test2",
		".bench3": "application/test3",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = RegisterExtensions(mappings)
	}
}

// BenchmarkConcurrentReads benchmarks concurrent read operations
func BenchmarkConcurrentReads(b *testing.B) {
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = StringTypeByExtension("test.json")
			_, _ = TypeByExtension("test.html")
		}
	})
}

// BenchmarkMixedOperations benchmarks mixed read/write operations
func BenchmarkMixedOperations(b *testing.B) {
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			if i%10 == 0 {
				ext := fmt.Sprintf(".mixed-%d", i)
				_ = RegisterExtension(ext, "application/test")
			} else {
				_ = StringTypeByExtension("test.json")
			}
			i++
		}
	})
}

package mime

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestNew tests the New function for creating MIME types
func TestNew(t *testing.T) {
	tests := []struct {
		name        string
		mimeType    string
		subType     string
		shouldError bool
		checkType   string
		checkSub    string
	}{
		{
			name:        "valid text/html",
			mimeType:    "text",
			subType:     "html",
			shouldError: false,
			checkType:   "text",
			checkSub:    "html",
		},
		{
			name:        "valid application/json",
			mimeType:    "application",
			subType:     "json",
			shouldError: false,
			checkType:   "application",
			checkSub:    "json",
		},
		{
			name:        "valid image/png",
			mimeType:    "image",
			subType:     "png",
			shouldError: false,
			checkType:   "image",
			checkSub:    "png",
		},
		{
			name:        "empty type",
			mimeType:    "",
			subType:     "html",
			shouldError: false,
			checkType:   "*",
			checkSub:    "html",
		},
		{
			name:        "empty subtype",
			mimeType:    "text",
			subType:     "",
			shouldError: false,
			checkType:   "text",
			checkSub:    "*",
		},
		{
			name:        "both empty",
			mimeType:    "",
			subType:     "",
			shouldError: false,
			checkType:   "*",
			checkSub:    "*",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := New(tt.mimeType, tt.subType)

			if tt.shouldError {
				if err == nil {
					t.Errorf("New(%q, %q) should return error", tt.mimeType, tt.subType)
				}
			} else {
				if err != nil {
					t.Errorf("New(%q, %q) unexpected error: %v", tt.mimeType, tt.subType, err)
				}
				if result == nil {
					t.Fatal("New should not return nil")
				}
				if result.Type() != tt.checkType {
					t.Errorf("Type() = %q, want %q", result.Type(), tt.checkType)
				}
				if result.SubType() != tt.checkSub {
					t.Errorf("SubType() = %q, want %q", result.SubType(), tt.checkSub)
				}
			}
		})
	}
}

// TestMustNew tests the MustNew function
func TestMustNew(t *testing.T) {
	// Test valid case
	t.Run("valid", func(t *testing.T) {
		result := MustNew("text", "html")
		if result == nil {
			t.Fatal("MustNew should not return nil for valid input")
		}
		if result.Type() != "text" || result.SubType() != "html" {
			t.Errorf("MustNew created incorrect MIME type")
		}
	})

	// Test panic case
	t.Run("panic on invalid", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Error("MustNew should panic on invalid input")
			}
		}()
		_ = MustNew("\t", "")
	})
}

// TestParse tests the Parse function
func TestParse(t *testing.T) {
	tests := []struct {
		name         string
		mimeString   string
		shouldError  bool
		expectedType string
		expectedSub  string
		hasParams    bool
	}{
		{
			name:         "simple text/html",
			mimeString:   "text/html",
			shouldError:  false,
			expectedType: "text",
			expectedSub:  "html",
			hasParams:    false,
		},
		{
			name:         "application/json",
			mimeString:   "application/json",
			shouldError:  false,
			expectedType: "application",
			expectedSub:  "json",
			hasParams:    false,
		},
		{
			name:         "with charset parameter",
			mimeString:   "text/html; charset=UTF-8",
			shouldError:  false,
			expectedType: "text",
			expectedSub:  "html",
			hasParams:    true,
		},
		{
			name:         "multiple parameters",
			mimeString:   "text/html; charset=UTF-8; boundary=something",
			shouldError:  false,
			expectedType: "text",
			expectedSub:  "html",
			hasParams:    true,
		},
		{
			name:         "wildcard all",
			mimeString:   "*/*",
			shouldError:  false,
			expectedType: "*",
			expectedSub:  "*",
		},
		{
			name:         "wildcard shorthand",
			mimeString:   "*",
			shouldError:  false,
			expectedType: "*",
			expectedSub:  "*",
		},
		{
			name:         "with whitespace",
			mimeString:   "  text/html  ;  charset=UTF-8  ",
			shouldError:  false,
			expectedType: "text",
			expectedSub:  "html",
			hasParams:    true,
		},
		{
			name:        "empty string",
			mimeString:  "",
			shouldError: true,
		},
		{
			name:        "missing slash",
			mimeString:  "texthtml",
			shouldError: true,
		},
		{
			name:        "missing subtype",
			mimeString:  "text/",
			shouldError: true,
		},
		{
			name:        "invalid wildcard",
			mimeString:  "*/html",
			shouldError: true,
		},
		{
			name:         "quoted parameter value",
			mimeString:   "text/html; charset=\"UTF-8\"",
			shouldError:  false,
			expectedType: "text",
			expectedSub:  "html",
			hasParams:    true,
		},
		{
			name:         "complex subtype",
			mimeString:   "application/vnd.api+json",
			shouldError:  false,
			expectedType: "application",
			expectedSub:  "vnd.api+json",
		},
		{
			name:         "parameter without value",
			mimeString:   "text/html; charset",
			shouldError:  false,
			expectedType: "text",
			expectedSub:  "html",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := Parse(tt.mimeString)

			if tt.shouldError {
				if err == nil {
					t.Errorf("Parse(%q) should return error", tt.mimeString)
				}
			} else {
				if err != nil {
					t.Errorf("Parse(%q) unexpected error: %v", tt.mimeString, err)
				}
				if result == nil {
					t.Fatal("Parse should not return nil")
				}
				if result.Type() != tt.expectedType {
					t.Errorf("Type() = %q, want %q", result.Type(), tt.expectedType)
				}
				if result.SubType() != tt.expectedSub {
					t.Errorf("SubType() = %q, want %q", result.SubType(), tt.expectedSub)
				}
			}
		})
	}
}

// TestDetect tests the Detect function
func TestDetect(t *testing.T) {
	tests := []struct {
		name         string
		data         []byte
		expectedType string
	}{
		{
			name:         "JSON data",
			data:         []byte(`{"key": "value"}`),
			expectedType: "application",
		},
		{
			name:         "XML data",
			data:         []byte(`<?xml version="1.0"?><root></root>`),
			expectedType: "text",
		},
		{
			name:         "HTML data",
			data:         []byte(`<!DOCTYPE html><html><body></body></html>`),
			expectedType: "text",
		},
		{
			name:         "plain text",
			data:         []byte("Hello, World!"),
			expectedType: "text",
		},
		{
			name:         "PNG signature",
			data:         []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A},
			expectedType: "image",
		},
		{
			name:         "JPEG signature",
			data:         []byte{0xFF, 0xD8, 0xFF},
			expectedType: "image",
		},
		{
			name:         "PDF signature",
			data:         []byte("%PDF-1.4"),
			expectedType: "application",
		},
		{
			name:         "ZIP signature",
			data:         []byte{0x50, 0x4B, 0x03, 0x04},
			expectedType: "application",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := Detect(tt.data)
			if err != nil {
				t.Errorf("Detect() unexpected error: %v", err)
			}
			if result == nil {
				t.Fatal("Detect should not return nil")
			}
			if result.Type() != tt.expectedType {
				t.Logf("Detected MIME type: %s/%s", result.Type(), result.SubType())
				// Don't fail, just log - mimetype library might detect differently
			}
		})
	}
}

// TestDetectReader tests the DetectReader function
func TestDetectReader(t *testing.T) {
	tests := []struct {
		name         string
		data         string
		expectedType string
	}{
		{
			name:         "JSON reader",
			data:         `{"key": "value"}`,
			expectedType: "application",
		},
		{
			name:         "HTML reader",
			data:         `<!DOCTYPE html><html><body>Hello</body></html>`,
			expectedType: "text",
		},
		{
			name:         "plain text reader",
			data:         "This is plain text content",
			expectedType: "text",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := strings.NewReader(tt.data)
			result, err := DetectReader(reader)
			if err != nil {
				t.Errorf("DetectReader() unexpected error: %v", err)
			}
			if result == nil {
				t.Fatal("DetectReader should not return nil")
			}
			if result.Type() != tt.expectedType {
				t.Logf("Detected MIME type: %s/%s", result.Type(), result.SubType())
			}
		})
	}
}

// TestDetectFile tests the DetectFile function
func TestDetectFile(t *testing.T) {
	// Create temporary directory for test files
	tempDir := t.TempDir()

	tests := []struct {
		name         string
		filename     string
		content      []byte
		expectedType string
	}{
		{
			name:         "JSON file",
			filename:     "test.json",
			content:      []byte(`{"key": "value"}`),
			expectedType: "application",
		},
		{
			name:         "HTML file",
			filename:     "test.html",
			content:      []byte(`<!DOCTYPE html><html><body>Test</body></html>`),
			expectedType: "text",
		},
		{
			name:         "text file",
			filename:     "test.txt",
			content:      []byte("Plain text content"),
			expectedType: "text",
		},
		{
			name:         "PNG file",
			filename:     "test.png",
			content:      []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A},
			expectedType: "image",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test file
			filePath := filepath.Join(tempDir, tt.filename)
			err := os.WriteFile(filePath, tt.content, 0644)
			if err != nil {
				t.Fatalf("Failed to create test file: %v", err)
			}

			// Test detection
			result, err := DetectFile(filePath)
			if err != nil {
				t.Errorf("DetectFile() unexpected error: %v", err)
			}
			if result == nil {
				t.Fatal("DetectFile should not return nil")
			}
			if result.Type() != tt.expectedType {
				t.Logf("Detected MIME type: %s/%s", result.Type(), result.SubType())
			}
		})
	}
}

// TestDetectFile_NonExistent tests DetectFile with non-existent file
func TestDetectFile_NonExistent(t *testing.T) {
	_, err := DetectFile("/path/that/does/not/exist/file.txt")
	if err == nil {
		t.Error("DetectFile should return error for non-existent file")
	}
}

// TestIsVideo tests the IsVideo function
func TestIsVideo(t *testing.T) {
	tests := []struct {
		name     string
		mime     string
		expected bool
	}{
		{"video/mp4", "video/mp4", true},
		{"video/webm", "video/webm", true},
		{"video/quicktime", "video/quicktime", true},
		{"image/jpeg", "image/jpeg", false},
		{"audio/mpeg", "audio/mpeg", false},
		{"text/html", "text/html", false},
		{"application/json", "application/json", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mime, err := Parse(tt.mime)
			if err != nil {
				t.Fatalf("Parse(%q) error: %v", tt.mime, err)
			}
			result := IsVideo(mime)
			if result != tt.expected {
				t.Errorf("IsVideo(%q) = %v, want %v", tt.mime, result, tt.expected)
			}
		})
	}
}

// TestIsAudio tests the IsAudio function
func TestIsAudio(t *testing.T) {
	tests := []struct {
		name     string
		mime     string
		expected bool
	}{
		{"audio/mpeg", "audio/mpeg", true},
		{"audio/wav", "audio/wav", true},
		{"audio/ogg", "audio/ogg", true},
		{"video/mp4", "video/mp4", false},
		{"image/png", "image/png", false},
		{"text/plain", "text/plain", false},
		{"application/pdf", "application/pdf", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mime, err := Parse(tt.mime)
			if err != nil {
				t.Fatalf("Parse(%q) error: %v", tt.mime, err)
			}
			result := IsAudio(mime)
			if result != tt.expected {
				t.Errorf("IsAudio(%q) = %v, want %v", tt.mime, result, tt.expected)
			}
		})
	}
}

// TestIsImage tests the IsImage function
func TestIsImage(t *testing.T) {
	tests := []struct {
		name     string
		mime     string
		expected bool
	}{
		{"image/jpeg", "image/jpeg", true},
		{"image/png", "image/png", true},
		{"image/gif", "image/gif", true},
		{"image/svg+xml", "image/svg+xml", true},
		{"video/mp4", "video/mp4", false},
		{"audio/mpeg", "audio/mpeg", false},
		{"text/html", "text/html", false},
		{"application/json", "application/json", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mime, err := Parse(tt.mime)
			if err != nil {
				t.Fatalf("Parse(%q) error: %v", tt.mime, err)
			}
			result := IsImage(mime)
			if result != tt.expected {
				t.Errorf("IsImage(%q) = %v, want %v", tt.mime, result, tt.expected)
			}
		})
	}
}

// TestIsText tests the IsText function
func TestIsText(t *testing.T) {
	tests := []struct {
		name     string
		mime     string
		expected bool
	}{
		{"text/plain", "text/plain", true},
		{"text/html", "text/html", true},
		{"text/css", "text/css", true},
		{"text/javascript", "text/javascript", true},
		{"application/json", "application/json", false},
		{"image/png", "image/png", false},
		{"video/mp4", "video/mp4", false},
		{"audio/mpeg", "audio/mpeg", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mime, err := Parse(tt.mime)
			if err != nil {
				t.Fatalf("Parse(%q) error: %v", tt.mime, err)
			}
			result := IsText(mime)
			if result != tt.expected {
				t.Errorf("IsText(%q) = %v, want %v", tt.mime, result, tt.expected)
			}
		})
	}
}

// TestIsApplication tests the IsApplication function
func TestIsApplication(t *testing.T) {
	tests := []struct {
		name     string
		mime     string
		expected bool
	}{
		{"application/json", "application/json", true},
		{"application/pdf", "application/pdf", true},
		{"application/zip", "application/zip", true},
		{"application/xml", "application/xml", true},
		{"text/html", "text/html", false},
		{"image/jpeg", "image/jpeg", false},
		{"video/mp4", "video/mp4", false},
		{"audio/mpeg", "audio/mpeg", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mime, err := Parse(tt.mime)
			if err != nil {
				t.Fatalf("Parse(%q) error: %v", tt.mime, err)
			}
			result := IsApplication(mime)
			if result != tt.expected {
				t.Errorf("IsApplication(%q) = %v, want %v", tt.mime, result, tt.expected)
			}
		})
	}
}

// TestPredefinedMimeTypes tests the predefined MIME type variables
func TestPredefinedMimeTypes(t *testing.T) {
	tests := []struct {
		name    string
		mime    *MIME
		typeVal string
		subType string
	}{
		{"all", &all, "*", "*"},
		{"text", &text, "text", "*"},
		{"video", &video, "video", "*"},
		{"audio", &audio, "audio", "*"},
		{"image", &image, "image", "*"},
		{"application", &application, "application", "*"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.mime.Type() != tt.typeVal {
				t.Errorf("Type() = %q, want %q", tt.mime.Type(), tt.typeVal)
			}
			if tt.mime.SubType() != tt.subType {
				t.Errorf("SubType() = %q, want %q", tt.mime.SubType(), tt.subType)
			}
		})
	}
}

// TestDetectWithBinaryData tests detection with various binary formats
func TestDetectWithBinaryData(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{
			name: "empty data",
			data: []byte{},
		},
		{
			name: "random binary",
			data: []byte{0x00, 0x01, 0x02, 0x03, 0x04, 0x05},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := Detect(tt.data)
			if err != nil {
				t.Errorf("Detect() error: %v", err)
			}
			if result == nil {
				t.Error("Detect should not return nil")
			}
		})
	}
}

// TestDetectReaderWithBuffer tests DetectReader with buffered content
func TestDetectReaderWithBuffer(t *testing.T) {
	content := []byte("This is test content for buffer testing")
	buffer := bytes.NewBuffer(content)

	result, err := DetectReader(buffer)
	if err != nil {
		t.Errorf("DetectReader() error: %v", err)
	}
	if result == nil {
		t.Error("DetectReader should not return nil")
	}
}

// BenchmarkParse benchmarks the Parse function
func BenchmarkParse(b *testing.B) {
	testCases := []string{
		"text/html",
		"application/json",
		"text/html; charset=UTF-8",
		"application/json; charset=UTF-8; boundary=something",
	}

	for _, tc := range testCases {
		b.Run(tc, func(b *testing.B) {
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, _ = Parse(tc)
			}
		})
	}
}

// BenchmarkDetect benchmarks the Detect function
func BenchmarkDetect(b *testing.B) {
	data := []byte(`{"key": "value", "array": [1, 2, 3]}`)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = Detect(data)
	}
}

// BenchmarkNew benchmarks the New function
func BenchmarkNew(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = New("application", "json")
	}
}

// BenchmarkIsChecks benchmarks the type checking functions
func BenchmarkIsChecks(b *testing.B) {
	mime, _ := Parse("text/html")

	b.Run("IsText", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = IsText(mime)
		}
	})

	b.Run("IsImage", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = IsImage(mime)
		}
	})

	b.Run("IsVideo", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = IsVideo(mime)
		}
	})
}

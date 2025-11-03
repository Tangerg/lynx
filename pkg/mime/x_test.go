package mime

import (
	"testing"
)

// TestRegisterXSubtype tests single x-prefix subtype registration
func TestRegisterXSubtype(t *testing.T) {
	tests := []struct {
		name            string
		xSubtype        string
		standardSubtype string
		testMimeString  string
		expectedSubtype string
	}{
		{
			name:            "custom format mapping",
			xSubtype:        "x-custom-format",
			standardSubtype: "custom-format",
			testMimeString:  "application/x-custom-format",
			expectedSubtype: "custom-format",
		},
		{
			name:            "override existing mapping",
			xSubtype:        "x-less",
			standardSubtype: "less",
			testMimeString:  "application/x-less",
			expectedSubtype: "less",
		},
		{
			name:            "type with plus suffix",
			xSubtype:        "x-custom+json",
			standardSubtype: "custom+json",
			testMimeString:  "application/x-custom+json",
			expectedSubtype: "custom+json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Register mapping
			RegisterXSubtype(tt.xSubtype, tt.standardSubtype)

			// Parse and normalize
			mime, err := Parse(tt.testMimeString)
			if err != nil {
				t.Fatalf("Failed to parse MIME: %v", err)
			}

			normalized := NormalizeXSubtype(mime)
			if normalized.SubType() != tt.expectedSubtype {
				t.Errorf("normalized subtype = %v, want %v",
					normalized.SubType(), tt.expectedSubtype)
			}
		})
	}
}

// TestRegisterXSubtypes tests batch registration
func TestRegisterXSubtypes(t *testing.T) {
	mappings := map[string]string{
		"x-batch1": "batch1",
		"x-batch2": "batch2",
		"x-batch3": "batch3",
	}

	RegisterXSubtypes(mappings)

	tests := []struct {
		mimeString      string
		expectedSubtype string
	}{
		{"application/x-batch1", "batch1"},
		{"text/x-batch2", "batch2"},
		{"image/x-batch3", "batch3"},
	}

	for _, tt := range tests {
		t.Run(tt.mimeString, func(t *testing.T) {
			mime, err := Parse(tt.mimeString)
			if err != nil {
				t.Fatalf("Failed to parse MIME: %v", err)
			}

			normalized := NormalizeXSubtype(mime)
			if normalized.SubType() != tt.expectedSubtype {
				t.Errorf("normalized subtype = %v, want %v",
					normalized.SubType(), tt.expectedSubtype)
			}
		})
	}
}

// TestNormalizeXSubtype_PredefinedMappings tests predefined mappings
func TestNormalizeXSubtype_PredefinedMappings(t *testing.T) {
	tests := []struct {
		name            string
		input           string
		expectedSubtype string
		expectedType    string
	}{
		{
			name:            "JavaScript type",
			input:           "application/x-javascript",
			expectedSubtype: "javascript",
			expectedType:    "application",
		},
		{
			name:            "JSON type",
			input:           "application/x-json",
			expectedSubtype: "json",
			expectedType:    "application",
		},
		{
			name:            "PNG image",
			input:           "image/x-png",
			expectedSubtype: "png",
			expectedType:    "image",
		},
		{
			name:            "YAML config",
			input:           "application/x-yaml",
			expectedSubtype: "yaml",
			expectedType:    "application",
		},
		{
			name:            "Markdown text",
			input:           "text/x-markdown",
			expectedSubtype: "markdown",
			expectedType:    "text",
		},
		{
			name:            "Gzip compression",
			input:           "application/x-gzip",
			expectedSubtype: "gzip",
			expectedType:    "application",
		},
		{
			name:            "Flash animation",
			input:           "application/x-shockwave-flash",
			expectedSubtype: "vnd.adobe.flash-movie",
			expectedType:    "application",
		},
		{
			name:            "RAR compression",
			input:           "application/x-rar-compressed",
			expectedSubtype: "vnd.rar",
			expectedType:    "application",
		},
		{
			name:            "ECMAScript type",
			input:           "application/x-ecmascript",
			expectedSubtype: "ecmascript",
			expectedType:    "application",
		},
		{
			name:            "LaTeX document",
			input:           "application/x-latex",
			expectedSubtype: "latex",
			expectedType:    "application",
		},
		{
			name:            "Shell script",
			input:           "application/x-sh",
			expectedSubtype: "sh",
			expectedType:    "application",
		},
		{
			name:            "Perl script",
			input:           "application/x-perl",
			expectedSubtype: "perl",
			expectedType:    "application",
		},
		{
			name:            "PHP script",
			input:           "application/x-httpd-php",
			expectedSubtype: "php",
			expectedType:    "application",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mime, err := Parse(tt.input)
			if err != nil {
				t.Fatalf("Failed to parse MIME: %v", err)
			}

			normalized := NormalizeXSubtype(mime)

			if normalized.Type() != tt.expectedType {
				t.Errorf("type = %v, want %v", normalized.Type(), tt.expectedType)
			}

			if normalized.SubType() != tt.expectedSubtype {
				t.Errorf("subtype = %v, want %v",
					normalized.SubType(), tt.expectedSubtype)
			}
		})
	}
}

// TestNormalizeXSubtype_FallbackBehavior tests fallback behavior (remove x- prefix when no mapping exists)
func TestNormalizeXSubtype_FallbackBehavior(t *testing.T) {
	tests := []struct {
		name            string
		input           string
		expectedSubtype string
	}{
		{
			name:            "unknown x- type",
			input:           "application/x-unknown",
			expectedSubtype: "unknown",
		},
		{
			name:            "custom x- type",
			input:           "text/x-custom-type",
			expectedSubtype: "custom-type",
		},
		{
			name:            "x- type with hyphens",
			input:           "application/x-my-special-format",
			expectedSubtype: "my-special-format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mime, err := Parse(tt.input)
			if err != nil {
				t.Fatalf("Failed to parse MIME: %v", err)
			}

			normalized := NormalizeXSubtype(mime)

			if normalized.SubType() != tt.expectedSubtype {
				t.Errorf("subtype = %v, want %v",
					normalized.SubType(), tt.expectedSubtype)
			}
		})
	}
}

// TestNormalizeXSubtype_WithParameters tests normalization with parameters
func TestNormalizeXSubtype_WithParameters(t *testing.T) {
	tests := []struct {
		name            string
		input           string
		expectedSubtype string
		expectedCharset string
		paramKey        string
		paramValue      string
	}{
		{
			name:            "JavaScript with charset",
			input:           "application/x-javascript; charset=UTF-8",
			expectedSubtype: "javascript",
			expectedCharset: "UTF-8",
		},
		{
			name:            "with custom parameter",
			input:           "text/x-markdown; version=1.0",
			expectedSubtype: "markdown",
			paramKey:        "version",
			paramValue:      "1.0",
		},
		{
			name:            "with multiple parameters",
			input:           "application/x-json; charset=UTF-8; version=2.0",
			expectedSubtype: "json",
			expectedCharset: "UTF-8",
			paramKey:        "version",
			paramValue:      "2.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mime, err := Parse(tt.input)
			if err != nil {
				t.Fatalf("Failed to parse MIME: %v", err)
			}

			normalized := NormalizeXSubtype(mime)

			if normalized.SubType() != tt.expectedSubtype {
				t.Errorf("subtype = %v, want %v",
					normalized.SubType(), tt.expectedSubtype)
			}

			if tt.expectedCharset != "" && normalized.Charset() != tt.expectedCharset {
				t.Errorf("charset = %v, want %v",
					normalized.Charset(), tt.expectedCharset)
			}

			if tt.paramKey != "" {
				value, ok := normalized.Param(tt.paramKey)
				if !ok {
					t.Errorf("parameter %v does not exist", tt.paramKey)
				}
				if value != tt.paramValue {
					t.Errorf("parameter value = %v, want %v", value, tt.paramValue)
				}
			}
		})
	}
}

// TestNormalizeXSubtype_NoXPrefix tests cases without x- prefix
func TestNormalizeXSubtype_NoXPrefix(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "standard JSON",
			input: "application/json",
		},
		{
			name:  "standard HTML",
			input: "text/html",
		},
		{
			name:  "standard PNG",
			input: "image/png",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mime, err := Parse(tt.input)
			if err != nil {
				t.Fatalf("Failed to parse MIME: %v", err)
			}

			normalized := NormalizeXSubtype(mime)

			// Should return a clone without changing original values
			if !mime.Equals(normalized) {
				t.Errorf("non-x-prefix MIME should remain unchanged")
			}

			// Ensure it's a different instance
			if mime == normalized {
				t.Errorf("should return a new instance, not the original reference")
			}
		})
	}
}

// TestNormalizeXSubtype_Immutability tests immutability
func TestNormalizeXSubtype_Immutability(t *testing.T) {
	original, _ := Parse("application/x-javascript; charset=UTF-8")
	originalString := original.String()

	normalized := NormalizeXSubtype(original)

	// Verify original object is not modified
	if original.String() != originalString {
		t.Errorf("original MIME object was modified")
	}

	if original.SubType() != "x-javascript" {
		t.Errorf("original subtype was modified")
	}

	// Verify a new object is returned
	if original == normalized {
		t.Errorf("should return a new instance")
	}
}

// TestNormalizeXSubtype_EdgeCases tests edge cases
func TestNormalizeXSubtype_EdgeCases(t *testing.T) {
	tests := []struct {
		name            string
		input           string
		expectedSubtype string
		shouldHaveError bool
	}{
		{
			name:            "only x- prefix",
			input:           "application/x-",
			expectedSubtype: "*",
		},
		{
			name:            "multiple x- prefixes",
			input:           "application/x-x-type",
			expectedSubtype: "x-type",
		},
		{
			name:            "x in the middle",
			input:           "application/test-x-type",
			expectedSubtype: "test-x-type", // Should not be normalized
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mime, err := Parse(tt.input)
			if err != nil && !tt.shouldHaveError {
				t.Fatalf("Parse failed: %v", err)
			}

			if err == nil {
				normalized := NormalizeXSubtype(mime)
				if normalized.SubType() != tt.expectedSubtype {
					t.Errorf("subtype = %v, want %v",
						normalized.SubType(), tt.expectedSubtype)
				}
			}
		})
	}
}

// TestNormalizeXSubtype_ComplexSubtypes tests complex subtypes
func TestNormalizeXSubtype_ComplexSubtypes(t *testing.T) {
	tests := []struct {
		name            string
		input           string
		expectedSubtype string
	}{
		{
			name:            "with vendor prefix",
			input:           "application/x-vnd.custom",
			expectedSubtype: "vnd.custom",
		},
		{
			name:            "with plus suffix",
			input:           "application/x-custom+xml",
			expectedSubtype: "custom+xml",
		},
		{
			name:            "multiple dot separators",
			input:           "application/x-vnd.company.product",
			expectedSubtype: "vnd.company.product",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mime, err := Parse(tt.input)
			if err != nil {
				t.Fatalf("Parse failed: %v", err)
			}

			normalized := NormalizeXSubtype(mime)
			if normalized.SubType() != tt.expectedSubtype {
				t.Errorf("subtype = %v, want %v",
					normalized.SubType(), tt.expectedSubtype)
			}
		})
	}
}

// TestNormalizeXSubtype_AllPredefinedMappings tests all predefined mappings comprehensively
func TestNormalizeXSubtype_AllPredefinedMappings(t *testing.T) {
	// Audio types
	audioTests := []struct {
		input    string
		expected string
	}{
		{"audio/x-wav", "wav"},
		{"audio/x-midi", "midi"},
		{"audio/x-aiff", "aiff"},
		{"audio/x-realaudio", "vnd.rn-realaudio"},
		{"audio/x-pn-realaudio", "vnd.rn-realaudio"},
		{"audio/x-ogg", "ogg"},
		{"audio/x-flac", "flac"},
		{"audio/x-ac3", "ac3"},
		{"audio/x-m4a", "mp4"},
		{"audio/x-m4r", "mp4"},
		{"audio/x-aac", "aac"},
		{"audio/x-mp3", "mpeg"},
		{"audio/x-mpeg", "mpeg"},
	}

	for _, tt := range audioTests {
		t.Run(tt.input, func(t *testing.T) {
			mime, _ := Parse(tt.input)
			normalized := NormalizeXSubtype(mime)
			if normalized.SubType() != tt.expected {
				t.Errorf("subtype = %v, want %v", normalized.SubType(), tt.expected)
			}
		})
	}

	// Video types
	videoTests := []struct {
		input    string
		expected string
	}{
		{"video/x-ms-asf", "vnd.ms-asf"},
		{"video/x-m4v", "mp4"},
		{"video/x-quicktime", "quicktime"},
	}

	for _, tt := range videoTests {
		t.Run(tt.input, func(t *testing.T) {
			mime, _ := Parse(tt.input)
			normalized := NormalizeXSubtype(mime)
			if normalized.SubType() != tt.expected {
				t.Errorf("subtype = %v, want %v", normalized.SubType(), tt.expected)
			}
		})
	}

	// Image types
	imageTests := []struct {
		input    string
		expected string
	}{
		{"image/x-png", "png"},
		{"image/x-icon", "vnd.microsoft.icon"},
		{"image/x-ms-bmp", "bmp"},
		{"image/x-tiff", "tiff"},
		{"image/x-photoshop", "vnd.adobe.photoshop"},
		{"image/x-webp", "webp"},
		{"image/x-windows-bmp", "bmp"},
	}

	for _, tt := range imageTests {
		t.Run(tt.input, func(t *testing.T) {
			mime, _ := Parse(tt.input)
			normalized := NormalizeXSubtype(mime)
			if normalized.SubType() != tt.expected {
				t.Errorf("subtype = %v, want %v", normalized.SubType(), tt.expected)
			}
		})
	}

	// Text types
	textTests := []struct {
		input    string
		expected string
	}{
		{"text/x-markdown", "markdown"},
		{"text/x-vcard", "vcard"},
		{"text/x-vcalendar", "calendar"},
		{"text/x-csv", "csv"},
		{"text/x-sgml", "sgml"},
		{"text/x-component", "html-component"},
	}

	for _, tt := range textTests {
		t.Run(tt.input, func(t *testing.T) {
			mime, _ := Parse(tt.input)
			normalized := NormalizeXSubtype(mime)
			if normalized.SubType() != tt.expected {
				t.Errorf("subtype = %v, want %v", normalized.SubType(), tt.expected)
			}
		})
	}

	// Application types
	appTests := []struct {
		input    string
		expected string
	}{
		{"application/x-compressed", "compressed"},
		{"application/x-zip-compressed", "zip"},
		{"application/x-stuffit", "stuffit"},
		{"application/x-director", "vnd.adobe.director"},
		{"application/x-msdos-program", "vnd.microsoft.portable-executable"},
		{"application/x-wais-source", "wais-source"},
		{"application/x-csh", "csh"},
		{"application/x-python", "python"},
		{"application/x-ruby", "ruby"},
		{"application/x-bytecode.python", "python-bytecode"},
		{"application/x-ole-storage", "vnd.ms-ole-storage"},
		{"application/x-tcl", "tcl"},
		{"application/x-pkcs7-signature", "pkcs7-signature"},
		{"application/x-pkcs7-mime", "pkcs7-mime"},
		{"application/x-dvi", "dvi"},
	}

	for _, tt := range appTests {
		t.Run(tt.input, func(t *testing.T) {
			mime, _ := Parse(tt.input)
			normalized := NormalizeXSubtype(mime)
			if normalized.SubType() != tt.expected {
				t.Errorf("subtype = %v, want %v", normalized.SubType(), tt.expected)
			}
		})
	}
}

// TestRegisterXSubtype_ThreadSafety tests thread safety of registration
func TestRegisterXSubtype_ThreadSafety(t *testing.T) {
	const goroutines = 100
	done := make(chan bool, goroutines)

	for i := 0; i < goroutines; i++ {
		go func(id int) {
			RegisterXSubtype("x-concurrent-"+string(rune(id)), "concurrent-"+string(rune(id)))
			done <- true
		}(i)
	}

	for i := 0; i < goroutines; i++ {
		<-done
	}
}

// BenchmarkNormalizeXSubtype benchmarks normalization performance
func BenchmarkNormalizeXSubtype(b *testing.B) {
	mime, _ := Parse("application/x-javascript; charset=UTF-8")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = NormalizeXSubtype(mime)
	}
}

// BenchmarkNormalizeXSubtype_WithMapping benchmarks with mapping
func BenchmarkNormalizeXSubtype_WithMapping(b *testing.B) {
	RegisterXSubtype("x-benchmark", "benchmark")
	mime, _ := Parse("application/x-benchmark")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = NormalizeXSubtype(mime)
	}
}

// BenchmarkNormalizeXSubtype_NoXPrefix benchmarks without x- prefix
func BenchmarkNormalizeXSubtype_NoXPrefix(b *testing.B) {
	mime, _ := Parse("application/json")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = NormalizeXSubtype(mime)
	}
}

// BenchmarkRegisterXSubtype benchmarks registration
func BenchmarkRegisterXSubtype(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		RegisterXSubtype("x-test", "test")
	}
}

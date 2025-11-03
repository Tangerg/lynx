package mime

import (
	"strings"
	"testing"

	"github.com/Tangerg/lynx/pkg/maps"
)

// TestInit tests the initialization of tokenBitSet
func TestInit(t *testing.T) {
	t.Run("tokenBitSet initialized", func(t *testing.T) {
		if tokenBitSet == nil {
			t.Fatal("tokenBitSet should be initialized")
		}
	})

	t.Run("valid token characters", func(t *testing.T) {
		validChars := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789!#$%&'*+-.^_`|~"
		for _, char := range validChars {
			if !tokenBitSet.Test(uint(char)) {
				t.Errorf("Character %c (%d) should be valid in tokens", char, char)
			}
		}
	})

	t.Run("invalid separator characters", func(t *testing.T) {
		separators := "()<>@,;:\\\"/[]?={} \t"
		for _, char := range separators {
			if tokenBitSet.Test(uint(char)) {
				t.Errorf("Separator character %c (%d) should not be valid in tokens", char, char)
			}
		}
	})

	t.Run("invalid control characters", func(t *testing.T) {
		// Test control characters 0-31
		for i := uint(0); i <= 31; i++ {
			if tokenBitSet.Test(i) {
				t.Errorf("Control character %d should not be valid in tokens", i)
			}
		}
		// Test DEL character (127)
		if tokenBitSet.Test(127) {
			t.Error("DEL character (127) should not be valid in tokens")
		}
	})
}

// TestNewBuilder tests the NewBuilder constructor
func TestNewBuilder(t *testing.T) {
	builder := NewBuilder()

	if builder == nil {
		t.Fatal("NewBuilder should not return nil")
	}

	if builder.mime == nil {
		t.Fatal("Builder's MIME should not be nil")
	}

	t.Run("default type is wildcard", func(t *testing.T) {
		if builder.mime._type != wildcardType {
			t.Errorf("Default type = %v, want %v", builder.mime._type, wildcardType)
		}
	})

	t.Run("default subtype is wildcard", func(t *testing.T) {
		if builder.mime.subType != wildcardType {
			t.Errorf("Default subtype = %v, want %v", builder.mime.subType, wildcardType)
		}
	})

	t.Run("default charset is empty", func(t *testing.T) {
		if builder.mime.charset != "" {
			t.Errorf("Default charset = %v, want empty string", builder.mime.charset)
		}
	})

	t.Run("params map is initialized", func(t *testing.T) {
		if builder.mime.params == nil {
			t.Error("Params map should be initialized")
		}
		if len(builder.mime.params) != 0 {
			t.Errorf("Params map should be empty, got %d entries", len(builder.mime.params))
		}
	})
}

// TestBuilder_checkToken tests the token validation
func TestBuilder_checkToken(t *testing.T) {
	builder := NewBuilder()

	tests := []struct {
		name    string
		token   string
		wantErr bool
	}{
		{
			name:    "valid simple token",
			token:   "application",
			wantErr: false,
		},
		{
			name:    "valid token with hyphen",
			token:   "text-plain",
			wantErr: false,
		},
		{
			name:    "valid token with numbers",
			token:   "version1",
			wantErr: false,
		},
		{
			name:    "valid token with special chars",
			token:   "custom.type+subtype",
			wantErr: false,
		},
		{
			name:    "invalid token with slash",
			token:   "text/html",
			wantErr: true,
		},
		{
			name:    "invalid token with space",
			token:   "text html",
			wantErr: true,
		},
		{
			name:    "invalid token with at sign",
			token:   "user@domain",
			wantErr: true,
		},
		{
			name:    "invalid token with comma",
			token:   "text,html",
			wantErr: true,
		},
		{
			name:    "invalid token with semicolon",
			token:   "text;charset",
			wantErr: true,
		},
		{
			name:    "invalid token with equals",
			token:   "key=value",
			wantErr: true,
		},
		{
			name:    "invalid token with parentheses",
			token:   "text(comment)",
			wantErr: true,
		},
		{
			name:    "invalid token with brackets",
			token:   "text[html]",
			wantErr: true,
		},
		{
			name:    "invalid token with quotes",
			token:   "text\"html\"",
			wantErr: true,
		},
		{
			name:    "invalid token with backslash",
			token:   "text\\html",
			wantErr: true,
		},
		{
			name:    "empty token",
			token:   "",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := builder.checkToken(tt.token)
			if (err != nil) != tt.wantErr {
				t.Errorf("checkToken() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && !strings.Contains(err.Error(), "invalid character") {
				t.Errorf("Error message should contain 'invalid character', got: %v", err)
			}
		})
	}
}

// TestBuilder_checkParam tests parameter validation
func TestBuilder_checkParam(t *testing.T) {
	builder := NewBuilder()

	tests := []struct {
		name       string
		paramKey   string
		paramValue string
		wantErr    bool
	}{
		{
			name:       "valid simple param",
			paramKey:   "charset",
			paramValue: "UTF-8",
			wantErr:    false,
		},
		{
			name:       "valid quoted value",
			paramKey:   "title",
			paramValue: "\"My Document\"",
			wantErr:    false,
		},
		{
			name:       "valid quoted value with spaces",
			paramKey:   "name",
			paramValue: "\"file name.txt\"",
			wantErr:    false,
		},
		{
			name:       "invalid key with space",
			paramKey:   "content type",
			paramValue: "value",
			wantErr:    true,
		},
		{
			name:       "invalid key with equals",
			paramKey:   "key=name",
			paramValue: "value",
			wantErr:    true,
		},
		{
			name:       "invalid unquoted value with space",
			paramKey:   "key",
			paramValue: "value with space",
			wantErr:    true,
		},
		{
			name:       "valid key with hyphen",
			paramKey:   "content-type",
			paramValue: "value",
			wantErr:    false,
		},
		{
			name:       "valid key with dot",
			paramKey:   "x.custom",
			paramValue: "value",
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := builder.checkParam(tt.paramKey, tt.paramValue)
			if (err != nil) != tt.wantErr {
				t.Errorf("checkParam() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestBuilder_WithType tests type setting
func TestBuilder_WithType(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple lowercase",
			input:    "text",
			expected: "text",
		},
		{
			name:     "uppercase normalized",
			input:    "TEXT",
			expected: "text",
		},
		{
			name:     "mixed case normalized",
			input:    "TeXt",
			expected: "text",
		},
		{
			name:     "quoted string",
			input:    "\"application\"",
			expected: "application",
		},
		{
			name:     "with whitespace",
			input:    "  image  ",
			expected: "  image  ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := NewBuilder().WithType(tt.input)
			if builder.mime._type != tt.expected {
				t.Errorf("WithType() type = %v, want %v", builder.mime._type, tt.expected)
			}
		})
	}
}

// TestBuilder_WithSubType tests subtype setting
func TestBuilder_WithSubType(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple lowercase",
			input:    "html",
			expected: "html",
		},
		{
			name:     "uppercase normalized",
			input:    "HTML",
			expected: "html",
		},
		{
			name:     "mixed case normalized",
			input:    "HtMl",
			expected: "html",
		},
		{
			name:     "quoted string",
			input:    "\"json\"",
			expected: "json",
		},
		{
			name:     "with plus sign",
			input:    "svg+xml",
			expected: "svg+xml",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := NewBuilder().WithSubType(tt.input)
			if builder.mime.subType != tt.expected {
				t.Errorf("WithSubType() subtype = %v, want %v", builder.mime.subType, tt.expected)
			}
		})
	}
}

// TestBuilder_WithCharset tests charset setting
func TestBuilder_WithCharset(t *testing.T) {
	tests := []struct {
		name            string
		input           string
		expectedCharset string
		shouldHaveParam bool
	}{
		{
			name:            "simple charset",
			input:           "utf-8",
			expectedCharset: "UTF-8",
			shouldHaveParam: true,
		},
		{
			name:            "lowercase normalized to uppercase",
			input:           "iso-8859-1",
			expectedCharset: "ISO-8859-1",
			shouldHaveParam: true,
		},
		{
			name:            "quoted charset",
			input:           "\"UTF-8\"",
			expectedCharset: "UTF-8",
			shouldHaveParam: true,
		},
		{
			name:            "empty charset",
			input:           "",
			expectedCharset: "",
			shouldHaveParam: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := NewBuilder().WithCharset(tt.input)

			if builder.mime.charset != tt.expectedCharset {
				t.Errorf("WithCharset() charset = %v, want %v",
					builder.mime.charset, tt.expectedCharset)
			}

			paramValue, hasParam := builder.mime.params.Get(paramCharset)
			if hasParam != tt.shouldHaveParam {
				t.Errorf("WithCharset() param exists = %v, want %v",
					hasParam, tt.shouldHaveParam)
			}

			if tt.shouldHaveParam && paramValue != tt.expectedCharset {
				t.Errorf("WithCharset() param value = %v, want %v",
					paramValue, tt.expectedCharset)
			}
		})
	}
}

// TestBuilder_WithParam tests parameter setting
func TestBuilder_WithParam(t *testing.T) {
	tests := []struct {
		name          string
		key           string
		value         string
		expectedKey   string
		expectedValue string
		shouldSet     bool
	}{
		{
			name:          "simple param",
			key:           "version",
			value:         "1.0",
			expectedKey:   "version",
			expectedValue: "1.0",
			shouldSet:     true,
		},
		{
			name:          "uppercase key normalized",
			key:           "VERSION",
			value:         "1.0",
			expectedKey:   "version",
			expectedValue: "1.0",
			shouldSet:     true,
		},
		{
			name:          "quoted key",
			key:           "\"quality\"",
			value:         "high",
			expectedKey:   "quality",
			expectedValue: "high",
			shouldSet:     true,
		},
		{
			name:          "empty key",
			key:           "",
			value:         "value",
			expectedKey:   "",
			expectedValue: "",
			shouldSet:     false,
		},
		{
			name:          "charset delegates to WithCharset",
			key:           "charset",
			value:         "utf-8",
			expectedKey:   "charset",
			expectedValue: "UTF-8",
			shouldSet:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := NewBuilder().WithParam(tt.key, tt.value)

			if tt.shouldSet {
				value, exists := builder.mime.params.Get(tt.expectedKey)
				if !exists {
					t.Errorf("WithParam() param %v should exist", tt.expectedKey)
				} else if value != tt.expectedValue {
					t.Errorf("WithParam() param value = %v, want %v",
						value, tt.expectedValue)
				}
			} else {
				if len(builder.mime.params) != 0 {
					t.Errorf("WithParam() should not set param for empty key")
				}
			}
		})
	}
}

// TestBuilder_WithParams tests batch parameter setting
func TestBuilder_WithParams(t *testing.T) {
	tests := []struct {
		name     string
		params   map[string]string
		expected map[string]string
	}{
		{
			name: "multiple params",
			params: map[string]string{
				"version": "1.0",
				"quality": "high",
				"level":   "5",
			},
			expected: map[string]string{
				"version": "1.0",
				"quality": "high",
				"level":   "5",
			},
		},
		{
			name: "with charset",
			params: map[string]string{
				"charset": "utf-8",
				"version": "2.0",
			},
			expected: map[string]string{
				"charset": "UTF-8",
				"version": "2.0",
			},
		},
		{
			name:     "empty params",
			params:   map[string]string{},
			expected: map[string]string{},
		},
		{
			name:     "nil params",
			params:   nil,
			expected: map[string]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := NewBuilder().WithParams(tt.params)

			if len(builder.mime.params) != len(tt.expected) {
				t.Errorf("WithParams() params count = %v, want %v",
					len(builder.mime.params), len(tt.expected))
			}

			for key, expectedValue := range tt.expected {
				value, exists := builder.mime.params.Get(key)
				if !exists {
					t.Errorf("WithParams() param %v should exist", key)
				} else if value != expectedValue {
					t.Errorf("WithParams() param %v = %v, want %v",
						key, value, expectedValue)
				}
			}
		})
	}
}

// TestBuilder_FromMime tests initialization from existing MIME
func TestBuilder_FromMime(t *testing.T) {
	t.Run("nil mime", func(t *testing.T) {
		builder := NewBuilder().FromMime(nil)
		if builder.mime._type != wildcardType {
			t.Error("FromMime(nil) should not change default type")
		}
	})

	t.Run("copy all fields", func(t *testing.T) {
		params := maps.NewHashMap[string, string]()
		params.Put("charset", "UTF-8")
		params.Put("version", "1.0")

		sourceMime := &MIME{
			_type:        "text",
			subType:      "html",
			charset:      "UTF-8",
			params:       params,
			cachedString: "text/html;charset=UTF-8;version=1.0",
		}

		builder := NewBuilder().FromMime(sourceMime)

		if builder.mime._type != sourceMime._type {
			t.Errorf("Type not copied: got %v, want %v",
				builder.mime._type, sourceMime._type)
		}
		if builder.mime.subType != sourceMime.subType {
			t.Errorf("SubType not copied: got %v, want %v",
				builder.mime.subType, sourceMime.subType)
		}
		if builder.mime.charset != sourceMime.charset {
			t.Errorf("Charset not copied: got %v, want %v",
				builder.mime.charset, sourceMime.charset)
		}
		if builder.mime.cachedString != sourceMime.cachedString {
			t.Errorf("StringCache not copied: got %v, want %v",
				builder.mime.cachedString, sourceMime.cachedString)
		}
		if len(builder.mime.params) != len(sourceMime.params) {
			t.Errorf("Params not copied: got %v entries, want %v",
				len(builder.mime.params), len(sourceMime.params))
		}
	})

	t.Run("deep copy params", func(t *testing.T) {
		params := maps.NewHashMap[string, string]()
		params.Put("key", "value")

		sourceMime := &MIME{
			_type:   "text",
			subType: "plain",
			params:  params,
		}

		builder := NewBuilder().FromMime(sourceMime)

		// Modify builder's params
		builder.mime.params.Put("key", "modified")

		// Original should be unchanged
		originalValue, _ := sourceMime.params.Get("key")
		if originalValue != "value" {
			t.Error("Source MIME params were modified (not a deep copy)")
		}
	})
}

// TestBuilder_Build tests the build and validation process
func TestBuilder_Build(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(*Builder) *Builder
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid simple MIME",
			setup: func(b *Builder) *Builder {
				return b.WithType("text").WithSubType("html")
			},
			wantErr: false,
		},
		{
			name: "valid with charset",
			setup: func(b *Builder) *Builder {
				return b.WithType("text").WithSubType("html").WithCharset("UTF-8")
			},
			wantErr: false,
		},
		{
			name: "valid with params",
			setup: func(b *Builder) *Builder {
				return b.WithType("application").WithSubType("json").
					WithParam("version", "1.0")
			},
			wantErr: false,
		},
		{
			name: "invalid type with slash",
			setup: func(b *Builder) *Builder {
				return b.WithType("text/html").WithSubType("plain")
			},
			wantErr: true,
			errMsg:  "invalid character",
		},
		{
			name: "invalid subtype with space",
			setup: func(b *Builder) *Builder {
				return b.WithType("text").WithSubType("ht ml")
			},
			wantErr: true,
			errMsg:  "invalid character",
		},
		{
			name: "invalid charset with comma",
			setup: func(b *Builder) *Builder {
				return b.WithType("text").WithSubType("html").WithCharset("UTF,8")
			},
			wantErr: true,
			errMsg:  "invalid character",
		},
		{
			name: "invalid param key",
			setup: func(b *Builder) *Builder {
				return b.WithType("text").WithSubType("html").
					WithParam("key@name", "value")
			},
			wantErr: true,
			errMsg:  "invalid character",
		},
		{
			name: "invalid param value",
			setup: func(b *Builder) *Builder {
				return b.WithType("text").WithSubType("html").
					WithParam("key", "val ue")
			},
			wantErr: true,
			errMsg:  "invalid character",
		},
		{
			name: "default wildcard type",
			setup: func(b *Builder) *Builder {
				return b.WithSubType("html")
			},
			wantErr: false,
		},
		{
			name: "default wildcard subtype",
			setup: func(b *Builder) *Builder {
				return b.WithType("text")
			},
			wantErr: false,
		},
		{
			name: "both wildcards",
			setup: func(b *Builder) *Builder {
				return b
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := NewBuilder()
			builder = tt.setup(builder)

			mime, err := builder.Build()

			if (err != nil) != tt.wantErr {
				t.Errorf("Build() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr {
				if err != nil && tt.errMsg != "" {
					if !strings.Contains(err.Error(), tt.errMsg) {
						t.Errorf("Build() error = %v, should contain %v",
							err.Error(), tt.errMsg)
					}
				}
				if mime != nil {
					t.Error("Build() should return nil MIME on error")
				}
			} else {
				if mime == nil {
					t.Error("Build() should return non-nil MIME on success")
				}
			}
		})
	}
}

// TestBuilder_MustBuild tests the MustBuild panic behavior
func TestBuilder_MustBuild(t *testing.T) {
	t.Run("valid build does not panic", func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("MustBuild() panicked unexpectedly: %v", r)
			}
		}()

		mime := NewBuilder().
			WithType("text").
			WithSubType("html").
			MustBuild()

		if mime == nil {
			t.Error("MustBuild() returned nil")
		}
	})

	t.Run("invalid build panics", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Error("MustBuild() should panic on invalid build")
			}
		}()

		_ = NewBuilder().
			WithType("text/html").
			WithSubType("plain").
			MustBuild()
	})
}

// TestBuilder_FluentAPI tests the fluent interface
func TestBuilder_FluentAPI(t *testing.T) {
	mime, err := NewBuilder().
		WithType("application").
		WithSubType("json").
		WithCharset("UTF-8").
		WithParam("version", "1.0").
		WithParam("indent", "true").
		Build()

	if err != nil {
		t.Fatalf("Fluent API failed: %v", err)
	}

	if mime.Type() != "application" {
		t.Errorf("Type = %v, want application", mime.Type())
	}
	if mime.SubType() != "json" {
		t.Errorf("SubType = %v, want json", mime.SubType())
	}
	if mime.Charset() != "UTF-8" {
		t.Errorf("Charset = %v, want UTF-8", mime.Charset())
	}

	version, _ := mime.Param("version")
	if version != "1.0" {
		t.Errorf("Param version = %v, want 1.0", version)
	}

	indent, _ := mime.Param("indent")
	if indent != "true" {
		t.Errorf("Param indent = %v, want true", indent)
	}
}

// TestBuilder_ComplexMIME tests building complex MIME types
func TestBuilder_ComplexMIME(t *testing.T) {
	mime, err := NewBuilder().
		WithType("multipart").
		WithSubType("form-data").
		WithParam("boundary", "----WebKitFormBoundary7MA4YWxkTrZu0gW").
		Build()

	if err != nil {
		t.Fatalf("Complex MIME build failed: %v", err)
	}

	if mime.TypeAndSubType() != "multipart/form-data" {
		t.Errorf("TypeAndSubType = %v, want multipart/form-data",
			mime.TypeAndSubType())
	}

	boundary, ok := mime.Param("boundary")
	if !ok {
		t.Error("Boundary param should exist")
	}
	if !strings.HasPrefix(boundary, "----") {
		t.Errorf("Boundary should start with ----, got %v", boundary)
	}
}

// BenchmarkBuilder_Build benchmarks the build process
func BenchmarkBuilder_Build(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = NewBuilder().
			WithType("application").
			WithSubType("json").
			WithCharset("UTF-8").
			Build()
	}
}

// BenchmarkBuilder_WithParams benchmarks parameter setting
func BenchmarkBuilder_WithParams(b *testing.B) {
	params := map[string]string{
		"charset": "UTF-8",
		"version": "1.0",
		"quality": "high",
		"level":   "5",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = NewBuilder().
			WithType("text").
			WithSubType("html").
			WithParams(params)
	}
}

// BenchmarkBuilder_FromMime benchmarks copying from existing MIME
func BenchmarkBuilder_FromMime(b *testing.B) {
	params := maps.NewHashMap[string, string]()
	params.Put("charset", "UTF-8")
	params.Put("version", "1.0")

	sourceMime := &MIME{
		_type:   "text",
		subType: "html",
		charset: "UTF-8",
		params:  params,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = NewBuilder().FromMime(sourceMime)
	}
}

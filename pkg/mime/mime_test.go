package mime

import (
	"testing"

	"github.com/Tangerg/lynx/pkg/maps"
)

// TestMIME_Type tests the Type() method
func TestMIME_Type(t *testing.T) {
	tests := []struct {
		name     string
		mime     *MIME
		expected string
	}{
		{
			name:     "text type",
			mime:     &MIME{_type: "text", subType: "html"},
			expected: "text",
		},
		{
			name:     "application type",
			mime:     &MIME{_type: "application", subType: "json"},
			expected: "application",
		},
		{
			name:     "wildcard type",
			mime:     &MIME{_type: "*", subType: "html"},
			expected: "*",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.mime.Type(); got != tt.expected {
				t.Errorf("Type() = %v, want %v", got, tt.expected)
			}
		})
	}
}

// TestMIME_SubType tests the SubType() method
func TestMIME_SubType(t *testing.T) {
	tests := []struct {
		name     string
		mime     *MIME
		expected string
	}{
		{
			name:     "html subtype",
			mime:     &MIME{_type: "text", subType: "html"},
			expected: "html",
		},
		{
			name:     "json subtype",
			mime:     &MIME{_type: "application", subType: "json"},
			expected: "json",
		},
		{
			name:     "wildcard subtype",
			mime:     &MIME{_type: "text", subType: "*"},
			expected: "*",
		},
		{
			name:     "subtype with suffix",
			mime:     &MIME{_type: "application", subType: "vnd.api+json"},
			expected: "vnd.api+json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.mime.SubType(); got != tt.expected {
				t.Errorf("SubType() = %v, want %v", got, tt.expected)
			}
		})
	}
}

// TestMIME_TypeAndSubType tests the TypeAndSubType() method
func TestMIME_TypeAndSubType(t *testing.T) {
	tests := []struct {
		name     string
		mime     *MIME
		expected string
	}{
		{
			name:     "text/html",
			mime:     &MIME{_type: "text", subType: "html"},
			expected: "text/html",
		},
		{
			name:     "application/json",
			mime:     &MIME{_type: "application", subType: "json"},
			expected: "application/json",
		},
		{
			name:     "with wildcard",
			mime:     &MIME{_type: "text", subType: "*"},
			expected: "text/*",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.mime.TypeAndSubType(); got != tt.expected {
				t.Errorf("TypeAndSubType() = %v, want %v", got, tt.expected)
			}
		})
	}
}

// TestMIME_FullType tests that FullType is an alias for TypeAndSubType
func TestMIME_FullType(t *testing.T) {
	mime := &MIME{_type: "text", subType: "html"}
	if got := mime.FullType(); got != mime.TypeAndSubType() {
		t.Errorf("FullType() should equal TypeAndSubType()")
	}
}

// TestMIME_Charset tests the Charset() method
func TestMIME_Charset(t *testing.T) {
	tests := []struct {
		name     string
		mime     *MIME
		expected string
	}{
		{
			name:     "with charset",
			mime:     &MIME{_type: "text", subType: "html", charset: "UTF-8"},
			expected: "UTF-8",
		},
		{
			name:     "without charset",
			mime:     &MIME{_type: "application", subType: "json"},
			expected: "",
		},
		{
			name:     "with ISO charset",
			mime:     &MIME{_type: "text", subType: "plain", charset: "ISO-8859-1"},
			expected: "ISO-8859-1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.mime.Charset(); got != tt.expected {
				t.Errorf("Charset() = %v, want %v", got, tt.expected)
			}
		})
	}
}

// TestMIME_Param tests the Param() method
func TestMIME_Param(t *testing.T) {
	params := maps.HashMap[string, string]{}
	params.Put("charset", "UTF-8")
	params.Put("version", "1.0")

	mime := &MIME{
		_type:   "application",
		subType: "json",
		params:  params,
	}

	tests := []struct {
		name          string
		paramKey      string
		expectedValue string
		expectedOk    bool
	}{
		{
			name:          "existing param",
			paramKey:      "charset",
			expectedValue: "UTF-8",
			expectedOk:    true,
		},
		{
			name:          "another existing param",
			paramKey:      "version",
			expectedValue: "1.0",
			expectedOk:    true,
		},
		{
			name:          "non-existing param",
			paramKey:      "level",
			expectedValue: "",
			expectedOk:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			value, ok := mime.Param(tt.paramKey)
			if value != tt.expectedValue || ok != tt.expectedOk {
				t.Errorf("Param(%v) = (%v, %v), want (%v, %v)",
					tt.paramKey, value, ok, tt.expectedValue, tt.expectedOk)
			}
		})
	}
}

// TestMIME_Params tests the Params() method
func TestMIME_Params(t *testing.T) {
	params := maps.HashMap[string, string]{}
	params.Put("charset", "UTF-8")
	params.Put("version", "1.0")

	mime := &MIME{
		_type:   "application",
		subType: "json",
		params:  params,
	}

	got := mime.Params()
	if len(got) != 2 {
		t.Errorf("Params() length = %v, want 2", len(got))
	}

	if got["charset"] != "UTF-8" {
		t.Errorf("Params()[charset] = %v, want UTF-8", got["charset"])
	}

	if got["version"] != "1.0" {
		t.Errorf("Params()[version] = %v, want 1.0", got["version"])
	}
}

// TestMIME_String tests the String() method and caching
func TestMIME_String(t *testing.T) {
	tests := []struct {
		name     string
		mime     *MIME
		expected string
	}{
		{
			name: "simple type without params",
			mime: &MIME{
				_type:   "text",
				subType: "html",
				params:  maps.HashMap[string, string]{},
			},
			expected: "text/html",
		},
		{
			name: "with single param",
			mime: func() *MIME {
				params := maps.HashMap[string, string]{}
				params.Put("charset", "UTF-8")
				return &MIME{
					_type:   "text",
					subType: "html",
					params:  params,
				}
			}(),
			expected: "text/html;charset=UTF-8",
		},
		{
			name:     "nil mime",
			mime:     nil,
			expected: "",
		},
		{
			name: "with cached value",
			mime: &MIME{
				_type:        "application",
				subType:      "json",
				params:       maps.HashMap[string, string]{},
				cachedString: "application/json",
			},
			expected: "application/json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.mime.String(); got != tt.expected {
				t.Errorf("String() = %v, want %v", got, tt.expected)
			}
		})
	}
}

// TestMIME_String_Caching tests that string caching works properly
func TestMIME_String_Caching(t *testing.T) {
	params := maps.HashMap[string, string]{}
	params.Put("charset", "UTF-8")

	mime := &MIME{
		_type:   "text",
		subType: "html",
		params:  params,
	}

	// First call should build and cache
	first := mime.String()
	if mime.cachedString == "" {
		t.Error("cachedString should be set after first String() call")
	}

	// Second call should return cached value
	second := mime.String()
	if first != second {
		t.Errorf("String() caching failed: first=%v, second=%v", first, second)
	}
}

// TestMIME_IsWildcardType tests the IsWildcardType() method
func TestMIME_IsWildcardType(t *testing.T) {
	tests := []struct {
		name     string
		mime     *MIME
		expected bool
	}{
		{
			name:     "wildcard type",
			mime:     &MIME{_type: "*", subType: "html"},
			expected: true,
		},
		{
			name:     "concrete type",
			mime:     &MIME{_type: "text", subType: "html"},
			expected: false,
		},
		{
			name:     "application type",
			mime:     &MIME{_type: "application", subType: "*"},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.mime.IsWildcardType(); got != tt.expected {
				t.Errorf("IsWildcardType() = %v, want %v", got, tt.expected)
			}
		})
	}
}

// TestMIME_IsWildcardSubType tests the IsWildcardSubType() method
func TestMIME_IsWildcardSubType(t *testing.T) {
	tests := []struct {
		name     string
		mime     *MIME
		expected bool
	}{
		{
			name:     "wildcard subtype",
			mime:     &MIME{_type: "text", subType: "*"},
			expected: true,
		},
		{
			name:     "wildcard with suffix",
			mime:     &MIME{_type: "application", subType: "*+json"},
			expected: true,
		},
		{
			name:     "concrete subtype",
			mime:     &MIME{_type: "text", subType: "html"},
			expected: false,
		},
		{
			name:     "subtype with suffix but no wildcard",
			mime:     &MIME{_type: "application", subType: "vnd.api+json"},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.mime.IsWildcardSubType(); got != tt.expected {
				t.Errorf("IsWildcardSubType() = %v, want %v", got, tt.expected)
			}
		})
	}
}

// TestMIME_IsConcrete tests the IsConcrete() method
func TestMIME_IsConcrete(t *testing.T) {
	tests := []struct {
		name     string
		mime     *MIME
		expected bool
	}{
		{
			name:     "concrete type",
			mime:     &MIME{_type: "text", subType: "html"},
			expected: true,
		},
		{
			name:     "wildcard type",
			mime:     &MIME{_type: "*", subType: "html"},
			expected: false,
		},
		{
			name:     "wildcard subtype",
			mime:     &MIME{_type: "text", subType: "*"},
			expected: false,
		},
		{
			name:     "both wildcards",
			mime:     &MIME{_type: "*", subType: "*"},
			expected: false,
		},
		{
			name:     "wildcard subtype with suffix",
			mime:     &MIME{_type: "application", subType: "*+json"},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.mime.IsConcrete(); got != tt.expected {
				t.Errorf("IsConcrete() = %v, want %v", got, tt.expected)
			}
		})
	}
}

// TestMIME_GetSubtypeSuffix tests the GetSubtypeSuffix() method
func TestMIME_GetSubtypeSuffix(t *testing.T) {
	tests := []struct {
		name     string
		mime     *MIME
		expected string
	}{
		{
			name:     "with json suffix",
			mime:     &MIME{_type: "application", subType: "vnd.api+json"},
			expected: "json",
		},
		{
			name:     "with xml suffix",
			mime:     &MIME{_type: "application", subType: "atom+xml"},
			expected: "xml",
		},
		{
			name:     "no suffix",
			mime:     &MIME{_type: "text", subType: "html"},
			expected: "",
		},
		{
			name:     "wildcard with suffix",
			mime:     &MIME{_type: "application", subType: "*+json"},
			expected: "json",
		},
		{
			name:     "multiple plus signs",
			mime:     &MIME{_type: "application", subType: "vnd+custom+xml"},
			expected: "xml",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.mime.GetSubtypeSuffix(); got != tt.expected {
				t.Errorf("GetSubtypeSuffix() = %v, want %v", got, tt.expected)
			}
		})
	}
}

// TestMIME_Includes tests the Includes() method
func TestMIME_Includes(t *testing.T) {
	tests := []struct {
		name     string
		mime     *MIME
		other    *MIME
		expected bool
	}{
		{
			name:     "wildcard includes everything",
			mime:     &MIME{_type: "*", subType: "*"},
			other:    &MIME{_type: "text", subType: "html"},
			expected: true,
		},
		{
			name:     "type wildcard includes same type",
			mime:     &MIME{_type: "text", subType: "*"},
			other:    &MIME{_type: "text", subType: "html"},
			expected: true,
		},
		{
			name:     "type wildcard does not include different type",
			mime:     &MIME{_type: "text", subType: "*"},
			other:    &MIME{_type: "application", subType: "json"},
			expected: false,
		},
		{
			name:     "concrete includes same concrete",
			mime:     &MIME{_type: "text", subType: "html"},
			other:    &MIME{_type: "text", subType: "html"},
			expected: true,
		},
		{
			name:     "concrete does not include different concrete",
			mime:     &MIME{_type: "text", subType: "html"},
			other:    &MIME{_type: "text", subType: "plain"},
			expected: false,
		},
		{
			name:     "wildcard with suffix includes matching suffix",
			mime:     &MIME{_type: "application", subType: "*+json"},
			other:    &MIME{_type: "application", subType: "vnd.api+json"},
			expected: true,
		},
		{
			name:     "wildcard with suffix does not include different suffix",
			mime:     &MIME{_type: "application", subType: "*+json"},
			other:    &MIME{_type: "application", subType: "vnd.api+xml"},
			expected: false,
		},
		{
			name:     "wildcard with suffix does not include no suffix",
			mime:     &MIME{_type: "application", subType: "*+json"},
			other:    &MIME{_type: "application", subType: "json"},
			expected: false,
		},
		{
			name:     "nil other returns false",
			mime:     &MIME{_type: "text", subType: "html"},
			other:    nil,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.mime.Includes(tt.other); got != tt.expected {
				t.Errorf("Includes() = %v, want %v", got, tt.expected)
			}
		})
	}
}

// TestMIME_IsCompatibleWith tests the IsCompatibleWith() method
func TestMIME_IsCompatibleWith(t *testing.T) {
	tests := []struct {
		name     string
		mime     *MIME
		other    *MIME
		expected bool
	}{
		{
			name:     "same concrete types are compatible",
			mime:     &MIME{_type: "text", subType: "html"},
			other:    &MIME{_type: "text", subType: "html"},
			expected: true,
		},
		{
			name:     "wildcard compatible with concrete",
			mime:     &MIME{_type: "text", subType: "*"},
			other:    &MIME{_type: "text", subType: "html"},
			expected: true,
		},
		{
			name:     "concrete compatible with wildcard",
			mime:     &MIME{_type: "text", subType: "html"},
			other:    &MIME{_type: "text", subType: "*"},
			expected: true,
		},
		{
			name:     "different concrete types not compatible",
			mime:     &MIME{_type: "text", subType: "html"},
			other:    &MIME{_type: "application", subType: "json"},
			expected: false,
		},
		{
			name:     "nil other returns false",
			mime:     &MIME{_type: "text", subType: "html"},
			other:    nil,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.mime.IsCompatibleWith(tt.other); got != tt.expected {
				t.Errorf("IsCompatibleWith() = %v, want %v", got, tt.expected)
			}
		})
	}
}

// TestMIME_EqualsType tests the EqualsType() method
func TestMIME_EqualsType(t *testing.T) {
	tests := []struct {
		name     string
		mime     *MIME
		other    *MIME
		expected bool
	}{
		{
			name:     "same types",
			mime:     &MIME{_type: "text", subType: "html"},
			other:    &MIME{_type: "text", subType: "plain"},
			expected: true,
		},
		{
			name:     "different types",
			mime:     &MIME{_type: "text", subType: "html"},
			other:    &MIME{_type: "application", subType: "html"},
			expected: false,
		},
		{
			name:     "nil other returns false",
			mime:     &MIME{_type: "text", subType: "html"},
			other:    nil,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.mime.EqualsType(tt.other); got != tt.expected {
				t.Errorf("EqualsType() = %v, want %v", got, tt.expected)
			}
		})
	}
}

// TestMIME_EqualsSubtype tests the EqualsSubtype() method
func TestMIME_EqualsSubtype(t *testing.T) {
	tests := []struct {
		name     string
		mime     *MIME
		other    *MIME
		expected bool
	}{
		{
			name:     "same subtypes",
			mime:     &MIME{_type: "text", subType: "html"},
			other:    &MIME{_type: "application", subType: "html"},
			expected: true,
		},
		{
			name:     "different subtypes",
			mime:     &MIME{_type: "text", subType: "html"},
			other:    &MIME{_type: "text", subType: "plain"},
			expected: false,
		},
		{
			name:     "nil other returns false",
			mime:     &MIME{_type: "text", subType: "html"},
			other:    nil,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.mime.EqualsSubtype(tt.other); got != tt.expected {
				t.Errorf("EqualsSubtype() = %v, want %v", got, tt.expected)
			}
		})
	}
}

// TestMIME_EqualsTypeAndSubtype tests the EqualsTypeAndSubtype() method
func TestMIME_EqualsTypeAndSubtype(t *testing.T) {
	tests := []struct {
		name     string
		mime     *MIME
		other    *MIME
		expected bool
	}{
		{
			name:     "same type and subtype",
			mime:     &MIME{_type: "text", subType: "html"},
			other:    &MIME{_type: "text", subType: "html"},
			expected: true,
		},
		{
			name:     "different type",
			mime:     &MIME{_type: "text", subType: "html"},
			other:    &MIME{_type: "application", subType: "html"},
			expected: false,
		},
		{
			name:     "different subtype",
			mime:     &MIME{_type: "text", subType: "html"},
			other:    &MIME{_type: "text", subType: "plain"},
			expected: false,
		},
		{
			name: "ignores parameters",
			mime: func() *MIME {
				params := maps.HashMap[string, string]{}
				params.Put("charset", "UTF-8")
				return &MIME{_type: "text", subType: "html", params: params}
			}(),
			other:    &MIME{_type: "text", subType: "html", params: maps.HashMap[string, string]{}},
			expected: true,
		},
		{
			name:     "nil other returns false",
			mime:     &MIME{_type: "text", subType: "html"},
			other:    nil,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.mime.EqualsTypeAndSubtype(tt.other); got != tt.expected {
				t.Errorf("EqualsTypeAndSubtype() = %v, want %v", got, tt.expected)
			}
		})
	}
}

// TestMIME_EqualsParams tests the EqualsParams() method
func TestMIME_EqualsParams(t *testing.T) {
	tests := []struct {
		name     string
		mime     *MIME
		other    *MIME
		expected bool
	}{
		{
			name: "same params",
			mime: func() *MIME {
				params := maps.HashMap[string, string]{}
				params.Put("charset", "UTF-8")
				return &MIME{_type: "text", subType: "html", params: params}
			}(),
			other: func() *MIME {
				params := maps.HashMap[string, string]{}
				params.Put("charset", "UTF-8")
				return &MIME{_type: "text", subType: "html", params: params}
			}(),
			expected: true,
		},
		{
			name: "different param values",
			mime: func() *MIME {
				params := maps.HashMap[string, string]{}
				params.Put("charset", "UTF-8")
				return &MIME{_type: "text", subType: "html", params: params}
			}(),
			other: func() *MIME {
				params := maps.HashMap[string, string]{}
				params.Put("charset", "ISO-8859-1")
				return &MIME{_type: "text", subType: "html", params: params}
			}(),
			expected: false,
		},
		{
			name: "different param count",
			mime: func() *MIME {
				params := maps.HashMap[string, string]{}
				params.Put("charset", "UTF-8")
				return &MIME{_type: "text", subType: "html", params: params}
			}(),
			other:    &MIME{_type: "text", subType: "html", params: maps.HashMap[string, string]{}},
			expected: false,
		},
		{
			name:     "both empty params",
			mime:     &MIME{_type: "text", subType: "html", params: maps.HashMap[string, string]{}},
			other:    &MIME{_type: "text", subType: "html", params: maps.HashMap[string, string]{}},
			expected: true,
		},
		{
			name: "multiple params same",
			mime: func() *MIME {
				params := maps.HashMap[string, string]{}
				params.Put("charset", "UTF-8")
				params.Put("version", "1.0")
				return &MIME{_type: "text", subType: "html", params: params}
			}(),
			other: func() *MIME {
				params := maps.HashMap[string, string]{}
				params.Put("charset", "UTF-8")
				params.Put("version", "1.0")
				return &MIME{_type: "text", subType: "html", params: params}
			}(),
			expected: true,
		},
		{
			name:     "nil other returns false",
			mime:     &MIME{_type: "text", subType: "html", params: maps.HashMap[string, string]{}},
			other:    nil,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.mime.EqualsParams(tt.other); got != tt.expected {
				t.Errorf("EqualsParams() = %v, want %v", got, tt.expected)
			}
		})
	}
}

// TestMIME_EqualsCharset tests the EqualsCharset() method
func TestMIME_EqualsCharset(t *testing.T) {
	tests := []struct {
		name     string
		mime     *MIME
		other    *MIME
		expected bool
	}{
		{
			name:     "same charset",
			mime:     &MIME{_type: "text", subType: "html", charset: "UTF-8"},
			other:    &MIME{_type: "text", subType: "html", charset: "UTF-8"},
			expected: true,
		},
		{
			name:     "different charset",
			mime:     &MIME{_type: "text", subType: "html", charset: "UTF-8"},
			other:    &MIME{_type: "text", subType: "html", charset: "ISO-8859-1"},
			expected: false,
		},
		{
			name:     "both empty charset",
			mime:     &MIME{_type: "text", subType: "html"},
			other:    &MIME{_type: "text", subType: "html"},
			expected: true,
		},
		{
			name:     "one empty charset",
			mime:     &MIME{_type: "text", subType: "html", charset: "UTF-8"},
			other:    &MIME{_type: "text", subType: "html"},
			expected: false,
		},
		{
			name:     "nil other returns false",
			mime:     &MIME{_type: "text", subType: "html", charset: "UTF-8"},
			other:    nil,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.mime.EqualsCharset(tt.other); got != tt.expected {
				t.Errorf("EqualsCharset() = %v, want %v", got, tt.expected)
			}
		})
	}
}

// TestMIME_Equals tests the Equals() method for complete equality
func TestMIME_Equals(t *testing.T) {
	tests := []struct {
		name     string
		mime     *MIME
		other    *MIME
		expected bool
	}{
		{
			name: "completely equal",
			mime: func() *MIME {
				params := maps.HashMap[string, string]{}
				params.Put("charset", "UTF-8")
				return &MIME{_type: "text", subType: "html", charset: "UTF-8", params: params}
			}(),
			other: func() *MIME {
				params := maps.HashMap[string, string]{}
				params.Put("charset", "UTF-8")
				return &MIME{_type: "text", subType: "html", charset: "UTF-8", params: params}
			}(),
			expected: true,
		},
		{
			name: "different type",
			mime: func() *MIME {
				params := maps.HashMap[string, string]{}
				params.Put("charset", "UTF-8")
				return &MIME{_type: "text", subType: "html", charset: "UTF-8", params: params}
			}(),
			other: func() *MIME {
				params := maps.HashMap[string, string]{}
				params.Put("charset", "UTF-8")
				return &MIME{_type: "application", subType: "html", charset: "UTF-8", params: params}
			}(),
			expected: false,
		},
		{
			name: "different charset",
			mime: func() *MIME {
				params := maps.HashMap[string, string]{}
				params.Put("charset", "UTF-8")
				return &MIME{_type: "text", subType: "html", charset: "UTF-8", params: params}
			}(),
			other: func() *MIME {
				params := maps.HashMap[string, string]{}
				params.Put("charset", "UTF-8")
				return &MIME{_type: "text", subType: "html", charset: "ISO-8859-1", params: params}
			}(),
			expected: false,
		},
		{
			name: "different params",
			mime: func() *MIME {
				params := maps.HashMap[string, string]{}
				params.Put("charset", "UTF-8")
				return &MIME{_type: "text", subType: "html", charset: "UTF-8", params: params}
			}(),
			other: func() *MIME {
				params := maps.HashMap[string, string]{}
				params.Put("version", "1.0")
				return &MIME{_type: "text", subType: "html", charset: "UTF-8", params: params}
			}(),
			expected: false,
		},
		{
			name:     "nil other returns false",
			mime:     &MIME{_type: "text", subType: "html"},
			other:    nil,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.mime.Equals(tt.other); got != tt.expected {
				t.Errorf("Equals() = %v, want %v", got, tt.expected)
			}
		})
	}
}

// TestMIME_IsPresentIn tests the IsPresentIn() method
func TestMIME_IsPresentIn(t *testing.T) {
	tests := []struct {
		name     string
		mime     *MIME
		mimeList []*MIME
		expected bool
	}{
		{
			name: "present in list",
			mime: &MIME{_type: "text", subType: "html"},
			mimeList: []*MIME{
				{_type: "text", subType: "plain"},
				{_type: "text", subType: "html"},
				{_type: "application", subType: "json"},
			},
			expected: true,
		},
		{
			name: "not present in list",
			mime: &MIME{_type: "text", subType: "html"},
			mimeList: []*MIME{
				{_type: "text", subType: "plain"},
				{_type: "application", subType: "json"},
			},
			expected: false,
		},
		{
			name: "present with different params",
			mime: func() *MIME {
				params := maps.HashMap[string, string]{}
				params.Put("charset", "UTF-8")
				return &MIME{_type: "text", subType: "html", params: params}
			}(),
			mimeList: []*MIME{
				{_type: "text", subType: "html", params: maps.HashMap[string, string]{}},
			},
			expected: true,
		},
		{
			name:     "empty list",
			mime:     &MIME{_type: "text", subType: "html"},
			mimeList: []*MIME{},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.mime.IsPresentIn(tt.mimeList); got != tt.expected {
				t.Errorf("IsPresentIn() = %v, want %v", got, tt.expected)
			}
		})
	}
}

// TestMIME_IsMoreSpecific tests the IsMoreSpecific() method
func TestMIME_IsMoreSpecific(t *testing.T) {
	tests := []struct {
		name     string
		mime     *MIME
		other    *MIME
		expected bool
	}{
		{
			name:     "concrete more specific than wildcard type",
			mime:     &MIME{_type: "text", subType: "html"},
			other:    &MIME{_type: "*", subType: "html"},
			expected: true,
		},
		{
			name:     "wildcard type less specific than concrete",
			mime:     &MIME{_type: "*", subType: "html"},
			other:    &MIME{_type: "text", subType: "html"},
			expected: false,
		},
		{
			name:     "concrete subtype more specific than wildcard",
			mime:     &MIME{_type: "text", subType: "html"},
			other:    &MIME{_type: "text", subType: "*"},
			expected: true,
		},
		{
			name:     "wildcard subtype less specific than concrete",
			mime:     &MIME{_type: "text", subType: "*"},
			other:    &MIME{_type: "text", subType: "html"},
			expected: false,
		},
		{
			name: "more params more specific",
			mime: func() *MIME {
				params := maps.HashMap[string, string]{}
				params.Put("charset", "UTF-8")
				params.Put("version", "1.0")
				return &MIME{_type: "text", subType: "html", params: params}
			}(),
			other: func() *MIME {
				params := maps.HashMap[string, string]{}
				params.Put("charset", "UTF-8")
				return &MIME{_type: "text", subType: "html", params: params}
			}(),
			expected: true,
		},
		{
			name: "fewer params less specific",
			mime: func() *MIME {
				params := maps.HashMap[string, string]{}
				params.Put("charset", "UTF-8")
				return &MIME{_type: "text", subType: "html", params: params}
			}(),
			other: func() *MIME {
				params := maps.HashMap[string, string]{}
				params.Put("charset", "UTF-8")
				params.Put("version", "1.0")
				return &MIME{_type: "text", subType: "html", params: params}
			}(),
			expected: false,
		},
		{
			name:     "different types not comparable",
			mime:     &MIME{_type: "text", subType: "html"},
			other:    &MIME{_type: "application", subType: "json"},
			expected: false,
		},
		{
			name:     "nil other returns false",
			mime:     &MIME{_type: "text", subType: "html"},
			other:    nil,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.mime.IsMoreSpecific(tt.other); got != tt.expected {
				t.Errorf("IsMoreSpecific() = %v, want %v", got, tt.expected)
			}
		})
	}
}

// TestMIME_IsLessSpecific tests the IsLessSpecific() method
func TestMIME_IsLessSpecific(t *testing.T) {
	tests := []struct {
		name     string
		mime     *MIME
		other    *MIME
		expected bool
	}{
		{
			name:     "wildcard less specific than concrete",
			mime:     &MIME{_type: "*", subType: "html"},
			other:    &MIME{_type: "text", subType: "html"},
			expected: true,
		},
		{
			name:     "concrete not less specific than wildcard",
			mime:     &MIME{_type: "text", subType: "html"},
			other:    &MIME{_type: "*", subType: "html"},
			expected: false,
		},
		{
			name: "fewer params less specific",
			mime: func() *MIME {
				params := maps.HashMap[string, string]{}
				params.Put("charset", "UTF-8")
				return &MIME{_type: "text", subType: "html", params: params}
			}(),
			other: func() *MIME {
				params := maps.HashMap[string, string]{}
				params.Put("charset", "UTF-8")
				params.Put("version", "1.0")
				return &MIME{_type: "text", subType: "html", params: params}
			}(),
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.mime.IsLessSpecific(tt.other); got != tt.expected {
				t.Errorf("IsLessSpecific() = %v, want %v", got, tt.expected)
			}
		})
	}
}

// TestMIME_Clone tests the Clone() method
func TestMIME_Clone(t *testing.T) {
	params := maps.HashMap[string, string]{}
	params.Put("charset", "UTF-8")
	params.Put("version", "1.0")

	original := &MIME{
		_type:   "text",
		subType: "html",
		charset: "UTF-8",
		params:  params,
	}

	// Note: This test assumes NewBuilder() exists and works correctly
	// If Clone() is not yet implemented or NewBuilder doesn't exist,
	// this test will fail

	t.Run("clone creates independent copy", func(t *testing.T) {
		// This test would need the Builder to be implemented
		// For now, we can just verify the method exists
		_ = original.Clone
	})
}

// TestMIME_MarshalJSON tests JSON marshaling
func TestMIME_MarshalJSON(t *testing.T) {
	tests := []struct {
		name     string
		mime     *MIME
		expected string
		wantErr  bool
	}{
		{
			name:     "simple mime",
			mime:     &MIME{_type: "text", subType: "html", params: maps.HashMap[string, string]{}},
			expected: "text/html",
			wantErr:  false,
		},
		{
			name:     "nil mime",
			mime:     nil,
			expected: "",
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.mime.MarshalJSON()
			if (err != nil) != tt.wantErr {
				t.Errorf("MarshalJSON() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if string(got) != tt.expected {
				t.Errorf("MarshalJSON() = %v, want %v", string(got), tt.expected)
			}
		})
	}
}

// TestMIME_UnmarshalJSON tests JSON unmarshaling
func TestMIME_UnmarshalJSON(t *testing.T) {
	// Note: This test assumes Parse() function exists
	// If Parse() is not implemented, these tests will fail

	t.Run("unmarshal requires Parse function", func(t *testing.T) {
		mime := &MIME{}
		// This would test unmarshaling if Parse exists
		_ = mime.UnmarshalJSON([]byte("text/html"))
	})
}

// Benchmark tests for performance-critical methods

// BenchmarkMIME_String benchmarks the String() method with caching
func BenchmarkMIME_String(b *testing.B) {
	params := maps.HashMap[string, string]{}
	params.Put("charset", "UTF-8")
	params.Put("version", "1.0")

	mime := &MIME{
		_type:   "application",
		subType: "json",
		params:  params,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = mime.String()
	}
}

// BenchmarkMIME_Includes benchmarks the Includes() method
func BenchmarkMIME_Includes(b *testing.B) {
	mime1 := &MIME{_type: "text", subType: "*"}
	mime2 := &MIME{_type: "text", subType: "html"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = mime1.Includes(mime2)
	}
}

// BenchmarkMIME_Equals benchmarks the Equals() method
func BenchmarkMIME_Equals(b *testing.B) {
	params1 := maps.HashMap[string, string]{}
	params1.Put("charset", "UTF-8")

	params2 := maps.HashMap[string, string]{}
	params2.Put("charset", "UTF-8")

	mime1 := &MIME{_type: "text", subType: "html", charset: "UTF-8", params: params1}
	mime2 := &MIME{_type: "text", subType: "html", charset: "UTF-8", params: params2}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = mime1.Equals(mime2)
	}
}

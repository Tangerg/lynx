package text

import (
	"strings"
	"testing"
)

// TestNewRenderer tests the NewRenderer constructor
func TestNewRenderer(t *testing.T) {
	t.Run("creates non-nil renderer", func(t *testing.T) {
		r := NewRenderer()
		if r == nil {
			t.Fatal("NewRenderer() returned nil")
		}
	})

	t.Run("initializes with default values", func(t *testing.T) {
		r := NewRenderer()

		if r.templateString != "" {
			t.Errorf("templateString = %q, want empty string", r.templateString)
		}

		if r.variables == nil {
			t.Error("variables map is nil")
		}

		if len(r.variables) != 0 {
			t.Errorf("variables count = %d, want 0", len(r.variables))
		}

		if r.leftDelimiter != "{{" {
			t.Errorf("leftDelimiter = %q, want {{", r.leftDelimiter)
		}

		if r.rightDelimiter != "}}" {
			t.Errorf("rightDelimiter = %q, want }}", r.rightDelimiter)
		}

		if r.changed {
			t.Error("changed should be false initially")
		}

		if r.renderedString != "" {
			t.Errorf("renderedString = %q, want empty string", r.renderedString)
		}
	})
}

// TestRenderer_WithTemplate tests the WithTemplate method
func TestRenderer_WithTemplate(t *testing.T) {
	tests := []struct {
		name     string
		template string
	}{
		{
			name:     "simple template",
			template: "Hello {{.name}}",
		},
		{
			name:     "empty template",
			template: "",
		},
		{
			name:     "complex template",
			template: "{{.greeting}} {{.name}}, you are {{.age}} years old",
		},
		{
			name:     "template with special characters",
			template: "Hello {{.name}}! Welcome to our app.",
		},
		{
			name:     "multiline template",
			template: "Line 1: {{.line1}}\nLine 2: {{.line2}}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewRenderer()
			result := r.WithTemplate(tt.template)

			// Should return the same instance for chaining
			if result != r {
				t.Error("WithTemplate() should return the same instance")
			}

			// Should set the template
			if r.templateString != tt.template {
				t.Errorf("templateString = %q, want %q", r.templateString, tt.template)
			}

			// Should mark as changed
			if !r.changed {
				t.Error("changed should be true after WithTemplate()")
			}

			// Should clear cached result
			if r.renderedString != "" {
				t.Error("renderedString should be empty after WithTemplate()")
			}
		})
	}
}

// TestRenderer_WithVariable tests the WithVariable method
func TestRenderer_WithVariable(t *testing.T) {
	tests := []struct {
		name  string
		key   string
		value any
	}{
		{
			name:  "string value",
			key:   "name",
			value: "Alice",
		},
		{
			name:  "int value",
			key:   "age",
			value: 30,
		},
		{
			name:  "bool value",
			key:   "active",
			value: true,
		},
		{
			name:  "nil value",
			key:   "optional",
			value: nil,
		},
		{
			name:  "empty key",
			key:   "",
			value: "value",
		},
		{
			name:  "struct value",
			key:   "user",
			value: struct{ Name string }{Name: "Bob"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewRenderer()
			result := r.WithVariable(tt.key, tt.value)

			// Should return the same instance for chaining
			if result != r {
				t.Error("WithVariable() should return the same instance")
			}

			// Should set the variable
			if val, exists := r.variables[tt.key]; !exists {
				t.Errorf("variable %q not found", tt.key)
			} else if val != tt.value {
				t.Errorf("variable %q = %v, want %v", tt.key, val, tt.value)
			}

			// Should mark as changed
			if !r.changed {
				t.Error("changed should be true after WithVariable()")
			}
		})
	}

	t.Run("overwrites existing variable", func(t *testing.T) {
		r := NewRenderer()
		r.WithVariable("name", "Alice")
		r.WithVariable("name", "Bob")

		if r.variables["name"] != "Bob" {
			t.Errorf("variable 'name' = %v, want Bob", r.variables["name"])
		}
	})

	t.Run("multiple variables", func(t *testing.T) {
		r := NewRenderer()
		r.WithVariable("name", "Alice").
			WithVariable("age", 30).
			WithVariable("active", true)

		if len(r.variables) != 3 {
			t.Errorf("variables count = %d, want 3", len(r.variables))
		}
	})
}

// TestRenderer_WithVariables tests the WithVariables method
func TestRenderer_WithVariables(t *testing.T) {
	t.Run("sets multiple variables", func(t *testing.T) {
		r := NewRenderer()
		vars := map[string]any{
			"name": "Alice",
			"age":  30,
			"city": "NYC",
		}

		result := r.WithVariables(vars)

		// Should return the same instance
		if result != r {
			t.Error("WithVariables() should return the same instance")
		}

		// Should set all variables
		if len(r.variables) != 3 {
			t.Errorf("variables count = %d, want 3", len(r.variables))
		}

		for key, expected := range vars {
			if val, exists := r.variables[key]; !exists {
				t.Errorf("variable %q not found", key)
			} else if val != expected {
				t.Errorf("variable %q = %v, want %v", key, val, expected)
			}
		}

		// Should mark as changed
		if !r.changed {
			t.Error("changed should be true after WithVariables()")
		}
	})

	t.Run("clears existing variables", func(t *testing.T) {
		r := NewRenderer()
		r.WithVariable("old", "value")

		newVars := map[string]any{
			"new": "value",
		}
		r.WithVariables(newVars)

		if _, exists := r.variables["old"]; exists {
			t.Error("old variable should be cleared")
		}

		if len(r.variables) != 1 {
			t.Errorf("variables count = %d, want 1", len(r.variables))
		}
	})

	t.Run("handles nil map", func(t *testing.T) {
		r := NewRenderer()
		r.WithVariable("test", "value")
		r.WithVariables(nil)

		if len(r.variables) != 0 {
			t.Errorf("variables count = %d, want 0", len(r.variables))
		}
	})

	t.Run("handles empty map", func(t *testing.T) {
		r := NewRenderer()
		r.WithVariable("test", "value")
		r.WithVariables(map[string]any{})

		if len(r.variables) != 0 {
			t.Errorf("variables count = %d, want 0", len(r.variables))
		}
	})
}

// TestRenderer_WithDelimiters tests the WithDelimiters method
func TestRenderer_WithDelimiters(t *testing.T) {
	tests := []struct {
		name  string
		left  string
		right string
	}{
		{
			name:  "default delimiters",
			left:  "{{",
			right: "}}",
		},
		{
			name:  "custom delimiters",
			left:  "[[",
			right: "]]",
		},
		{
			name:  "single character delimiters",
			left:  "<",
			right: ">",
		},
		{
			name:  "empty delimiters",
			left:  "",
			right: "",
		},
		{
			name:  "asymmetric delimiters",
			left:  "<%",
			right: "%>",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewRenderer()
			result := r.WithDelimiters(tt.left, tt.right)

			// Should return the same instance
			if result != r {
				t.Error("WithDelimiters() should return the same instance")
			}

			// Should set delimiters
			if r.leftDelimiter != tt.left {
				t.Errorf("leftDelimiter = %q, want %q", r.leftDelimiter, tt.left)
			}

			if r.rightDelimiter != tt.right {
				t.Errorf("rightDelimiter = %q, want %q", r.rightDelimiter, tt.right)
			}

			// Should mark as changed
			if !r.changed {
				t.Error("changed should be true after WithDelimiters()")
			}
		})
	}
}

// TestRenderer_Reset tests the Reset method
func TestRenderer_Reset(t *testing.T) {
	t.Run("resets to initial state", func(t *testing.T) {
		r := NewRenderer()

		// Configure the renderer
		r.WithTemplate("Hello {{.name}}").
			WithVariable("name", "Alice").
			WithDelimiters("[[", "]]")

		// Render to populate cache
		_, _ = r.Render()

		// Reset
		result := r.Reset()

		// Should return the same instance
		if result != r {
			t.Error("Reset() should return the same instance")
		}

		// Should reset all fields
		if r.templateString != "" {
			t.Errorf("templateString = %q, want empty", r.templateString)
		}

		if len(r.variables) != 0 {
			t.Errorf("variables count = %d, want 0", len(r.variables))
		}

		if r.leftDelimiter != "{{" {
			t.Errorf("leftDelimiter = %q, want {{", r.leftDelimiter)
		}

		if r.rightDelimiter != "}}" {
			t.Errorf("rightDelimiter = %q, want }}", r.rightDelimiter)
		}

		if r.changed {
			t.Error("changed should be false after Reset()")
		}

		if r.renderedString != "" {
			t.Errorf("renderedString = %q, want empty", r.renderedString)
		}
	})
}

// TestRenderer_Clone tests the Clone method
func TestRenderer_Clone(t *testing.T) {
	t.Run("creates independent copy", func(t *testing.T) {
		original := NewRenderer()
		original.WithTemplate("Hello {{.name}}").
			WithVariable("name", "Alice").
			WithVariable("age", 30).
			WithDelimiters("[[", "]]")

		// Render to populate cache
		_, _ = original.Render()

		// Clone
		cloned := original.Clone()

		// Should be different instances
		if cloned == original {
			t.Error("Clone() should return a different instance")
		}

		// Should have the same configuration
		if cloned.templateString != original.templateString {
			t.Error("templateString should be copied")
		}

		if len(cloned.variables) != len(original.variables) {
			t.Error("variables count should be the same")
		}

		for key, val := range original.variables {
			if cloned.variables[key] != val {
				t.Errorf("variable %q not copied correctly", key)
			}
		}

		if cloned.leftDelimiter != original.leftDelimiter {
			t.Error("leftDelimiter should be copied")
		}

		if cloned.rightDelimiter != original.rightDelimiter {
			t.Error("rightDelimiter should be copied")
		}

		if cloned.changed != original.changed {
			t.Error("changed flag should be copied")
		}

		if cloned.renderedString != original.renderedString {
			t.Error("renderedString should be copied")
		}
	})

	t.Run("modifications don't affect original", func(t *testing.T) {
		original := NewRenderer()
		original.WithVariable("name", "Alice")

		cloned := original.Clone()
		cloned.WithVariable("name", "Bob")
		cloned.WithVariable("age", 30)

		// Original should be unchanged
		if original.variables["name"] != "Alice" {
			t.Error("original should not be modified")
		}

		if _, exists := original.variables["age"]; exists {
			t.Error("original should not have new variable")
		}

		// Cloned should have modifications
		if cloned.variables["name"] != "Bob" {
			t.Error("cloned should have modified value")
		}

		if cloned.variables["age"] != 30 {
			t.Error("cloned should have new variable")
		}
	})
}

// TestRenderer_Render tests the Render method
func TestRenderer_Render(t *testing.T) {
	tests := []struct {
		name        string
		template    string
		variables   map[string]any
		expected    string
		shouldError bool
	}{
		{
			name:        "simple substitution",
			template:    "Hello {{.name}}",
			variables:   map[string]any{"name": "Alice"},
			expected:    "Hello Alice",
			shouldError: false,
		},
		{
			name:        "multiple variables",
			template:    "{{.greeting}} {{.name}}, you are {{.age}} years old",
			variables:   map[string]any{"greeting": "Hello", "name": "Bob", "age": 30},
			expected:    "Hello Bob, you are 30 years old",
			shouldError: false,
		},
		{
			name:        "no variables",
			template:    "Static text",
			variables:   map[string]any{},
			expected:    "Static text",
			shouldError: false,
		},
		{
			name:        "empty template",
			template:    "",
			variables:   map[string]any{"name": "Alice"},
			expected:    "",
			shouldError: false,
		},
		{
			name:        "missing variable",
			template:    "Hello {{.name}}",
			variables:   map[string]any{},
			expected:    "Hello <no value>",
			shouldError: false,
		},
		{
			name:        "invalid template syntax",
			template:    "Hello {{.name",
			variables:   map[string]any{"name": "Alice"},
			expected:    "",
			shouldError: true,
		},
		{
			name:        "with conditionals",
			template:    "{{if .show}}Hello {{.name}}{{end}}",
			variables:   map[string]any{"show": true, "name": "Alice"},
			expected:    "Hello Alice",
			shouldError: false,
		},
		{
			name:        "with range",
			template:    "{{range .items}}{{.}} {{end}}",
			variables:   map[string]any{"items": []string{"a", "b", "c"}},
			expected:    "a b c ",
			shouldError: false,
		},
		{
			name:        "nested struct",
			template:    "{{.user.name}}",
			variables:   map[string]any{"user": map[string]any{"name": "Alice"}},
			expected:    "Alice",
			shouldError: false,
		},
		{
			name:        "unicode content",
			template:    "你好 {{.name}}",
			variables:   map[string]any{"name": "世界"},
			expected:    "你好 世界",
			shouldError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewRenderer()
			r.WithTemplate(tt.template).WithVariables(tt.variables)

			result, err := r.Render()

			if tt.shouldError {
				if err == nil {
					t.Error("Render() should return error")
				}
				return
			}

			if err != nil {
				t.Errorf("Render() unexpected error: %v", err)
				return
			}

			if result != tt.expected {
				t.Errorf("Render() = %q, want %q", result, tt.expected)
			}
		})
	}

	t.Run("caches result", func(t *testing.T) {
		r := NewRenderer()
		r.WithTemplate("Hello {{.name}}").WithVariable("name", "Alice")

		// First render
		result1, err := r.Render()
		if err != nil {
			t.Fatalf("First Render() error: %v", err)
		}

		if r.changed {
			t.Error("changed should be false after first Render()")
		}

		if r.renderedString == "" {
			t.Error("renderedString should be cached")
		}

		// Second render should use cache
		result2, err := r.Render()
		if err != nil {
			t.Fatalf("Second Render() error: %v", err)
		}

		if result1 != result2 {
			t.Error("cached result should be the same")
		}
	})

	t.Run("invalidates cache on change", func(t *testing.T) {
		r := NewRenderer()
		r.WithTemplate("Hello {{.name}}").WithVariable("name", "Alice")

		// First render
		result1, _ := r.Render()

		// Change variable
		r.WithVariable("name", "Bob")

		if !r.changed {
			t.Error("changed should be true after modification")
		}

		if r.renderedString != "" {
			t.Error("renderedString should be cleared")
		}

		// Second render should give different result
		result2, _ := r.Render()

		if result1 == result2 {
			t.Error("result should be different after change")
		}
	})
}

// TestRenderer_MustRender tests the MustRender method
func TestRenderer_MustRender(t *testing.T) {
	t.Run("returns result on success", func(t *testing.T) {
		r := NewRenderer()
		r.WithTemplate("Hello {{.name}}").WithVariable("name", "Alice")

		result := r.MustRender()

		if result != "Hello Alice" {
			t.Errorf("MustRender() = %q, want 'Hello Alice'", result)
		}
	})

	t.Run("panics on error", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Error("MustRender() should panic on error")
			}
		}()

		r := NewRenderer()
		r.WithTemplate("Hello {{.name")
		r.MustRender()
	})
}

// TestRenderer_RequireVariables tests the RequireVariables method
func TestRenderer_RequireVariables(t *testing.T) {
	t.Run("passes when all variables present", func(t *testing.T) {
		r := NewRenderer()
		r.WithTemplate("Hello {{.name}}, you are {{.age}} years old")

		err := r.RequireVariables("name", "age")

		if err != nil {
			t.Errorf("RequireVariables() unexpected error: %v", err)
		}
	})

	t.Run("fails when variable missing", func(t *testing.T) {
		r := NewRenderer()
		r.WithTemplate("Hello {{.name}}")

		err := r.RequireVariables("name", "age")

		if err == nil {
			t.Error("RequireVariables() should return error")
		}

		if !strings.Contains(err.Error(), "age") {
			t.Errorf("error should mention missing variable 'age': %v", err)
		}
	})

	t.Run("fails when multiple variables missing", func(t *testing.T) {
		r := NewRenderer()
		r.WithTemplate("Hello")

		err := r.RequireVariables("name", "age", "city")

		if err == nil {
			t.Error("RequireVariables() should return error")
		}

		errMsg := err.Error()
		if !strings.Contains(errMsg, "name") || !strings.Contains(errMsg, "age") || !strings.Contains(errMsg, "city") {
			t.Errorf("error should mention all missing variables: %v", err)
		}
	})

	t.Run("works with custom delimiters", func(t *testing.T) {
		r := NewRenderer()
		r.WithDelimiters("[[", "]]").WithTemplate("Hello [[.name]]")

		err := r.RequireVariables("name")

		if err != nil {
			t.Errorf("RequireVariables() unexpected error: %v", err)
		}
	})

	t.Run("no error when no variables required", func(t *testing.T) {
		r := NewRenderer()
		r.WithTemplate("Hello {{.name}}")

		err := r.RequireVariables()

		if err != nil {
			t.Errorf("RequireVariables() unexpected error: %v", err)
		}
	})

	t.Run("empty template with required variables", func(t *testing.T) {
		r := NewRenderer()
		r.WithTemplate("")

		err := r.RequireVariables("name")

		if err == nil {
			t.Error("RequireVariables() should return error for empty template")
		}
	})
}

// TestRender tests the package-level Render function
func TestRender(t *testing.T) {
	tests := []struct {
		name        string
		template    string
		data        map[string]any
		expected    string
		shouldError bool
	}{
		{
			name:        "simple render",
			template:    "Hello {{.name}}",
			data:        map[string]any{"name": "Alice"},
			expected:    "Hello Alice",
			shouldError: false,
		},
		{
			name:        "multiple variables",
			template:    "{{.a}} + {{.b}} = {{.c}}",
			data:        map[string]any{"a": 1, "b": 2, "c": 3},
			expected:    "1 + 2 = 3",
			shouldError: false,
		},
		{
			name:        "empty template",
			template:    "",
			data:        map[string]any{},
			expected:    "",
			shouldError: false,
		},
		{
			name:        "nil data",
			template:    "Static",
			data:        nil,
			expected:    "Static",
			shouldError: false,
		},
		{
			name:        "invalid template",
			template:    "{{.name",
			data:        map[string]any{"name": "Alice"},
			expected:    "",
			shouldError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := Render(tt.template, tt.data)

			if tt.shouldError {
				if err == nil {
					t.Error("Render() should return error")
				}
				return
			}

			if err != nil {
				t.Errorf("Render() unexpected error: %v", err)
				return
			}

			if result != tt.expected {
				t.Errorf("Render() = %q, want %q", result, tt.expected)
			}
		})
	}
}

// TestMustRender tests the package-level MustRender function
func TestMustRender(t *testing.T) {
	t.Run("returns result on success", func(t *testing.T) {
		result := MustRender("Hello {{.name}}", map[string]any{"name": "Alice"})

		if result != "Hello Alice" {
			t.Errorf("MustRender() = %q, want 'Hello Alice'", result)
		}
	})

	t.Run("panics on error", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Error("MustRender() should panic on error")
			}
		}()

		MustRender("{{.name", map[string]any{"name": "Alice"})
	})
}

// TestRenderer_MethodChaining tests method chaining
func TestRenderer_MethodChaining(t *testing.T) {
	t.Run("all methods chainable", func(t *testing.T) {
		result, err := NewRenderer().
			WithTemplate("Hello {{.name}}, you are {{.age}} years old in {{.city}}").
			WithVariable("name", "Alice").
			WithVariable("age", 30).
			WithVariable("city", "NYC").
			WithDelimiters("{{", "}}").
			Render()

		if err != nil {
			t.Fatalf("Render() error: %v", err)
		}

		expected := "Hello Alice, you are 30 years old in NYC"
		if result != expected {
			t.Errorf("result = %q, want %q", result, expected)
		}
	})

	t.Run("reset and reconfigure", func(t *testing.T) {
		r := NewRenderer()

		// First configuration
		r.WithTemplate("Hello {{.name}}").WithVariable("name", "Alice")
		result1, _ := r.Render()

		// Reset and reconfigure
		result2, err := r.Reset().
			WithTemplate("Hi {{.name}}").
			WithVariable("name", "Bob").
			Render()

		if err != nil {
			t.Fatalf("Render() error: %v", err)
		}

		if result1 == result2 {
			t.Error("results should be different after reset")
		}

		if result2 != "Hi Bob" {
			t.Errorf("result = %q, want 'Hi Bob'", result2)
		}
	})
}

// TestRenderer_EdgeCases tests edge cases
func TestRenderer_EdgeCases(t *testing.T) {
	t.Run("very long template", func(t *testing.T) {
		template := strings.Repeat("{{.x}} ", 1000)
		r := NewRenderer()
		r.WithTemplate(template).WithVariable("x", "a")

		result, err := r.Render()
		if err != nil {
			t.Errorf("Render() error: %v", err)
		}

		if len(result) == 0 {
			t.Error("result should not be empty")
		}
	})

	t.Run("deeply nested data", func(t *testing.T) {
		data := map[string]any{
			"level1": map[string]any{
				"level2": map[string]any{
					"level3": "value",
				},
			},
		}

		r := NewRenderer()
		r.WithTemplate("{{.level1.level2.level3}}").WithVariables(data)

		result, err := r.Render()
		if err != nil {
			t.Errorf("Render() error: %v", err)
		}

		if result != "value" {
			t.Errorf("result = %q, want 'value'", result)
		}
	})

	t.Run("special characters in template", func(t *testing.T) {
		template := "Price: ${{.price}}\nTax: {{.tax}}%"
		r := NewRenderer()
		r.WithTemplate(template).
			WithVariable("price", 100).
			WithVariable("tax", 10)

		result, err := r.Render()
		if err != nil {
			t.Errorf("Render() error: %v", err)
		}

		expected := "Price: $100\nTax: 10%"
		if result != expected {
			t.Errorf("result = %q, want %q", result, expected)
		}
	})

	t.Run("unicode variables", func(t *testing.T) {
		r := NewRenderer()
		r.WithTemplate("{{.中文}} {{.日本語}} {{.한국어}}").
			WithVariable("中文", "你好").
			WithVariable("日本語", "こんにちは").
			WithVariable("한국어", "안녕하세요")

		result, err := r.Render()
		if err != nil {
			t.Errorf("Render() error: %v", err)
		}

		expected := "你好 こんにちは 안녕하세요"
		if result != expected {
			t.Errorf("result = %q, want %q", result, expected)
		}
	})
}

// BenchmarkRenderer benchmarks the Renderer
func BenchmarkRenderer_Render(b *testing.B) {
	r := NewRenderer()
	r.WithTemplate("Hello {{.name}}, you are {{.age}} years old").
		WithVariable("name", "Alice").
		WithVariable("age", 30)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = r.Render()
	}
}

func BenchmarkRenderer_RenderWithCache(b *testing.B) {
	r := NewRenderer()
	r.WithTemplate("Hello {{.name}}, you are {{.age}} years old").
		WithVariable("name", "Alice").
		WithVariable("age", 30)

	// First render to populate cache
	_, _ = r.Render()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = r.Render()
	}
}

func BenchmarkRender_PackageFunction(b *testing.B) {
	template := "Hello {{.name}}, you are {{.age}} years old"
	data := map[string]any{"name": "Alice", "age": 30}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = Render(template, data)
	}
}

func BenchmarkRenderer_Clone(b *testing.B) {
	r := NewRenderer()
	r.WithTemplate("Hello {{.name}}").
		WithVariable("name", "Alice").
		WithVariable("age", 30).
		WithVariable("city", "NYC")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = r.Clone()
	}
}

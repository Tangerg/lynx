package chat

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRemoveMarkdownCodeBlockDelimiters tests the markdown code block removal utility
func TestRemoveMarkdownCodeBlockDelimiters(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Standard JSON code block",
			input:    "```json\n{\"key\": \"value\"}\n```",
			expected: "{\"key\": \"value\"}",
		},
		{
			name:     "Uppercase JSON code block",
			input:    "```JSON\n{\"key\": \"value\"}\n```",
			expected: "{\"key\": \"value\"}",
		},
		{
			name:     "Plain code block without language",
			input:    "```\n{\"key\": \"value\"}\n```",
			expected: "{\"key\": \"value\"}",
		},
		{
			name:     "Single line code block",
			input:    "```{\"key\": \"value\"}```",
			expected: "{\"key\": \"value\"}",
		},
		{
			name:     "Multi-line with extra whitespace",
			input:    "```json\n  {\n    \"key\": \"value\"\n  }\n```",
			expected: "{\n    \"key\": \"value\"\n  }",
		},
		{
			name:     "No code block markers",
			input:    "{\"key\": \"value\"}",
			expected: "{\"key\": \"value\"}",
		},
		{
			name:     "Only opening marker",
			input:    "```json\n{\"key\": \"value\"}",
			expected: "```json\n{\"key\": \"value\"}",
		},
		{
			name:     "Only closing marker",
			input:    "{\"key\": \"value\"}\n```",
			expected: "{\"key\": \"value\"}\n```",
		},
		{
			name:     "Empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "Very short string",
			input:    "test",
			expected: "test",
		},
		{
			name:     "Whitespace only",
			input:    "   ",
			expected: "",
		},
		{
			name:     "Code block with leading/trailing whitespace",
			input:    "  ```json\n{\"key\": \"value\"}\n```  ",
			expected: "{\"key\": \"value\"}",
		},
		{
			name:     "Code block with language identifier and extra text",
			input:    "```javascript\nconst x = 1;\n```",
			expected: "const x = 1;",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := removeMarkdownCodeBlockDelimiters(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestListParser tests the ListParser implementation
func TestListParser(t *testing.T) {
	parser := NewListParser()

	t.Run("Constructor", func(t *testing.T) {
		assert.NotNil(t, parser)
	})

	t.Run("Instructions not empty", func(t *testing.T) {
		instructions := parser.Instructions()
		assert.NotEmpty(t, instructions)
		assert.Contains(t, instructions, "Comma-separated")
		assert.Contains(t, instructions, "OUTPUT FORMAT")
	})

	t.Run("Parse simple comma-separated list", func(t *testing.T) {
		input := "apple, banana, cherry"
		result, err := parser.Parse(input)

		require.NoError(t, err)
		assert.Equal(t, []string{"apple", "banana", "cherry"}, result)
	})

	t.Run("Parse with extra whitespace", func(t *testing.T) {
		input := "apple,  banana  ,   cherry"
		result, err := parser.Parse(input)

		require.NoError(t, err)
		assert.Equal(t, []string{"apple", "banana", "cherry"}, result)
	})

	t.Run("Parse single item", func(t *testing.T) {
		input := "apple"
		result, err := parser.Parse(input)

		require.NoError(t, err)
		assert.Equal(t, []string{"apple"}, result)
	})

	t.Run("Parse empty string", func(t *testing.T) {
		input := ""
		result, err := parser.Parse(input)

		require.NoError(t, err)
		assert.Equal(t, []string{""}, result)
	})

	t.Run("Parse with leading/trailing whitespace", func(t *testing.T) {
		input := "  apple, banana, cherry  "
		result, err := parser.Parse(input)

		require.NoError(t, err)
		assert.Equal(t, []string{"apple", "banana", "cherry"}, result)
	})

	t.Run("Parse with newlines", func(t *testing.T) {
		input := "apple,\nbanana,\ncherry"
		result, err := parser.Parse(input)

		require.NoError(t, err)
		assert.Len(t, result, 3)
		assert.Equal(t, "apple", result[0])
		assert.Equal(t, "banana", result[1])
		assert.Equal(t, "cherry", result[2])
	})

	t.Run("Parse with commas in quoted text", func(t *testing.T) {
		// Note: This parser doesn't handle quoted strings specially
		input := `"item, with, comma", normal item`
		result, err := parser.Parse(input)

		require.NoError(t, err)
		// Will split on all commas
		assert.True(t, len(result) > 2)
	})

	t.Run("Parse multiple consecutive commas", func(t *testing.T) {
		input := "apple,,banana"
		result, err := parser.Parse(input)

		require.NoError(t, err)
		assert.Equal(t, []string{"apple", "", "banana"}, result)
	})
}

// TestMapParser tests the MapParser implementation
func TestMapParser(t *testing.T) {
	parser := NewMapParser()

	t.Run("Constructor", func(t *testing.T) {
		assert.NotNil(t, parser)
	})

	t.Run("Instructions not empty", func(t *testing.T) {
		instructions := parser.Instructions()
		assert.NotEmpty(t, instructions)
		assert.Contains(t, instructions, "JSON object")
		assert.Contains(t, instructions, "RFC8259")
		assert.Contains(t, instructions, "OUTPUT FORMAT")
	})

	t.Run("Parse valid JSON object", func(t *testing.T) {
		input := `{"name": "John", "age": 30, "active": true}`
		result, err := parser.Parse(input)

		require.NoError(t, err)
		assert.Equal(t, "John", result["name"])
		assert.Equal(t, float64(30), result["age"]) // JSON numbers decode to float64
		assert.Equal(t, true, result["active"])
	})

	t.Run("Parse JSON with markdown code blocks", func(t *testing.T) {
		input := "```json\n{\"key\": \"value\"}\n```"
		result, err := parser.Parse(input)

		require.NoError(t, err)
		assert.Equal(t, "value", result["key"])
	})

	t.Run("Parse nested JSON object", func(t *testing.T) {
		input := `{"user": {"name": "John", "age": 30}}`
		result, err := parser.Parse(input)

		require.NoError(t, err)
		userMap := result["user"].(map[string]any)
		assert.Equal(t, "John", userMap["name"])
		assert.Equal(t, float64(30), userMap["age"])
	})

	t.Run("Parse JSON array values", func(t *testing.T) {
		input := `{"items": ["apple", "banana", "cherry"]}`
		result, err := parser.Parse(input)

		require.NoError(t, err)
		items := result["items"].([]any)
		assert.Len(t, items, 3)
		assert.Equal(t, "apple", items[0])
	})

	t.Run("Parse empty JSON object", func(t *testing.T) {
		input := `{}`
		result, err := parser.Parse(input)

		require.NoError(t, err)
		assert.Empty(t, result)
	})

	t.Run("Parse JSON with extra whitespace", func(t *testing.T) {
		input := `  {  "key"  :  "value"  }  `
		result, err := parser.Parse(input)

		require.NoError(t, err)
		assert.Equal(t, "value", result["key"])
	})

	t.Run("Error on invalid JSON", func(t *testing.T) {
		input := `{invalid json}`
		result, err := parser.Parse(input)

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "failed to parse JSON content")
	})

	t.Run("Error on JSON array instead of object", func(t *testing.T) {
		input := `["item1", "item2"]`
		result, err := parser.Parse(input)

		assert.Error(t, err)
		assert.Nil(t, result)
	})

	t.Run("Error on plain string", func(t *testing.T) {
		input := `"just a string"`
		result, err := parser.Parse(input)

		assert.Error(t, err)
		assert.Nil(t, result)
	})

	t.Run("Parse JSON with special characters", func(t *testing.T) {
		input := `{"message": "Hello\nWorld\t!", "emoji": "😀"}`
		result, err := parser.Parse(input)

		require.NoError(t, err)
		assert.Contains(t, result["message"], "Hello")
		assert.Equal(t, "😀", result["emoji"])
	})

	t.Run("Parse JSON with null values", func(t *testing.T) {
		input := `{"key": null}`
		result, err := parser.Parse(input)

		require.NoError(t, err)
		assert.Nil(t, result["key"])
	})
}

// TestJSONParser tests the generic JSONParser implementation
func TestJSONParser(t *testing.T) {
	// Test struct for parsing
	type Person struct {
		Name   string `json:"name"`
		Age    int    `json:"age"`
		Active bool   `json:"active"`
	}

	type ComplexStruct struct {
		ID      int      `json:"id"`
		Tags    []string `json:"tags"`
		Metrics struct {
			Score float64 `json:"score"`
			Rank  int     `json:"rank"`
		} `json:"metrics"`
	}

	t.Run("Constructor for simple struct", func(t *testing.T) {
		parser := NewJSONParser[Person]()
		assert.NotNil(t, parser)
	})

	t.Run("Instructions generation", func(t *testing.T) {
		parser := NewJSONParser[Person]()
		instructions := parser.Instructions()

		assert.NotEmpty(t, instructions)
		assert.Contains(t, instructions, "JSON SCHEMA")
		assert.Contains(t, instructions, "RFC8259")
		assert.Contains(t, instructions, "OUTPUT FORMAT")
	})

	t.Run("Instructions are cached", func(t *testing.T) {
		parser := NewJSONParser[Person]()
		instructions1 := parser.Instructions()
		instructions2 := parser.Instructions()

		// Should return the same cached string
		assert.Equal(t, instructions1, instructions2)
	})

	t.Run("Parse valid JSON to struct", func(t *testing.T) {
		parser := NewJSONParser[Person]()
		input := `{"name": "John Doe", "age": 30, "active": true}`

		result, err := parser.Parse(input)

		require.NoError(t, err)
		assert.Equal(t, "John Doe", result.Name)
		assert.Equal(t, 30, result.Age)
		assert.Equal(t, true, result.Active)
	})

	t.Run("Parse JSON with markdown code blocks", func(t *testing.T) {
		parser := NewJSONParser[Person]()
		input := "```json\n{\"name\": \"Jane\", \"age\": 25, \"active\": false}\n```"

		result, err := parser.Parse(input)

		require.NoError(t, err)
		assert.Equal(t, "Jane", result.Name)
		assert.Equal(t, 25, result.Age)
		assert.Equal(t, false, result.Active)
	})

	t.Run("Parse complex nested struct", func(t *testing.T) {
		parser := NewJSONParser[ComplexStruct]()
		input := `{
			"id": 123,
			"tags": ["tag1", "tag2"],
			"metrics": {
				"score": 95.5,
				"rank": 1
			}
		}`

		result, err := parser.Parse(input)

		require.NoError(t, err)
		assert.Equal(t, 123, result.ID)
		assert.Equal(t, []string{"tag1", "tag2"}, result.Tags)
		assert.Equal(t, 95.5, result.Metrics.Score)
		assert.Equal(t, 1, result.Metrics.Rank)
	})

	t.Run("Parse to slice type", func(t *testing.T) {
		parser := NewJSONParser[[]string]()
		input := `["apple", "banana", "cherry"]`

		result, err := parser.Parse(input)

		require.NoError(t, err)
		assert.Equal(t, []string{"apple", "banana", "cherry"}, result)
	})

	t.Run("Parse to map type", func(t *testing.T) {
		parser := NewJSONParser[map[string]int]()
		input := `{"a": 1, "b": 2, "c": 3}`

		result, err := parser.Parse(input)

		require.NoError(t, err)
		assert.Equal(t, 1, result["a"])
		assert.Equal(t, 2, result["b"])
		assert.Equal(t, 3, result["c"])
	})

	t.Run("Error on invalid JSON", func(t *testing.T) {
		parser := NewJSONParser[Person]()
		input := `{invalid json}`

		result, err := parser.Parse(input)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to parse JSON content")
		assert.Empty(t, result.Name)
	})

	t.Run("Error on type mismatch", func(t *testing.T) {
		parser := NewJSONParser[Person]()
		input := `{"name": 123, "age": "not a number"}`

		_, err := parser.Parse(input)

		assert.Error(t, err)
		// Type mismatch should cause parsing error
		assert.Contains(t, err.Error(), "failed to parse JSON content")
	})

	t.Run("Parse with missing optional fields", func(t *testing.T) {
		parser := NewJSONParser[Person]()
		input := `{"name": "John"}`

		result, err := parser.Parse(input)

		require.NoError(t, err)
		assert.Equal(t, "John", result.Name)
		assert.Equal(t, 0, result.Age)        // Zero value for int
		assert.Equal(t, false, result.Active) // Zero value for bool
	})

	t.Run("Parse empty JSON object to struct", func(t *testing.T) {
		parser := NewJSONParser[Person]()
		input := `{}`

		result, err := parser.Parse(input)

		require.NoError(t, err)
		assert.Empty(t, result.Name)
		assert.Equal(t, 0, result.Age)
		assert.False(t, result.Active)
	})

	t.Run("Parse with extra whitespace", func(t *testing.T) {
		parser := NewJSONParser[Person]()
		input := `  {  "name"  :  "John"  ,  "age"  :  30  }  `

		result, err := parser.Parse(input)

		require.NoError(t, err)
		assert.Equal(t, "John", result.Name)
		assert.Equal(t, 30, result.Age)
	})
}

// TestAnyParser tests the AnyParser wrapper
func TestAnyParser(t *testing.T) {
	t.Run("Instructions delegation", func(t *testing.T) {
		baseParser := NewListParser()
		anyParser := WrapParserAsAny(baseParser)

		assert.Equal(t, baseParser.Instructions(), anyParser.Instructions())
	})

	t.Run("Parse delegation for ListParser", func(t *testing.T) {
		baseParser := NewListParser()
		anyParser := WrapParserAsAny(baseParser)

		input := "apple, banana, cherry"
		result, err := anyParser.Parse(input)

		require.NoError(t, err)

		// Result should be convertible to []string
		strSlice, ok := result.([]string)
		require.True(t, ok)
		assert.Equal(t, []string{"apple", "banana", "cherry"}, strSlice)
	})

	t.Run("Parse delegation for MapParser", func(t *testing.T) {
		baseParser := NewMapParser()
		anyParser := WrapParserAsAny(baseParser)

		input := `{"key": "value"}`
		result, err := anyParser.Parse(input)

		require.NoError(t, err)

		resultMap, ok := result.(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "value", resultMap["key"])
	})

	t.Run("Parse delegation for JSONParser", func(t *testing.T) {
		type TestStruct struct {
			Name string `json:"name"`
		}

		baseParser := NewJSONParser[TestStruct]()
		anyParser := WrapParserAsAny(baseParser)

		input := `{"name": "test"}`
		result, err := anyParser.Parse(input)

		require.NoError(t, err)

		testStruct, ok := result.(TestStruct)
		require.True(t, ok)
		assert.Equal(t, "test", testStruct.Name)
	})

	t.Run("Error propagation", func(t *testing.T) {
		baseParser := NewMapParser()
		anyParser := WrapParserAsAny(baseParser)

		input := `{invalid json}`
		result, err := anyParser.Parse(input)

		assert.Error(t, err)
		assert.Nil(t, result)
	})

	t.Run("Nil parse function error", func(t *testing.T) {
		anyParser := &AnyParser{
			FormatInstructions: "test instructions",
			ParseFunction:      nil,
		}

		result, err := anyParser.Parse("test")

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "parse function is not initialized")
	})
}

// TestListParserAsAny tests the ListParserAsAny convenience function
func TestListParserAsAny(t *testing.T) {
	t.Run("Creates valid AnyParser", func(t *testing.T) {
		parser := ListParserAsAny()

		assert.NotNil(t, parser)
		assert.NotEmpty(t, parser.Instructions())
		assert.NotNil(t, parser.ParseFunction)
	})

	t.Run("Parse functionality works", func(t *testing.T) {
		parser := ListParserAsAny()
		input := "item1, item2, item3"

		result, err := parser.Parse(input)

		require.NoError(t, err)
		strSlice, ok := result.([]string)
		require.True(t, ok)
		assert.Len(t, strSlice, 3)
	})
}

// TestMapParserAsAny tests the MapParserAsAny convenience function
func TestMapParserAsAny(t *testing.T) {
	t.Run("Creates valid AnyParser", func(t *testing.T) {
		parser := MapParserAsAny()

		assert.NotNil(t, parser)
		assert.NotEmpty(t, parser.Instructions())
		assert.NotNil(t, parser.ParseFunction)
	})

	t.Run("Parse functionality works", func(t *testing.T) {
		parser := MapParserAsAny()
		input := `{"key": "value"}`

		result, err := parser.Parse(input)

		require.NoError(t, err)
		resultMap, ok := result.(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "value", resultMap["key"])
	})
}

// TestJSONParserAsAnyOf tests the JSONParserAsAnyOf convenience function
func TestJSONParserAsAnyOf(t *testing.T) {
	type TestStruct struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	}

	t.Run("Creates valid AnyParser", func(t *testing.T) {
		parser := JSONParserAsAnyOf[TestStruct]()

		assert.NotNil(t, parser)
		assert.NotEmpty(t, parser.Instructions())
		assert.NotNil(t, parser.ParseFunction)
	})

	t.Run("Parse functionality works", func(t *testing.T) {
		parser := JSONParserAsAnyOf[TestStruct]()
		input := `{"id": 123, "name": "test"}`

		result, err := parser.Parse(input)

		require.NoError(t, err)
		testStruct, ok := result.(TestStruct)
		require.True(t, ok)
		assert.Equal(t, 123, testStruct.ID)
		assert.Equal(t, "test", testStruct.Name)
	})

	t.Run("Works with slice types", func(t *testing.T) {
		parser := JSONParserAsAnyOf[[]int]()
		input := `[1, 2, 3, 4, 5]`

		result, err := parser.Parse(input)

		require.NoError(t, err)
		intSlice, ok := result.([]int)
		require.True(t, ok)
		assert.Equal(t, []int{1, 2, 3, 4, 5}, intSlice)
	})
}

// TestStructuredParserInterface tests that all parsers implement the interface correctly
func TestStructuredParserInterface(t *testing.T) {
	t.Run("ListParser implements StructuredParser", func(t *testing.T) {
		var parser StructuredParser[[]string]
		parser = NewListParser()
		assert.NotNil(t, parser)
	})

	t.Run("MapParser implements StructuredParser", func(t *testing.T) {
		var parser StructuredParser[map[string]any]
		parser = NewMapParser()
		assert.NotNil(t, parser)
	})

	t.Run("JSONParser implements StructuredParser", func(t *testing.T) {
		type TestType struct{}
		var parser StructuredParser[TestType]
		parser = NewJSONParser[TestType]()
		assert.NotNil(t, parser)
	})

	t.Run("AnyParser implements StructuredParser", func(t *testing.T) {
		var parser StructuredParser[any]
		parser = ListParserAsAny()
		assert.NotNil(t, parser)
	})
}

// TestEdgeCases tests various edge cases and boundary conditions
func TestEdgeCases(t *testing.T) {
	t.Run("Very long comma-separated list", func(t *testing.T) {
		parser := NewListParser()
		items := make([]string, 1000)
		for i := range items {
			items[i] = "item"
		}
		input := strings.Join(items, ", ")

		result, err := parser.Parse(input)

		require.NoError(t, err)
		assert.Len(t, result, 1000)
	})

	t.Run("Very large JSON object", func(t *testing.T) {
		parser := NewMapParser()
		largeMap := make(map[string]any)
		for i := 0; i < 1000; i++ {
			largeMap[fmt.Sprintf("key%d", i)] = i
		}
		input, _ := json.Marshal(largeMap)

		result, err := parser.Parse(string(input))

		require.NoError(t, err)
		assert.Len(t, result, 1000)
	})

	t.Run("Unicode characters in list", func(t *testing.T) {
		parser := NewListParser()
		input := "苹果, 香蕉, 🍒"

		result, err := parser.Parse(input)

		require.NoError(t, err)
		assert.Equal(t, []string{"苹果", "香蕉", "🍒"}, result)
	})

	t.Run("Unicode characters in JSON", func(t *testing.T) {
		parser := NewMapParser()
		input := `{"中文": "测试", "emoji": "😀"}`

		result, err := parser.Parse(input)

		require.NoError(t, err)
		assert.Equal(t, "测试", result["中文"])
		assert.Equal(t, "😀", result["emoji"])
	})

	t.Run("Deeply nested JSON structure", func(t *testing.T) {
		type DeepStruct struct {
			Level1 struct {
				Level2 struct {
					Level3 struct {
						Value string `json:"value"`
					} `json:"level3"`
				} `json:"level2"`
			} `json:"level1"`
		}

		parser := NewJSONParser[DeepStruct]()
		input := `{"level1": {"level2": {"level3": {"value": "deep"}}}}`

		result, err := parser.Parse(input)

		require.NoError(t, err)
		assert.Equal(t, "deep", result.Level1.Level2.Level3.Value)
	})
}

// BenchmarkListParser benchmarks the ListParser performance
func BenchmarkListParser(b *testing.B) {
	parser := NewListParser()
	input := "apple, banana, cherry, date, elderberry"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = parser.Parse(input)
	}
}

// BenchmarkMapParser benchmarks the MapParser performance
func BenchmarkMapParser(b *testing.B) {
	parser := NewMapParser()
	input := `{"name": "John", "age": 30, "city": "New York"}`

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = parser.Parse(input)
	}
}

// BenchmarkJSONParser benchmarks the JSONParser performance
func BenchmarkJSONParser(b *testing.B) {
	type Person struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
		City string `json:"city"`
	}

	parser := NewJSONParser[Person]()
	input := `{"name": "John", "age": 30, "city": "New York"}`

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = parser.Parse(input)
	}
}

// BenchmarkRemoveMarkdownCodeBlockDelimiters benchmarks the delimiter removal
func BenchmarkRemoveMarkdownCodeBlockDelimiters(b *testing.B) {
	input := "```json\n{\"key\": \"value\"}\n```"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = removeMarkdownCodeBlockDelimiters(input)
	}
}

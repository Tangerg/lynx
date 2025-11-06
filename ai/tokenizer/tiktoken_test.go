package tokenizer

import (
	"context"
	"testing"

	"github.com/pkoukk/tiktoken-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Tangerg/lynx/ai/media"
	"github.com/Tangerg/lynx/pkg/mime"
)

// TestNewTiktokenWithCL100KBase tests the convenience constructor
func TestNewTiktokenWithCL100KBase(t *testing.T) {
	t.Run("creates tiktoken with CL100K_BASE encoding", func(t *testing.T) {
		tk := NewTiktokenWithCL100KBase()

		require.NotNil(t, tk)
		assert.Equal(t, tiktoken.MODEL_CL100K_BASE, tk.encodingName)
		assert.NotNil(t, tk.encoding)
	})

	t.Run("multiple calls create independent instances", func(t *testing.T) {
		tk1 := NewTiktokenWithCL100KBase()
		tk2 := NewTiktokenWithCL100KBase()

		assert.NotSame(t, tk1, tk2)
	})
}

// TestNewTiktoken tests the main constructor
func TestNewTiktoken(t *testing.T) {
	t.Run("valid encoding name", func(t *testing.T) {
		tk, err := NewTiktoken(tiktoken.MODEL_CL100K_BASE)

		require.NoError(t, err)
		require.NotNil(t, tk)
		assert.Equal(t, tiktoken.MODEL_CL100K_BASE, tk.encodingName)
		assert.NotNil(t, tk.encoding)
	})

	t.Run("GPT-3.5 encoding", func(t *testing.T) {
		tk, err := NewTiktoken("cl100k_base")

		require.NoError(t, err)
		require.NotNil(t, tk)
		assert.Equal(t, "cl100k_base", tk.encodingName)
	})

	t.Run("invalid encoding name", func(t *testing.T) {
		tk, err := NewTiktoken("invalid_encoding")

		require.Error(t, err)
		assert.Nil(t, tk)
	})

	t.Run("empty encoding name", func(t *testing.T) {
		tk, err := NewTiktoken("")

		require.Error(t, err)
		assert.Nil(t, tk)
	})
}

// TestTiktoken_EstimateText tests text token estimation
func TestTiktoken_EstimateText(t *testing.T) {
	ctx := context.Background()
	tk := NewTiktokenWithCL100KBase()

	t.Run("simple text", func(t *testing.T) {
		count, err := tk.EstimateText(ctx, "hello world")

		require.NoError(t, err)
		assert.Greater(t, count, 0)
		assert.LessOrEqual(t, count, 10) // Should be around 2 tokens
	})

	t.Run("empty text", func(t *testing.T) {
		count, err := tk.EstimateText(ctx, "")

		require.NoError(t, err)
		// Empty text still has MIME type tokens
		assert.Greater(t, count, 0)
	})

	t.Run("long text", func(t *testing.T) {
		longText := "This is a long sentence that will be tokenized into multiple tokens. " +
			"It contains many words and should result in a higher token count."

		count, err := tk.EstimateText(ctx, longText)

		require.NoError(t, err)
		assert.Greater(t, count, 10)
	})

	t.Run("unicode text", func(t *testing.T) {
		count, err := tk.EstimateText(ctx, "你好世界 Hello World")

		require.NoError(t, err)
		assert.Greater(t, count, 0)
	})

	t.Run("text with emojis", func(t *testing.T) {
		count, err := tk.EstimateText(ctx, "Hello 🌍 World 🎉")

		require.NoError(t, err)
		assert.Greater(t, count, 0)
	})

	t.Run("special characters", func(t *testing.T) {
		count, err := tk.EstimateText(ctx, "!@#$%^&*()_+-=[]{}|;':\",./<>?")

		require.NoError(t, err)
		assert.Greater(t, count, 0)
	})

	t.Run("multiline text", func(t *testing.T) {
		text := `Line 1
Line 2
Line 3`

		count, err := tk.EstimateText(ctx, text)

		require.NoError(t, err)
		assert.Greater(t, count, 0)
	})

	t.Run("code snippet", func(t *testing.T) {
		code := `func main() {
    fmt.Println("Hello, World!")
}`

		count, err := tk.EstimateText(ctx, code)

		require.NoError(t, err)
		assert.Greater(t, count, 0)
	})

	t.Run("JSON text", func(t *testing.T) {
		jsonText := `{"key": "value", "number": 42, "nested": {"inner": "data"}}`

		count, err := tk.EstimateText(ctx, jsonText)

		require.NoError(t, err)
		assert.Greater(t, count, 0)
	})

	t.Run("very long text", func(t *testing.T) {
		veryLongText := ""
		for i := 0; i < 1000; i++ {
			veryLongText += "This is a test sentence. "
		}

		count, err := tk.EstimateText(ctx, veryLongText)

		require.NoError(t, err)
		assert.Greater(t, count, 1000)
	})
}

// TestTiktoken_EstimateMedia tests media token estimation
func TestTiktoken_EstimateMedia(t *testing.T) {
	ctx := context.Background()
	tk := NewTiktokenWithCL100KBase()

	t.Run("nil media slice", func(t *testing.T) {
		count, err := tk.EstimateMedia(ctx)

		require.NoError(t, err)
		assert.Equal(t, 0, count)
	})

	t.Run("empty media slice", func(t *testing.T) {
		count, err := tk.EstimateMedia(ctx, []*media.Media{}...)

		require.NoError(t, err)
		assert.Equal(t, 0, count)
	})

	t.Run("single text media", func(t *testing.T) {
		mt, err := mime.New("text", "plain")
		require.NoError(t, err)

		m := &media.Media{
			Data:     "hello world",
			MimeType: mt,
		}

		count, err := tk.EstimateMedia(ctx, m)

		require.NoError(t, err)
		assert.Greater(t, count, 0)
	})

	t.Run("multiple media objects", func(t *testing.T) {
		mt, err := mime.New("text", "plain")
		require.NoError(t, err)

		m1 := &media.Media{
			Data:     "first media",
			MimeType: mt,
		}
		m2 := &media.Media{
			Data:     "second media",
			MimeType: mt,
		}

		count, err := tk.EstimateMedia(ctx, m1, m2)

		require.NoError(t, err)
		assert.Greater(t, count, 0)

		// Should be greater than single media
		singleCount, _ := tk.EstimateMedia(ctx, m1)
		assert.Greater(t, count, singleCount)
	})

	t.Run("media with string data", func(t *testing.T) {
		mt, err := mime.New("text", "plain")
		require.NoError(t, err)

		m := &media.Media{
			Data:     "string data",
			MimeType: mt,
		}

		count, err := tk.EstimateMedia(ctx, m)

		require.NoError(t, err)
		assert.Greater(t, count, 0)
	})

	t.Run("media with byte slice data", func(t *testing.T) {
		mt, err := mime.New("application", "octet-stream")
		require.NoError(t, err)

		m := &media.Media{
			Data:     []byte("byte data"),
			MimeType: mt,
		}

		count, err := tk.EstimateMedia(ctx, m)

		require.NoError(t, err)
		assert.Greater(t, count, 0)
	})

	t.Run("media with struct data (JSON marshaling)", func(t *testing.T) {
		mt, err := mime.New("application", "json")
		require.NoError(t, err)

		type TestStruct struct {
			Name  string `json:"name"`
			Value int    `json:"value"`
		}

		m := &media.Media{
			Data: TestStruct{
				Name:  "test",
				Value: 42,
			},
			MimeType: mt,
		}

		count, err := tk.EstimateMedia(ctx, m)

		require.NoError(t, err)
		assert.Greater(t, count, 0)
	})

	t.Run("media with map data (JSON marshaling)", func(t *testing.T) {
		mt, err := mime.New("application", "json")
		require.NoError(t, err)

		m := &media.Media{
			Data: map[string]interface{}{
				"key1": "value1",
				"key2": 123,
				"key3": true,
			},
			MimeType: mt,
		}

		count, err := tk.EstimateMedia(ctx, m)

		require.NoError(t, err)
		assert.Greater(t, count, 0)
	})

	t.Run("media with slice data (JSON marshaling)", func(t *testing.T) {
		mt, err := mime.New("application", "json")
		require.NoError(t, err)

		m := &media.Media{
			Data:     []string{"item1", "item2", "item3"},
			MimeType: mt,
		}

		count, err := tk.EstimateMedia(ctx, m)

		require.NoError(t, err)
		assert.Greater(t, count, 0)
	})

	t.Run("media with nil data", func(t *testing.T) {
		mt, err := mime.New("text", "plain")
		require.NoError(t, err)

		m := &media.Media{
			Data:     nil,
			MimeType: mt,
		}

		count, err := tk.EstimateMedia(ctx, m)

		require.NoError(t, err)
		// Should only count MIME type tokens
		assert.Greater(t, count, 0)
	})

	t.Run("nil media object in slice", func(t *testing.T) {
		mt, err := mime.New("text", "plain")
		require.NoError(t, err)

		m := &media.Media{
			Data:     "valid media",
			MimeType: mt,
		}

		count, err := tk.EstimateMedia(ctx, m, nil)

		require.NoError(t, err)
		// Should handle nil gracefully
		assert.Greater(t, count, 0)
	})

	t.Run("different MIME types", func(t *testing.T) {
		mimeTypes := []struct {
			typeStr    string
			subtypeStr string
		}{
			{"text", "plain"},
			{"text", "html"},
			{"application", "json"},
			{"image", "png"},
			{"audio", "mpeg"},
			{"video", "mp4"},
		}

		for _, mt := range mimeTypes {
			t.Run(mt.typeStr+"/"+mt.subtypeStr, func(t *testing.T) {
				mimeType, err := mime.New(mt.typeStr, mt.subtypeStr)
				require.NoError(t, err)

				m := &media.Media{
					Data:     "test data",
					MimeType: mimeType,
				}

				count, err := tk.EstimateMedia(ctx, m)

				require.NoError(t, err)
				assert.Greater(t, count, 0)
			})
		}
	})
}

// TestTiktoken_estimateMedia tests the internal estimation method
func TestTiktoken_estimateMedia(t *testing.T) {
	ctx := context.Background()
	tk := NewTiktokenWithCL100KBase()

	t.Run("nil media", func(t *testing.T) {
		count, err := tk.estimateMedia(ctx, nil)

		require.NoError(t, err)
		assert.Equal(t, 0, count)
	})

	t.Run("includes MIME type tokens", func(t *testing.T) {
		mt, err := mime.New("text", "plain")
		require.NoError(t, err)

		m := &media.Media{
			Data:     "",
			MimeType: mt,
		}

		count, err := tk.estimateMedia(ctx, m)

		require.NoError(t, err)
		// Should have tokens for "text/plain"
		assert.Greater(t, count, 0)
	})

	t.Run("unmarshalable data type", func(t *testing.T) {
		mt, err := mime.New("application", "json")
		require.NoError(t, err)

		// Create a type that cannot be JSON marshaled (channel)
		m := &media.Media{
			Data:     make(chan int),
			MimeType: mt,
		}

		count, err := tk.estimateMedia(ctx, m)

		// Should not return error, just count MIME type tokens
		require.NoError(t, err)
		assert.Greater(t, count, 0)
	})
}

// TestTiktoken_Encode tests token encoding
func TestTiktoken_Encode(t *testing.T) {
	ctx := context.Background()
	tk := NewTiktokenWithCL100KBase()

	t.Run("simple text", func(t *testing.T) {
		tokens, err := tk.Encode(ctx, "hello world")

		require.NoError(t, err)
		assert.Greater(t, len(tokens), 0)
		assert.LessOrEqual(t, len(tokens), 10)

		// All tokens should be valid integers
		for _, token := range tokens {
			assert.GreaterOrEqual(t, token, 0)
		}
	})

	t.Run("empty text", func(t *testing.T) {
		tokens, err := tk.Encode(ctx, "")

		require.NoError(t, err)
		assert.Empty(t, tokens)
	})

	t.Run("single word", func(t *testing.T) {
		tokens, err := tk.Encode(ctx, "hello")

		require.NoError(t, err)
		assert.Greater(t, len(tokens), 0)
	})

	t.Run("long text", func(t *testing.T) {
		longText := "This is a long sentence with many words that will be tokenized."

		tokens, err := tk.Encode(ctx, longText)

		require.NoError(t, err)
		assert.Greater(t, len(tokens), 5)
	})

	t.Run("unicode text", func(t *testing.T) {
		tokens, err := tk.Encode(ctx, "你好世界")

		require.NoError(t, err)
		assert.Greater(t, len(tokens), 0)
	})

	t.Run("special characters", func(t *testing.T) {
		tokens, err := tk.Encode(ctx, "!@#$%^&*()")

		require.NoError(t, err)
		assert.Greater(t, len(tokens), 0)
	})

	t.Run("emojis", func(t *testing.T) {
		tokens, err := tk.Encode(ctx, "😀🎉🌍")

		require.NoError(t, err)
		assert.Greater(t, len(tokens), 0)
	})

	t.Run("consistent encoding", func(t *testing.T) {
		text := "consistent test"

		tokens1, err1 := tk.Encode(ctx, text)
		tokens2, err2 := tk.Encode(ctx, text)

		require.NoError(t, err1)
		require.NoError(t, err2)
		assert.Equal(t, tokens1, tokens2)
	})
}

// TestTiktoken_Decode tests token decoding
func TestTiktoken_Decode(t *testing.T) {
	ctx := context.Background()
	tk := NewTiktokenWithCL100KBase()

	t.Run("simple tokens", func(t *testing.T) {
		// First encode to get valid tokens
		originalText := "hello world"
		tokens, err := tk.Encode(ctx, originalText)
		require.NoError(t, err)

		// Then decode
		decodedText, err := tk.Decode(ctx, tokens)

		require.NoError(t, err)
		assert.Equal(t, originalText, decodedText)
	})

	t.Run("empty token slice", func(t *testing.T) {
		text, err := tk.Decode(ctx, []int{})

		require.NoError(t, err)
		assert.Empty(t, text)
	})

	t.Run("nil token slice", func(t *testing.T) {
		text, err := tk.Decode(ctx, nil)

		require.NoError(t, err)
		assert.Empty(t, text)
	})

	t.Run("round trip encoding and decoding", func(t *testing.T) {
		originalTexts := []string{
			"hello world",
			"This is a test",
			"GPT-4 is amazing",
			"Special chars: !@#$",
		}

		for _, original := range originalTexts {
			tokens, err := tk.Encode(ctx, original)
			require.NoError(t, err)

			decoded, err := tk.Decode(ctx, tokens)
			require.NoError(t, err)

			assert.Equal(t, original, decoded, "Round trip failed for: "+original)
		}
	})

	t.Run("unicode round trip", func(t *testing.T) {
		original := "你好世界 Hello"
		tokens, err := tk.Encode(ctx, original)
		require.NoError(t, err)

		decoded, err := tk.Decode(ctx, tokens)
		require.NoError(t, err)

		assert.Equal(t, original, decoded)
	})

	t.Run("emoji round trip", func(t *testing.T) {
		original := "Hello 🌍 World 🎉"
		tokens, err := tk.Encode(ctx, original)
		require.NoError(t, err)

		decoded, err := tk.Decode(ctx, tokens)
		require.NoError(t, err)

		assert.Equal(t, original, decoded)
	})
}

// TestTiktoken_InterfaceCompliance verifies interface implementations
func TestTiktoken_InterfaceCompliance(t *testing.T) {
	tk := NewTiktokenWithCL100KBase()

	t.Run("implements Estimator", func(t *testing.T) {
		var _ Estimator = tk
	})

	t.Run("implements TextEstimator", func(t *testing.T) {
		var _ TextEstimator = tk
	})

	t.Run("implements MediaEstimator", func(t *testing.T) {
		var _ MediaEstimator = tk
	})

	t.Run("implements Tokenizer", func(t *testing.T) {
		var _ Tokenizer = tk
	})

	t.Run("implements Encoder", func(t *testing.T) {
		var _ Encoder = tk
	})

	t.Run("implements Decoder", func(t *testing.T) {
		var _ Decoder = tk
	})
}

// TestTiktoken_ContextHandling tests context behavior
func TestTiktoken_ContextHandling(t *testing.T) {
	tk := NewTiktokenWithCL100KBase()

	t.Run("nil context", func(t *testing.T) {
		// Should not panic with nil context
		count, err := tk.EstimateText(nil, "test")
		require.NoError(t, err)
		assert.Greater(t, count, 0)

		tokens, err := tk.Encode(nil, "test")
		require.NoError(t, err)
		assert.Greater(t, len(tokens), 0)

		text, err := tk.Decode(nil, tokens)
		require.NoError(t, err)
		assert.NotEmpty(t, text)
	})

	t.Run("canceled context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		// Context cancellation is not currently enforced, but should not cause errors
		_, err := tk.EstimateText(ctx, "test")
		assert.NoError(t, err)
	})
}

// TestTiktoken_EdgeCases tests edge cases
func TestTiktoken_EdgeCases(t *testing.T) {
	ctx := context.Background()
	tk := NewTiktokenWithCL100KBase()

	t.Run("very long text estimation", func(t *testing.T) {
		veryLongText := ""
		for i := 0; i < 100000; i++ {
			veryLongText += "a"
		}

		count, err := tk.EstimateText(ctx, veryLongText)

		require.NoError(t, err)
		assert.Greater(t, count, 1000)
	})

	t.Run("text with null bytes", func(t *testing.T) {
		text := "hello\x00world"

		count, err := tk.EstimateText(ctx, text)

		require.NoError(t, err)
		assert.Greater(t, count, 0)
	})

	t.Run("repeated encoding", func(t *testing.T) {
		text := "test"

		for i := 0; i < 100; i++ {
			tokens, err := tk.Encode(ctx, text)
			require.NoError(t, err)
			assert.Greater(t, len(tokens), 0)
		}
	})

	t.Run("large token array decoding", func(t *testing.T) {
		// Create a large token array
		tokens := make([]int, 10000)
		for i := range tokens {
			tokens[i] = 100 + (i % 1000) // Valid token IDs
		}

		_, err := tk.Decode(ctx, tokens)
		require.NoError(t, err)
	})
}

// TestTiktoken_Comparison tests comparison between different encodings
func TestTiktoken_Comparison(t *testing.T) {
	ctx := context.Background()

	t.Run("different encodings produce different results", func(t *testing.T) {
		tk1, err := NewTiktoken("cl100k_base")
		require.NoError(t, err)

		tk2, err := NewTiktoken("o200k_base")
		require.NoError(t, err)

		text := "hello world"

		tokens1, _ := tk1.Encode(ctx, text)
		tokens2, _ := tk2.Encode(ctx, text)

		// Different encodings may produce different token sequences
		// (though they might occasionally be the same for simple text)
		assert.NotNil(t, tokens1)
		assert.NotNil(t, tokens2)
	})
}

// BenchmarkTiktoken benchmarks performance
func BenchmarkTiktoken_EstimateText(b *testing.B) {
	ctx := context.Background()
	tk := NewTiktokenWithCL100KBase()
	text := "This is a test sentence for benchmarking."

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = tk.EstimateText(ctx, text)
	}
}

func BenchmarkTiktoken_EstimateTextLong(b *testing.B) {
	ctx := context.Background()
	tk := NewTiktokenWithCL100KBase()
	text := ""
	for i := 0; i < 1000; i++ {
		text += "This is a test sentence. "
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = tk.EstimateText(ctx, text)
	}
}

func BenchmarkTiktoken_EstimateMedia(b *testing.B) {
	ctx := context.Background()
	tk := NewTiktokenWithCL100KBase()
	mt, _ := mime.New("text", "plain")
	m := &media.Media{
		Data:     "test data",
		MimeType: mt,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = tk.EstimateMedia(ctx, m)
	}
}

func BenchmarkTiktoken_Encode(b *testing.B) {
	ctx := context.Background()
	tk := NewTiktokenWithCL100KBase()
	text := "This is a test sentence for benchmarking."

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = tk.Encode(ctx, text)
	}
}

func BenchmarkTiktoken_Decode(b *testing.B) {
	ctx := context.Background()
	tk := NewTiktokenWithCL100KBase()
	text := "This is a test sentence for benchmarking."
	tokens, _ := tk.Encode(ctx, text)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = tk.Decode(ctx, tokens)
	}
}

func BenchmarkTiktoken_RoundTrip(b *testing.B) {
	ctx := context.Background()
	tk := NewTiktokenWithCL100KBase()
	text := "This is a test sentence for benchmarking."

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tokens, _ := tk.Encode(ctx, text)
		_, _ = tk.Decode(ctx, tokens)
	}
}

func BenchmarkTiktoken_EstimateMediaJSON(b *testing.B) {
	ctx := context.Background()
	tk := NewTiktokenWithCL100KBase()
	mt, _ := mime.New("application", "json")

	type TestData struct {
		Name  string   `json:"name"`
		Value int      `json:"value"`
		Items []string `json:"items"`
	}

	m := &media.Media{
		Data: TestData{
			Name:  "test",
			Value: 42,
			Items: []string{"a", "b", "c"},
		},
		MimeType: mt,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = tk.EstimateMedia(ctx, m)
	}
}

package batchers

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Tangerg/lynx/ai/media/document"
	"github.com/Tangerg/lynx/ai/tokenizer"
)

// mockFormatter for testing
type mockFormatter struct {
	prefix string
}

func (m *mockFormatter) Format(doc *document.Document, mode document.MetadataMode) string {
	if m.prefix != "" {
		return m.prefix + doc.Text
	}
	return doc.Text
}

// TestTokenCountBatcherConfig_validate tests config validation
func TestTokenCountBatcherConfig_validate(t *testing.T) {
	t.Run("nil config", func(t *testing.T) {
		var config *TokenCountBatcherConfig
		err := config.validate()

		require.Error(t, err)
		assert.Equal(t, "config is required", err.Error())
	})

	t.Run("nil token count estimator", func(t *testing.T) {
		config := &TokenCountBatcherConfig{
			TokenCountEstimator: nil,
			Formatter:           &mockFormatter{},
			MetadataMode:        document.MetadataModeAll,
		}

		err := config.validate()

		require.Error(t, err)
		assert.Equal(t, "token count estimator is required", err.Error())
	})

	t.Run("nil formatter", func(t *testing.T) {
		config := &TokenCountBatcherConfig{
			TokenCountEstimator: tokenizer.NewTiktokenWithCL100KBase(),
			Formatter:           nil,
			MetadataMode:        document.MetadataModeAll,
		}

		err := config.validate()

		require.Error(t, err)
		assert.Equal(t, "formatter is required", err.Error())
	})

	t.Run("negative max input token count", func(t *testing.T) {
		config := &TokenCountBatcherConfig{
			TokenCountEstimator: tokenizer.NewTiktokenWithCL100KBase(),
			MaxInputTokenCount:  -1,
			Formatter:           &mockFormatter{},
			MetadataMode:        document.MetadataModeAll,
		}

		err := config.validate()

		require.Error(t, err)
		assert.Equal(t, "max input token count must be greater than 0", err.Error())
	})

	t.Run("reserve percentage below 0", func(t *testing.T) {
		config := &TokenCountBatcherConfig{
			TokenCountEstimator: tokenizer.NewTiktokenWithCL100KBase(),
			MaxInputTokenCount:  1000,
			ReservePercentage:   -0.1,
			Formatter:           &mockFormatter{},
			MetadataMode:        document.MetadataModeAll,
		}

		err := config.validate()

		require.Error(t, err)
		assert.Equal(t, "reserve percentage must be in range [0, 1)", err.Error())
	})

	t.Run("reserve percentage equals 1", func(t *testing.T) {
		config := &TokenCountBatcherConfig{
			TokenCountEstimator: tokenizer.NewTiktokenWithCL100KBase(),
			MaxInputTokenCount:  1000,
			ReservePercentage:   1.0,
			Formatter:           &mockFormatter{},
			MetadataMode:        document.MetadataModeAll,
		}

		err := config.validate()

		require.Error(t, err)
		assert.Equal(t, "reserve percentage must be in range [0, 1)", err.Error())
	})

	t.Run("reserve percentage above 1", func(t *testing.T) {
		config := &TokenCountBatcherConfig{
			TokenCountEstimator: tokenizer.NewTiktokenWithCL100KBase(),
			MaxInputTokenCount:  1000,
			ReservePercentage:   1.5,
			Formatter:           &mockFormatter{},
			MetadataMode:        document.MetadataModeAll,
		}

		err := config.validate()

		require.Error(t, err)
		assert.Equal(t, "reserve percentage must be in range [0, 1)", err.Error())
	})

	t.Run("invalid metadata mode", func(t *testing.T) {
		config := &TokenCountBatcherConfig{
			TokenCountEstimator: tokenizer.NewTiktokenWithCL100KBase(),
			MaxInputTokenCount:  1000,
			ReservePercentage:   0.1,
			Formatter:           &mockFormatter{},
			MetadataMode:        document.MetadataMode("abc"),
		}

		err := config.validate()

		require.Error(t, err)
		assert.Equal(t, "invalid metadata mode", err.Error())
	})

	t.Run("valid config with all fields", func(t *testing.T) {
		config := &TokenCountBatcherConfig{
			TokenCountEstimator: tokenizer.NewTiktokenWithCL100KBase(),
			MaxInputTokenCount:  1000,
			ReservePercentage:   0.2,
			Formatter:           &mockFormatter{},
			MetadataMode:        document.MetadataModeAll,
		}

		err := config.validate()

		require.NoError(t, err)
		assert.Equal(t, 1000, config.MaxInputTokenCount)
		assert.Equal(t, 0.2, config.ReservePercentage)
	})

	t.Run("applies default max input token count", func(t *testing.T) {
		config := &TokenCountBatcherConfig{
			TokenCountEstimator: tokenizer.NewTiktokenWithCL100KBase(),
			MaxInputTokenCount:  0,
			Formatter:           &mockFormatter{},
			MetadataMode:        document.MetadataModeAll,
		}

		err := config.validate()

		require.NoError(t, err)
		assert.Equal(t, 8191, config.MaxInputTokenCount)
	})

	t.Run("applies default reserve percentage", func(t *testing.T) {
		config := &TokenCountBatcherConfig{
			TokenCountEstimator: tokenizer.NewTiktokenWithCL100KBase(),
			MaxInputTokenCount:  1000,
			ReservePercentage:   0,
			Formatter:           &mockFormatter{},
			MetadataMode:        document.MetadataModeAll,
		}

		err := config.validate()

		require.NoError(t, err)
		assert.Equal(t, 0.1, config.ReservePercentage)
	})

	t.Run("valid metadata modes", func(t *testing.T) {
		modes := []document.MetadataMode{
			document.MetadataModeAll,
			document.MetadataModeEmbed,
			document.MetadataModeInference,
			document.MetadataModeNone,
		}

		for _, mode := range modes {
			t.Run(mode.String(), func(t *testing.T) {
				config := &TokenCountBatcherConfig{
					TokenCountEstimator: tokenizer.NewTiktokenWithCL100KBase(),
					MaxInputTokenCount:  1000,
					ReservePercentage:   0.1,
					Formatter:           &mockFormatter{},
					MetadataMode:        mode,
				}

				err := config.validate()

				require.NoError(t, err)
			})
		}
	})

	t.Run("zero reserve percentage is valid", func(t *testing.T) {
		config := &TokenCountBatcherConfig{
			TokenCountEstimator: tokenizer.NewTiktokenWithCL100KBase(),
			MaxInputTokenCount:  1000,
			ReservePercentage:   0,
			Formatter:           &mockFormatter{},
			MetadataMode:        document.MetadataModeAll,
		}

		err := config.validate()

		require.NoError(t, err)
		assert.Equal(t, 0.1, config.ReservePercentage) // Should apply default
	})
}

// TestNewTokenCountBatcher tests the constructor
func TestNewTokenCountBatcher(t *testing.T) {
	t.Run("with nil config", func(t *testing.T) {
		batcher, err := NewTokenCountBatcher(nil)

		require.Error(t, err)
		assert.Nil(t, batcher)
		assert.Equal(t, "config is required", err.Error())
	})

	t.Run("with invalid config", func(t *testing.T) {
		config := &TokenCountBatcherConfig{
			TokenCountEstimator: nil,
		}

		batcher, err := NewTokenCountBatcher(config)

		require.Error(t, err)
		assert.Nil(t, batcher)
	})

	t.Run("with valid config", func(t *testing.T) {
		config := &TokenCountBatcherConfig{
			TokenCountEstimator: tokenizer.NewTiktokenWithCL100KBase(),
			MaxInputTokenCount:  1000,
			ReservePercentage:   0.1,
			Formatter:           &mockFormatter{},
			MetadataMode:        document.MetadataModeAll,
		}

		batcher, err := NewTokenCountBatcher(config)

		require.NoError(t, err)
		require.NotNil(t, batcher)
		// Actual max tokens = 1000 * (1 - 0.1) = 900
		assert.Equal(t, 900, batcher.maxInputTokenCount)
		assert.NotNil(t, batcher.tokenCountEstimator)
		assert.NotNil(t, batcher.formatter)
		assert.Equal(t, document.MetadataModeAll, batcher.metadataMode)
	})

	t.Run("calculates actual max tokens correctly", func(t *testing.T) {
		testCases := []struct {
			name              string
			maxTokens         int
			reservePercentage float64
			expectedActual    int
		}{
			{"10% reserve of 1000", 1000, 0.1, 900},
			{"20% reserve of 1000", 1000, 0.2, 800},
			{"5% reserve of 500", 500, 0.05, 475},
			{"15% reserve of 8191", 8191, 0.15, 6962},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				config := &TokenCountBatcherConfig{
					TokenCountEstimator: tokenizer.NewTiktokenWithCL100KBase(),
					MaxInputTokenCount:  tc.maxTokens,
					ReservePercentage:   tc.reservePercentage,
					Formatter:           &mockFormatter{},
					MetadataMode:        document.MetadataModeAll,
				}

				batcher, err := NewTokenCountBatcher(config)

				require.NoError(t, err)
				assert.Equal(t, tc.expectedActual, batcher.maxInputTokenCount)
			})
		}
	})

	t.Run("with default values", func(t *testing.T) {
		config := &TokenCountBatcherConfig{
			TokenCountEstimator: tokenizer.NewTiktokenWithCL100KBase(),
			Formatter:           &mockFormatter{},
			MetadataMode:        document.MetadataModeAll,
		}

		batcher, err := NewTokenCountBatcher(config)

		require.NoError(t, err)
		// Default: 8191 * (1 - 0.1) = 7372 (rounded)
		assert.Equal(t, 7372, batcher.maxInputTokenCount)
	})

	t.Run("multiple instances are independent", func(t *testing.T) {
		config1 := &TokenCountBatcherConfig{
			TokenCountEstimator: tokenizer.NewTiktokenWithCL100KBase(),
			MaxInputTokenCount:  1000,
			ReservePercentage:   0.1,
			Formatter:           &mockFormatter{},
			MetadataMode:        document.MetadataModeAll,
		}
		config2 := &TokenCountBatcherConfig{
			TokenCountEstimator: tokenizer.NewTiktokenWithCL100KBase(),
			MaxInputTokenCount:  2000,
			ReservePercentage:   0.2,
			Formatter:           &mockFormatter{},
			MetadataMode:        document.MetadataModeEmbed,
		}

		batcher1, err1 := NewTokenCountBatcher(config1)
		batcher2, err2 := NewTokenCountBatcher(config2)

		require.NoError(t, err1)
		require.NoError(t, err2)
		assert.NotSame(t, batcher1, batcher2)
		assert.Equal(t, 900, batcher1.maxInputTokenCount)
		assert.Equal(t, 1600, batcher2.maxInputTokenCount)
		assert.Equal(t, document.MetadataModeAll, batcher1.metadataMode)
		assert.Equal(t, document.MetadataModeEmbed, batcher2.metadataMode)
	})
}

// TestTokenCountBatcher_Batch tests the batching logic
func TestTokenCountBatcher_Batch(t *testing.T) {
	ctx := context.Background()

	t.Run("empty document list", func(t *testing.T) {
		config := &TokenCountBatcherConfig{
			TokenCountEstimator: tokenizer.NewTiktokenWithCL100KBase(),
			MaxInputTokenCount:  1000,
			ReservePercentage:   0.1,
			Formatter:           &mockFormatter{},
			MetadataMode:        document.MetadataModeAll,
		}
		batcher, err := NewTokenCountBatcher(config)
		require.NoError(t, err)

		batches, err := batcher.Batch(ctx, []*document.Document{})

		require.NoError(t, err)
		assert.Empty(t, batches)
	})

	t.Run("nil document list", func(t *testing.T) {
		config := &TokenCountBatcherConfig{
			TokenCountEstimator: tokenizer.NewTiktokenWithCL100KBase(),
			MaxInputTokenCount:  1000,
			ReservePercentage:   0.1,
			Formatter:           &mockFormatter{},
			MetadataMode:        document.MetadataModeAll,
		}
		batcher, err := NewTokenCountBatcher(config)
		require.NoError(t, err)

		batches, err := batcher.Batch(ctx, nil)

		require.NoError(t, err)
		assert.Empty(t, batches)
	})

	t.Run("single small document", func(t *testing.T) {
		config := &TokenCountBatcherConfig{
			TokenCountEstimator: tokenizer.NewTiktokenWithCL100KBase(),
			MaxInputTokenCount:  1000,
			ReservePercentage:   0.1,
			Formatter:           &mockFormatter{},
			MetadataMode:        document.MetadataModeAll,
		}
		batcher, err := NewTokenCountBatcher(config)
		require.NoError(t, err)

		doc, _ := document.NewDocument("This is a short document.", nil)

		batches, err := batcher.Batch(ctx, []*document.Document{doc})

		require.NoError(t, err)
		require.Len(t, batches, 1)
		assert.Len(t, batches[0], 1)
		assert.Equal(t, "This is a short document.", batches[0][0].Text)
	})

	t.Run("multiple small documents in single batch", func(t *testing.T) {
		config := &TokenCountBatcherConfig{
			TokenCountEstimator: tokenizer.NewTiktokenWithCL100KBase(),
			MaxInputTokenCount:  1000,
			ReservePercentage:   0.1,
			Formatter:           &mockFormatter{},
			MetadataMode:        document.MetadataModeAll,
		}
		batcher, err := NewTokenCountBatcher(config)
		require.NoError(t, err)

		doc1, _ := document.NewDocument("First document.", nil)
		doc2, _ := document.NewDocument("Second document.", nil)
		doc3, _ := document.NewDocument("Third document.", nil)

		batches, err := batcher.Batch(ctx, []*document.Document{doc1, doc2, doc3})

		require.NoError(t, err)
		require.Len(t, batches, 1)
		assert.Len(t, batches[0], 3)
	})

	t.Run("documents requiring multiple batches", func(t *testing.T) {
		config := &TokenCountBatcherConfig{
			TokenCountEstimator: tokenizer.NewTiktokenWithCL100KBase(),
			MaxInputTokenCount:  50,
			ReservePercentage:   0.1,
			Formatter:           &mockFormatter{},
			MetadataMode:        document.MetadataModeAll,
		}
		batcher, err := NewTokenCountBatcher(config)
		require.NoError(t, err)

		// Create documents that will exceed the token limit when combined
		docs := make([]*document.Document, 0)
		for i := 0; i < 5; i++ {
			doc, _ := document.NewDocument(strings.Repeat("word ", 20), nil)
			docs = append(docs, doc)
		}

		batches, err := batcher.Batch(ctx, docs)

		require.NoError(t, err)
		assert.Greater(t, len(batches), 1)

		// Verify all documents are included
		totalDocs := 0
		for _, batch := range batches {
			totalDocs += len(batch)
		}
		assert.Equal(t, 5, totalDocs)
	})

	t.Run("single document exceeding max tokens", func(t *testing.T) {
		config := &TokenCountBatcherConfig{
			TokenCountEstimator: tokenizer.NewTiktokenWithCL100KBase(),
			MaxInputTokenCount:  20,
			ReservePercentage:   0.1,
			Formatter:           &mockFormatter{},
			MetadataMode:        document.MetadataModeAll,
		}
		batcher, err := NewTokenCountBatcher(config)
		require.NoError(t, err)

		// Create a document with many tokens
		doc, _ := document.NewDocument(strings.Repeat("word ", 100), nil)

		batches, err := batcher.Batch(ctx, []*document.Document{doc})

		require.Error(t, err)
		assert.Nil(t, batches)
		assert.Contains(t, err.Error(), "tokens in a single document exceeds the maximum number of allowed input tokens")
	})

	t.Run("preserves document order", func(t *testing.T) {
		config := &TokenCountBatcherConfig{
			TokenCountEstimator: tokenizer.NewTiktokenWithCL100KBase(),
			MaxInputTokenCount:  1000,
			ReservePercentage:   0.1,
			Formatter:           &mockFormatter{},
			MetadataMode:        document.MetadataModeAll,
		}
		batcher, err := NewTokenCountBatcher(config)
		require.NoError(t, err)

		doc1, _ := document.NewDocument("First", nil)
		doc2, _ := document.NewDocument("Second", nil)
		doc3, _ := document.NewDocument("Third", nil)

		batches, err := batcher.Batch(ctx, []*document.Document{doc1, doc2, doc3})

		require.NoError(t, err)
		require.Len(t, batches, 1)
		assert.Equal(t, "First", batches[0][0].Text)
		assert.Equal(t, "Second", batches[0][1].Text)
		assert.Equal(t, "Third", batches[0][2].Text)
	})

	t.Run("with document metadata", func(t *testing.T) {
		config := &TokenCountBatcherConfig{
			TokenCountEstimator: tokenizer.NewTiktokenWithCL100KBase(),
			MaxInputTokenCount:  1000,
			ReservePercentage:   0.1,
			Formatter:           &mockFormatter{},
			MetadataMode:        document.MetadataModeAll,
		}
		batcher, err := NewTokenCountBatcher(config)
		require.NoError(t, err)

		doc, _ := document.NewDocument("Document with metadata", nil)
		doc.Metadata["source"] = "test"
		doc.Metadata["page"] = 1

		batches, err := batcher.Batch(ctx, []*document.Document{doc})

		require.NoError(t, err)
		require.Len(t, batches, 1)
		assert.Equal(t, "test", batches[0][0].Metadata["source"])
		assert.Equal(t, 1, batches[0][0].Metadata["page"])
	})

	t.Run("uses custom formatter", func(t *testing.T) {
		config := &TokenCountBatcherConfig{
			TokenCountEstimator: tokenizer.NewTiktokenWithCL100KBase(),
			MaxInputTokenCount:  1000,
			ReservePercentage:   0.1,
			Formatter:           &mockFormatter{prefix: "PREFIX: "},
			MetadataMode:        document.MetadataModeAll,
		}
		batcher, err := NewTokenCountBatcher(config)
		require.NoError(t, err)

		doc, _ := document.NewDocument("content", nil)

		batches, err := batcher.Batch(ctx, []*document.Document{doc})

		require.NoError(t, err)
		require.Len(t, batches, 1)
		// The formatter is used for token estimation
		// but doesn't modify the actual document
		assert.Equal(t, "content", batches[0][0].Text)
	})

	t.Run("different metadata modes", func(t *testing.T) {
		modes := []document.MetadataMode{
			document.MetadataModeAll,
			document.MetadataModeEmbed,
			document.MetadataModeInference,
			document.MetadataModeNone,
		}

		for _, mode := range modes {
			t.Run(mode.String(), func(t *testing.T) {
				config := &TokenCountBatcherConfig{
					TokenCountEstimator: tokenizer.NewTiktokenWithCL100KBase(),
					MaxInputTokenCount:  1000,
					ReservePercentage:   0.1,
					Formatter:           &mockFormatter{},
					MetadataMode:        mode,
				}
				batcher, err := NewTokenCountBatcher(config)
				require.NoError(t, err)

				doc, _ := document.NewDocument("Test content", nil)
				doc.Metadata["key"] = "value"

				batches, err := batcher.Batch(ctx, []*document.Document{doc})

				require.NoError(t, err)
				require.Len(t, batches, 1)
			})
		}
	})

	t.Run("exact boundary case", func(t *testing.T) {
		config := &TokenCountBatcherConfig{
			TokenCountEstimator: tokenizer.NewTiktokenWithCL100KBase(),
			MaxInputTokenCount:  30,
			ReservePercentage:   0.1,
			Formatter:           &mockFormatter{},
			MetadataMode:        document.MetadataModeAll,
		}
		batcher, err := NewTokenCountBatcher(config)
		require.NoError(t, err)

		// Create documents that exactly fill the batch
		doc1, _ := document.NewDocument("This is document one.", nil)
		doc2, _ := document.NewDocument("This is document two.", nil)

		batches, err := batcher.Batch(ctx, []*document.Document{doc1, doc2})

		require.NoError(t, err)
		assert.Greater(t, len(batches), 0)
	})

	t.Run("many small documents", func(t *testing.T) {
		config := &TokenCountBatcherConfig{
			TokenCountEstimator: tokenizer.NewTiktokenWithCL100KBase(),
			MaxInputTokenCount:  100,
			ReservePercentage:   0.1,
			Formatter:           &mockFormatter{},
			MetadataMode:        document.MetadataModeAll,
		}
		batcher, err := NewTokenCountBatcher(config)
		require.NoError(t, err)

		docs := make([]*document.Document, 50)
		for i := range docs {
			docs[i], _ = document.NewDocument("Small doc.", nil)
		}

		batches, err := batcher.Batch(ctx, docs)

		require.NoError(t, err)
		assert.Greater(t, len(batches), 0)

		// Verify all documents are included
		totalDocs := 0
		for _, batch := range batches {
			totalDocs += len(batch)
		}
		assert.Equal(t, 50, totalDocs)
	})
}

// TestTokenCountBatcher_RealWorldScenarios tests real-world use cases
func TestTokenCountBatcher_RealWorldScenarios(t *testing.T) {
	ctx := context.Background()

	t.Run("article paragraphs batching", func(t *testing.T) {
		config := &TokenCountBatcherConfig{
			TokenCountEstimator: tokenizer.NewTiktokenWithCL100KBase(),
			MaxInputTokenCount:  200,
			ReservePercentage:   0.1,
			Formatter:           &mockFormatter{},
			MetadataMode:        document.MetadataModeAll,
		}
		batcher, err := NewTokenCountBatcher(config)
		require.NoError(t, err)

		paragraphs := []string{
			"Artificial Intelligence has transformed modern technology. It enables machines to learn and adapt.",
			"Machine learning algorithms process vast amounts of data. They identify patterns and make predictions.",
			"Deep learning uses neural networks with multiple layers. This approach has revolutionized AI capabilities.",
			"Natural language processing allows computers to understand human language. It powers chatbots and translation services.",
			"Computer vision enables machines to interpret visual information. Applications include facial recognition and autonomous vehicles.",
		}

		docs := make([]*document.Document, 0, len(paragraphs))
		for i, p := range paragraphs {
			doc, _ := document.NewDocument(p, nil)
			doc.Metadata["paragraph"] = i + 1
			doc.Metadata["source"] = "AI Article"
			docs = append(docs, doc)
		}

		batches, err := batcher.Batch(ctx, docs)

		require.NoError(t, err)
		assert.Greater(t, len(batches), 0)

		for i, batch := range batches {
			t.Logf("Batch %d: %d documents", i, len(batch))
			for j, doc := range batch {
				t.Logf("  Doc %d: %s", j, doc.Text[:50]+"...")
			}
		}
	})

	t.Run("chat message batching", func(t *testing.T) {
		config := &TokenCountBatcherConfig{
			TokenCountEstimator: tokenizer.NewTiktokenWithCL100KBase(),
			MaxInputTokenCount:  150,
			ReservePercentage:   0.1,
			Formatter:           &mockFormatter{},
			MetadataMode:        document.MetadataModeAll,
		}
		batcher, err := NewTokenCountBatcher(config)
		require.NoError(t, err)

		messages := []struct {
			role    string
			content string
		}{
			{"user", "What is machine learning?"},
			{"assistant", "Machine learning is a subset of AI that enables systems to learn from data."},
			{"user", "Can you give me an example?"},
			{"assistant", "Sure! Email spam filters use machine learning to identify spam messages."},
			{"user", "That's interesting. Tell me more."},
			{"assistant", "Machine learning models improve over time as they process more examples."},
		}

		docs := make([]*document.Document, 0, len(messages))
		for _, msg := range messages {
			doc, _ := document.NewDocument(msg.content, nil)
			doc.Metadata["role"] = msg.role
			docs = append(docs, doc)
		}

		batches, err := batcher.Batch(ctx, docs)

		require.NoError(t, err)
		assert.Greater(t, len(batches), 0)
	})

	t.Run("code snippets batching", func(t *testing.T) {
		config := &TokenCountBatcherConfig{
			TokenCountEstimator: tokenizer.NewTiktokenWithCL100KBase(),
			MaxInputTokenCount:  300,
			ReservePercentage:   0.1,
			Formatter:           &mockFormatter{},
			MetadataMode:        document.MetadataModeAll,
		}
		batcher, err := NewTokenCountBatcher(config)
		require.NoError(t, err)

		snippets := []string{
			`func main() {
    fmt.Println("Hello, World!")
}`,
			`type User struct {
    ID   int
    Name string
}`,
			`func (u *User) GetName() string {
    return u.Name
}`,
		}

		docs := make([]*document.Document, 0, len(snippets))
		for i, snippet := range snippets {
			doc, _ := document.NewDocument(snippet, nil)
			doc.Metadata["snippet_id"] = i + 1
			doc.Metadata["language"] = "go"
			docs = append(docs, doc)
		}

		batches, err := batcher.Batch(ctx, docs)

		require.NoError(t, err)
		assert.Greater(t, len(batches), 0)
	})

	t.Run("mixed size documents", func(t *testing.T) {
		config := &TokenCountBatcherConfig{
			TokenCountEstimator: tokenizer.NewTiktokenWithCL100KBase(),
			MaxInputTokenCount:  100,
			ReservePercentage:   0.1,
			Formatter:           &mockFormatter{},
			MetadataMode:        document.MetadataModeAll,
		}
		batcher, err := NewTokenCountBatcher(config)
		require.NoError(t, err)

		docs := []*document.Document{
			mustNewDocument("Short."),
			mustNewDocument(strings.Repeat("Medium length document. ", 10)),
			mustNewDocument("Tiny."),
			mustNewDocument(strings.Repeat("Another medium document. ", 10)),
			mustNewDocument("Small."),
		}

		batches, err := batcher.Batch(ctx, docs)

		require.NoError(t, err)
		assert.Greater(t, len(batches), 0)

		for i, batch := range batches {
			t.Logf("Batch %d: %d documents", i, len(batch))
		}
	})
}

// TestTokenCountBatcher_EdgeCases tests edge cases
func TestTokenCountBatcher_EdgeCases(t *testing.T) {
	ctx := context.Background()

	t.Run("very small max tokens", func(t *testing.T) {
		config := &TokenCountBatcherConfig{
			TokenCountEstimator: tokenizer.NewTiktokenWithCL100KBase(),
			MaxInputTokenCount:  10,
			ReservePercentage:   0.1,
			Formatter:           &mockFormatter{},
			MetadataMode:        document.MetadataModeAll,
		}
		batcher, err := NewTokenCountBatcher(config)
		require.NoError(t, err)

		doc, _ := document.NewDocument("Short text.", nil)

		batches, err := batcher.Batch(ctx, []*document.Document{doc})

		// Might succeed or fail depending on actual token count
		if err == nil {
			assert.Greater(t, len(batches), 0)
		}
	})

	t.Run("zero reserve percentage", func(t *testing.T) {
		config := &TokenCountBatcherConfig{
			TokenCountEstimator: tokenizer.NewTiktokenWithCL100KBase(),
			MaxInputTokenCount:  100,
			ReservePercentage:   0.0, // will set default 0.1
			Formatter:           &mockFormatter{},
			MetadataMode:        document.MetadataModeAll,
		}
		batcher, err := NewTokenCountBatcher(config)
		require.NoError(t, err)

		// With 0% reserve, actual max = 90
		assert.Equal(t, 90, batcher.maxInputTokenCount)

		doc, _ := document.NewDocument("Test content.", nil)

		batches, err := batcher.Batch(ctx, []*document.Document{doc})

		require.NoError(t, err)
		assert.Greater(t, len(batches), 0)
	})

	t.Run("high reserve percentage", func(t *testing.T) {
		config := &TokenCountBatcherConfig{
			TokenCountEstimator: tokenizer.NewTiktokenWithCL100KBase(),
			MaxInputTokenCount:  1000,
			ReservePercentage:   0.5,
			Formatter:           &mockFormatter{},
			MetadataMode:        document.MetadataModeAll,
		}
		batcher, err := NewTokenCountBatcher(config)
		require.NoError(t, err)

		// With 50% reserve, actual max = 500
		assert.Equal(t, 500, batcher.maxInputTokenCount)
	})

	t.Run("document with only whitespace", func(t *testing.T) {
		config := &TokenCountBatcherConfig{
			TokenCountEstimator: tokenizer.NewTiktokenWithCL100KBase(),
			MaxInputTokenCount:  100,
			ReservePercentage:   0.1,
			Formatter:           &mockFormatter{},
			MetadataMode:        document.MetadataModeAll,
		}
		batcher, err := NewTokenCountBatcher(config)
		require.NoError(t, err)

		doc, _ := document.NewDocument("   \n\t   ", nil)

		batches, err := batcher.Batch(ctx, []*document.Document{doc})

		require.NoError(t, err)
		assert.Greater(t, len(batches), 0)
	})

	t.Run("unicode and emoji documents", func(t *testing.T) {
		config := &TokenCountBatcherConfig{
			TokenCountEstimator: tokenizer.NewTiktokenWithCL100KBase(),
			MaxInputTokenCount:  100,
			ReservePercentage:   0.1,
			Formatter:           &mockFormatter{},
			MetadataMode:        document.MetadataModeAll,
		}
		batcher, err := NewTokenCountBatcher(config)
		require.NoError(t, err)

		docs := []*document.Document{
			mustNewDocument("Hello 世界 🌍"),
			mustNewDocument("测试文本 Test text 🎉"),
			mustNewDocument("Emoji: 😀🎨🚀"),
		}

		batches, err := batcher.Batch(ctx, docs)

		require.NoError(t, err)
		assert.Greater(t, len(batches), 0)
	})

	t.Run("documents with special characters", func(t *testing.T) {
		config := &TokenCountBatcherConfig{
			TokenCountEstimator: tokenizer.NewTiktokenWithCL100KBase(),
			MaxInputTokenCount:  100,
			ReservePercentage:   0.1,
			Formatter:           &mockFormatter{},
			MetadataMode:        document.MetadataModeAll,
		}
		batcher, err := NewTokenCountBatcher(config)
		require.NoError(t, err)

		docs := []*document.Document{
			mustNewDocument("Code: var x = 10;"),
			mustNewDocument("Path: /usr/local/bin"),
			mustNewDocument("Email: test@example.com"),
		}

		batches, err := batcher.Batch(ctx, docs)

		require.NoError(t, err)
		assert.Greater(t, len(batches), 0)
	})
}

// TestTokenCountBatcher_InterfaceCompliance verifies interface implementation
func TestTokenCountBatcher_InterfaceCompliance(t *testing.T) {
	config := &TokenCountBatcherConfig{
		TokenCountEstimator: tokenizer.NewTiktokenWithCL100KBase(),
		MaxInputTokenCount:  100,
		ReservePercentage:   0.1,
		Formatter:           &mockFormatter{},
		MetadataMode:        document.MetadataModeAll,
	}
	batcher, err := NewTokenCountBatcher(config)
	require.NoError(t, err)

	var _ document.Batcher = batcher
}

// BenchmarkTokenCountBatcher benchmarks performance
func BenchmarkTokenCountBatcher_Batch_Small(b *testing.B) {
	ctx := context.Background()
	config := &TokenCountBatcherConfig{
		TokenCountEstimator: tokenizer.NewTiktokenWithCL100KBase(),
		MaxInputTokenCount:  1000,
		ReservePercentage:   0.1,
		Formatter:           &mockFormatter{},
		MetadataMode:        document.MetadataModeAll,
	}
	batcher, _ := NewTokenCountBatcher(config)

	docs := make([]*document.Document, 10)
	for i := range docs {
		docs[i] = mustNewDocument("This is a small test document.")
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = batcher.Batch(ctx, docs)
	}
}

func BenchmarkTokenCountBatcher_Batch_Medium(b *testing.B) {
	ctx := context.Background()
	config := &TokenCountBatcherConfig{
		TokenCountEstimator: tokenizer.NewTiktokenWithCL100KBase(),
		MaxInputTokenCount:  1000,
		ReservePercentage:   0.1,
		Formatter:           &mockFormatter{},
		MetadataMode:        document.MetadataModeAll,
	}
	batcher, _ := NewTokenCountBatcher(config)

	docs := make([]*document.Document, 50)
	for i := range docs {
		docs[i] = mustNewDocument(strings.Repeat("This is test content. ", 20))
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = batcher.Batch(ctx, docs)
	}
}

func BenchmarkTokenCountBatcher_Batch_Large(b *testing.B) {
	ctx := context.Background()
	config := &TokenCountBatcherConfig{
		TokenCountEstimator: tokenizer.NewTiktokenWithCL100KBase(),
		MaxInputTokenCount:  1000,
		ReservePercentage:   0.1,
		Formatter:           &mockFormatter{},
		MetadataMode:        document.MetadataModeAll,
	}
	batcher, _ := NewTokenCountBatcher(config)

	docs := make([]*document.Document, 100)
	for i := range docs {
		docs[i] = mustNewDocument(strings.Repeat("This is a large test document. ", 50))
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = batcher.Batch(ctx, docs)
	}
}

func BenchmarkTokenCountBatcher_Batch_MixedSizes(b *testing.B) {
	ctx := context.Background()
	config := &TokenCountBatcherConfig{
		TokenCountEstimator: tokenizer.NewTiktokenWithCL100KBase(),
		MaxInputTokenCount:  500,
		ReservePercentage:   0.1,
		Formatter:           &mockFormatter{},
		MetadataMode:        document.MetadataModeAll,
	}
	batcher, _ := NewTokenCountBatcher(config)

	docs := make([]*document.Document, 30)
	for i := range docs {
		if i%3 == 0 {
			docs[i] = mustNewDocument("Short.")
		} else if i%3 == 1 {
			docs[i] = mustNewDocument(strings.Repeat("Medium. ", 20))
		} else {
			docs[i] = mustNewDocument(strings.Repeat("Long document. ", 40))
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = batcher.Batch(ctx, docs)
	}
}

func BenchmarkTokenCountBatcher_Batch_WithMetadata(b *testing.B) {
	ctx := context.Background()
	config := &TokenCountBatcherConfig{
		TokenCountEstimator: tokenizer.NewTiktokenWithCL100KBase(),
		MaxInputTokenCount:  1000,
		ReservePercentage:   0.1,
		Formatter:           &mockFormatter{},
		MetadataMode:        document.MetadataModeAll,
	}
	batcher, _ := NewTokenCountBatcher(config)

	docs := make([]*document.Document, 50)
	for i := range docs {
		doc := mustNewDocument("Document with metadata.")
		doc.Metadata["id"] = i
		doc.Metadata["source"] = "benchmark"
		doc.Metadata["timestamp"] = "2024-01-01"
		docs[i] = doc
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = batcher.Batch(ctx, docs)
	}
}

// Helper function
func mustNewDocument(text string) *document.Document {
	doc, err := document.NewDocument(text, nil)
	if err != nil {
		panic(err)
	}
	return doc
}

package rag_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Tangerg/lynx/ai/media/document"
	"github.com/Tangerg/lynx/ai/model/chat"
	"github.com/Tangerg/lynx/ai/rag"
	"github.com/Tangerg/lynx/ai/rag/document/refiners"
	"github.com/Tangerg/lynx/ai/rag/document/retrievers"
	"github.com/Tangerg/lynx/ai/rag/query/augmenters"
	"github.com/Tangerg/lynx/ai/rag/query/expanders"
	"github.com/Tangerg/lynx/ai/rag/query/transformers"
)

// TestPipelineMiddleware_BasicCall tests basic middleware functionality with chat call
func TestPipelineMiddleware_BasicCall(t *testing.T) {
	config := newTestConfig()
	skipIfNoAPIKey(t, config)

	// Setup minimal pipeline
	vectorStoreRetriever, err := retrievers.NewVectorStoreRetriever(&retrievers.VectorStoreRetrieverConfig{
		VectorStore: NewMultilingualMockVectorStore(),
		TopK:        5,
	})
	require.NoError(t, err)

	callMiddleware, streamMiddleware, err := rag.NewPipelineMiddleware(&rag.PipelineConfig{
		DocumentRetrievers: []rag.DocumentRetriever{
			vectorStoreRetriever,
		},
	})
	require.NoError(t, err)
	assert.NotNil(t, callMiddleware)
	assert.NotNil(t, streamMiddleware)

	// Create chat client
	chatClient, err := chat.NewClientWithModel(newTestChatModel(t))
	require.NoError(t, err)

	// Create request
	request, err := chat.NewRequest([]chat.Message{
		chat.NewUserMessage("What is RAG?"),
	})
	require.NoError(t, err)

	// Execute with middleware
	ctx, cancel := context.WithTimeout(context.Background(), config.timeout)
	defer cancel()

	text, resp, err := chatClient.
		ChatRequest(request).
		WithMiddlewares(callMiddleware, streamMiddleware).
		Call().
		Text(ctx)

	require.NoError(t, err)
	assert.NotEmpty(t, text)

	// Verify documents are attached
	docs, ok := resp.Metadata.Get(rag.DocumentContextKey)
	assert.True(t, ok)
	assert.NotNil(t, docs)

	documents, ok := docs.([]*document.Document)
	assert.True(t, ok)
	assert.NotEmpty(t, documents)

	t.Logf("Response: %s", text)
	t.Logf("Retrieved %d documents", len(documents))
}

// TestPipelineMiddleware_FullPipeline tests middleware with complete RAG pipeline
func TestPipelineMiddleware_FullPipeline(t *testing.T) {
	config := newTestConfig()
	skipIfNoAPIKey(t, config)

	// Setup transformers
	compressionTransformer, err := transformers.NewCompressionTransformer(&transformers.CompressionTransformerConfig{
		ChatModel: newTestChatModel(t),
	})
	require.NoError(t, err)

	rewriteTransformer, err := transformers.NewRewriteTransformer(&transformers.RewriteTransformerConfig{
		ChatModel: newTestChatModel(t),
	})
	require.NoError(t, err)

	translationTransformer, err := transformers.NewTranslationTransformer(&transformers.TranslationTransformerConfig{
		ChatModel:      newTestChatModel(t),
		TargetLanguage: "English",
	})
	require.NoError(t, err)

	// Setup expander
	multiExpander, err := expanders.NewMultiExpander(&expanders.MultiExpanderConfig{
		ChatModel:       newTestChatModel(t),
		IncludeOriginal: true,
		NumberOfQueries: 3,
	})
	require.NoError(t, err)

	// Setup retriever
	vectorStoreRetriever, err := retrievers.NewVectorStoreRetriever(&retrievers.VectorStoreRetrieverConfig{
		VectorStore: NewMultilingualMockVectorStore(),
		TopK:        10,
	})
	require.NoError(t, err)

	// Setup augmenter
	augmenter, err := augmenters.NewContextualAugmenter(&augmenters.ContextualAugmenterConfig{})
	require.NoError(t, err)

	// Create middleware
	callMiddleware, streamMiddleware, err := rag.NewPipelineMiddleware(&rag.PipelineConfig{
		QueryTransformers: []rag.QueryTransformer{
			compressionTransformer,
			rewriteTransformer,
			translationTransformer,
		},
		QueryExpander: multiExpander,
		DocumentRetrievers: []rag.DocumentRetriever{
			vectorStoreRetriever,
		},
		DocumentRefiners: []rag.DocumentRefiner{
			refiners.NewDeduplicationRefiner(),
			refiners.NewRankRefiner(5),
		},
		QueryAugmenter: augmenter,
	})
	require.NoError(t, err)

	// Create chat client
	chatClient, err := chat.NewClientWithModel(newTestChatModel(t))
	require.NoError(t, err)

	// Create request with conversation history
	request, err := chat.NewRequest([]chat.Message{
		chat.NewUserMessage("什么是RAG?"),
		chat.NewAssistantMessage("RAG 是 Retrieval-Augmented Generation 的缩写，它是一种将信息检索与文本生成相结合的技术，以提供更加准确和上下文相关的响应。"),
		chat.NewUserMessage("那么它的主要组件是什么?"),
		chat.NewAssistantMessage("RAG架构由三个主要组件组成：查询处理、文档检索和响应生成。"),
		chat.NewUserMessage("展开说说"),
	})
	require.NoError(t, err)

	// Execute with middleware
	ctx, cancel := context.WithTimeout(context.Background(), config.timeout)
	defer cancel()

	text, resp, err := chatClient.
		ChatRequest(request).
		WithMiddlewares(callMiddleware, streamMiddleware).
		Call().
		Text(ctx)

	require.NoError(t, err)
	assert.NotEmpty(t, text)

	// Verify documents are attached
	docs, ok := resp.Metadata.Get(rag.DocumentContextKey)
	assert.True(t, ok)
	assert.NotNil(t, docs)

	documents, ok := docs.([]*document.Document)
	assert.True(t, ok)
	assert.NotEmpty(t, documents)
	assert.LessOrEqual(t, len(documents), 5, "should be limited by rank refiner")

	t.Log("=== Conversation ===")
	for _, msg := range request.Messages {
		t.Log(chat.MessageToString(msg))
	}
	t.Log("\n=== Response ===")
	t.Logf("Text: %s", text)
	t.Log("\n=== Retrieved Documents ===")
	for i, doc := range documents {
		lang, _ := doc.Metadata["language"]
		t.Logf("[%d] ID: %s, Language: %v, Score: %.4f", i+1, doc.ID, lang, doc.Score)
		t.Logf("    Text: %s", doc.Text)
	}
}

// TestPipelineMiddleware_MultilingualConversation tests middleware with multilingual conversations
func TestPipelineMiddleware_MultilingualConversation(t *testing.T) {
	config := newTestConfig()
	skipIfNoAPIKey(t, config)

	testCases := []struct {
		name         string
		conversation []chat.Message
		description  string
	}{
		{
			name: "Chinese conversation",
			conversation: []chat.Message{
				chat.NewUserMessage("什么是RAG系统?"),
				chat.NewAssistantMessage("RAG系统是检索增强生成系统，结合了信息检索和文本生成。"),
				chat.NewUserMessage("它有什么优势?"),
			},
			description: "Multi-turn Chinese conversation",
		},
		{
			name: "Japanese conversation",
			conversation: []chat.Message{
				chat.NewUserMessage("RAGシステムとは何ですか？"),
				chat.NewAssistantMessage("RAGシステムは検索拡張生成システムで、情報検索とテキスト生成を組み合わせたものです。"),
				chat.NewUserMessage("その利点は何ですか？"),
			},
			description: "Multi-turn Japanese conversation",
		},
		{
			name: "Korean conversation",
			conversation: []chat.Message{
				chat.NewUserMessage("RAG 시스템이란 무엇입니까?"),
				chat.NewAssistantMessage("RAG 시스템은 검색 증강 생성 시스템으로, 정보 검색과 텍스트 생성을 결합한 것입니다."),
				chat.NewUserMessage("그 장점은 무엇입니까?"),
			},
			description: "Multi-turn Korean conversation",
		},
		{
			name: "Mixed language conversation",
			conversation: []chat.Message{
				chat.NewUserMessage("What is RAG?"),
				chat.NewAssistantMessage("RAG stands for Retrieval-Augmented Generation."),
				chat.NewUserMessage("RAGシステムの主要コンポーネントは何ですか？"),
			},
			description: "Mixed English and Japanese conversation",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Setup pipeline with translation
			translationTransformer, err := transformers.NewTranslationTransformer(&transformers.TranslationTransformerConfig{
				ChatModel:      newTestChatModel(t),
				TargetLanguage: "English",
			})
			require.NoError(t, err)

			vectorStoreRetriever, err := retrievers.NewVectorStoreRetriever(&retrievers.VectorStoreRetrieverConfig{
				VectorStore: NewMultilingualMockVectorStore(),
				TopK:        5,
			})
			require.NoError(t, err)

			augmenter, err := augmenters.NewContextualAugmenter(&augmenters.ContextualAugmenterConfig{})
			require.NoError(t, err)

			callMiddleware, streamMiddleware, err := rag.NewPipelineMiddleware(&rag.PipelineConfig{
				QueryTransformers: []rag.QueryTransformer{
					translationTransformer,
				},
				DocumentRetrievers: []rag.DocumentRetriever{
					vectorStoreRetriever,
				},
				DocumentRefiners: []rag.DocumentRefiner{
					refiners.NewRankRefiner(3),
				},
				QueryAugmenter: augmenter,
			})
			require.NoError(t, err)

			// Create chat client
			chatClient, err := chat.NewClientWithModel(newTestChatModel(t))
			require.NoError(t, err)

			// Create request
			request, err := chat.NewRequest(tc.conversation)
			require.NoError(t, err)

			// Execute
			ctx, cancel := context.WithTimeout(context.Background(), config.timeout)
			defer cancel()

			text, resp, err := chatClient.
				ChatRequest(request).
				WithMiddlewares(callMiddleware, streamMiddleware).
				Call().
				Text(ctx)

			require.NoError(t, err)
			assert.NotEmpty(t, text)

			// Verify documents
			docs, ok := resp.Metadata.Get(rag.DocumentContextKey)
			assert.True(t, ok)

			documents, ok := docs.([]*document.Document)
			assert.True(t, ok)
			assert.NotEmpty(t, documents)

			t.Logf("Description: %s", tc.description)
			t.Logf("Response: %s", text)
			t.Logf("Documents: %d", len(documents))
		})
	}
}

// TestPipelineMiddleware_StreamResponse tests middleware with streaming responses
func TestPipelineMiddleware_StreamResponse(t *testing.T) {
	config := newTestConfig()
	skipIfNoAPIKey(t, config)

	// Setup pipeline
	vectorStoreRetriever, err := retrievers.NewVectorStoreRetriever(&retrievers.VectorStoreRetrieverConfig{
		VectorStore: NewMultilingualMockVectorStore(),
		TopK:        5,
	})
	require.NoError(t, err)

	augmenter, err := augmenters.NewContextualAugmenter(&augmenters.ContextualAugmenterConfig{})
	require.NoError(t, err)

	callMiddleware, streamMiddleware, err := rag.NewPipelineMiddleware(&rag.PipelineConfig{
		DocumentRetrievers: []rag.DocumentRetriever{
			vectorStoreRetriever,
		},
		QueryAugmenter: augmenter,
	})
	require.NoError(t, err)

	// Create chat client
	chatClient, err := chat.NewClientWithModel(newTestChatModel(t))
	require.NoError(t, err)

	// Create request
	request, err := chat.NewRequest([]chat.Message{
		chat.NewUserMessage("Explain RAG in detail"),
	})
	require.NoError(t, err)

	// Execute with streaming
	ctx, cancel := context.WithTimeout(context.Background(), config.timeout)
	defer cancel()

	stream := chatClient.
		ChatRequest(request).
		WithMiddlewares(callMiddleware, streamMiddleware).
		Stream().
		Response(ctx)

	var fullText string
	var lastResponse *chat.Response
	chunkCount := 0

	for response, err := range stream {
		require.NoError(t, err)
		assert.NotNil(t, response)

		// Verify documents are attached to each chunk
		docs, ok := response.Metadata.Get(rag.DocumentContextKey)
		assert.True(t, ok)
		assert.NotNil(t, docs)

		if response.Result() != nil && response.Result().AssistantMessage != nil {
			fullText += response.Result().AssistantMessage.Text
			chunkCount++
		}

		lastResponse = response
	}

	assert.NotEmpty(t, fullText)
	assert.Greater(t, chunkCount, 0)

	// Verify final documents
	docs, ok := lastResponse.Metadata.Get(rag.DocumentContextKey)
	assert.True(t, ok)
	documents, ok := docs.([]*document.Document)
	assert.True(t, ok)
	assert.NotEmpty(t, documents)

	t.Logf("Received %d chunks", chunkCount)
	t.Logf("Full text: %s", fullText)
	t.Logf("Retrieved %d documents", len(documents))
}

// TestPipelineMiddleware_WithRequestParams tests middleware with request parameters
func TestPipelineMiddleware_WithRequestParams(t *testing.T) {
	config := newTestConfig()
	skipIfNoAPIKey(t, config)

	vectorStoreRetriever, err := retrievers.NewVectorStoreRetriever(&retrievers.VectorStoreRetrieverConfig{
		VectorStore: NewMultilingualMockVectorStore(),
		TopK:        5,
	})
	require.NoError(t, err)

	callMiddleware, streamMiddleware, err := rag.NewPipelineMiddleware(&rag.PipelineConfig{
		DocumentRetrievers: []rag.DocumentRetriever{
			vectorStoreRetriever,
		},
	})
	require.NoError(t, err)

	chatClient, err := chat.NewClientWithModel(newTestChatModel(t))
	require.NoError(t, err)

	// Create request with custom parameters
	request, err := chat.NewRequest(
		[]chat.Message{
			chat.NewUserMessage("What is RAG?"),
		})

	require.NoError(t, err)
	request.Params = map[string]any{
		"user_id":   "test_user_123",
		"session":   "session_456",
		"timestamp": "2024-01-01T00:00:00Z",
	}

	ctx, cancel := context.WithTimeout(context.Background(), config.timeout)
	defer cancel()

	text, resp, err := chatClient.
		ChatRequest(request).
		WithMiddlewares(callMiddleware, streamMiddleware).
		Call().
		Text(ctx)

	require.NoError(t, err)
	assert.NotEmpty(t, text)

	docs, ok := resp.Metadata.Get(rag.DocumentContextKey)
	assert.True(t, ok)
	assert.NotNil(t, docs)

	t.Logf("Response with custom params: %s", text)
}

// TestPipelineMiddleware_ErrorHandling tests middleware error scenarios
func TestPipelineMiddleware_ErrorHandling(t *testing.T) {
	t.Run("invalid configuration", func(t *testing.T) {
		callMiddleware, streamMiddleware, err := rag.NewPipelineMiddleware(&rag.PipelineConfig{
			DocumentRetrievers: []rag.DocumentRetriever{},
		})

		assert.Error(t, err)
		assert.Nil(t, callMiddleware)
		assert.Nil(t, streamMiddleware)
	})

	t.Run("nil configuration", func(t *testing.T) {
		callMiddleware, streamMiddleware, err := rag.NewPipelineMiddleware(nil)

		assert.Error(t, err)
		assert.Nil(t, callMiddleware)
		assert.Nil(t, streamMiddleware)
	})
}

// TestPipelineMiddleware_Performance tests middleware performance characteristics
func TestPipelineMiddleware_Performance(t *testing.T) {
	config := newTestConfig()
	skipIfNoAPIKey(t, config)

	vectorStoreRetriever, err := retrievers.NewVectorStoreRetriever(&retrievers.VectorStoreRetrieverConfig{
		VectorStore: NewMultilingualMockVectorStore(),
		TopK:        5,
	})
	require.NoError(t, err)

	callMiddleware, streamMiddleware, err := rag.NewPipelineMiddleware(&rag.PipelineConfig{
		DocumentRetrievers: []rag.DocumentRetriever{
			vectorStoreRetriever,
		},
		DocumentRefiners: []rag.DocumentRefiner{
			refiners.NewRankRefiner(3),
		},
	})
	require.NoError(t, err)

	chatClient, err := chat.NewClientWithModel(newTestChatModel(t))
	require.NoError(t, err)

	testQueries := []string{
		"What is RAG?",
		"Explain vector databases",
		"How does semantic search work?",
	}

	for _, query := range testQueries {
		t.Run(query, func(t *testing.T) {
			request, err := chat.NewRequest([]chat.Message{
				chat.NewUserMessage(query),
			})
			require.NoError(t, err)

			ctx, cancel := context.WithTimeout(context.Background(), config.timeout)
			defer cancel()

			text, resp, err := chatClient.
				ChatRequest(request).
				WithMiddlewares(callMiddleware, streamMiddleware).
				Call().
				Text(ctx)

			require.NoError(t, err)
			assert.NotEmpty(t, text)

			docs, ok := resp.Metadata.Get(rag.DocumentContextKey)
			assert.True(t, ok)

			documents, ok := docs.([]*document.Document)
			assert.True(t, ok)
			assert.LessOrEqual(t, len(documents), 3)
		})
	}
}

// TestPipelineMiddleware_ConcurrentRequests tests middleware with concurrent requests
func TestPipelineMiddleware_ConcurrentRequests(t *testing.T) {
	config := newTestConfig()
	skipIfNoAPIKey(t, config)

	vectorStoreRetriever, err := retrievers.NewVectorStoreRetriever(&retrievers.VectorStoreRetrieverConfig{
		VectorStore: NewMultilingualMockVectorStore(),
		TopK:        5,
	})
	require.NoError(t, err)

	callMiddleware, streamMiddleware, err := rag.NewPipelineMiddleware(&rag.PipelineConfig{
		DocumentRetrievers: []rag.DocumentRetriever{
			vectorStoreRetriever,
		},
	})
	require.NoError(t, err)

	chatClient, err := chat.NewClientWithModel(newTestChatModel(t))
	require.NoError(t, err)

	queries := []string{
		"What is RAG?",
		"Explain embeddings",
		"How does retrieval work?",
	}

	// Execute concurrent requests
	type result struct {
		query string
		text  string
		docs  int
		err   error
	}

	results := make(chan result, len(queries))

	for _, query := range queries {
		go func(q string) {
			request, err := chat.NewRequest([]chat.Message{
				chat.NewUserMessage(q),
			})
			if err != nil {
				results <- result{query: q, err: err}
				return
			}

			ctx, cancel := context.WithTimeout(context.Background(), config.timeout)
			defer cancel()

			text, resp, err := chatClient.
				ChatRequest(request).
				WithMiddlewares(callMiddleware, streamMiddleware).
				Call().
				Text(ctx)

			if err != nil {
				results <- result{query: q, err: err}
				return
			}

			docs, _ := resp.Metadata.Get(rag.DocumentContextKey)
			documents, _ := docs.([]*document.Document)

			results <- result{
				query: q,
				text:  text,
				docs:  len(documents),
				err:   nil,
			}
		}(query)
	}

	// Collect results
	for i := 0; i < len(queries); i++ {
		res := <-results
		require.NoError(t, res.err)
		assert.NotEmpty(t, res.text)
		assert.Greater(t, res.docs, 0)
		t.Logf("Query: %s, Docs: %d", res.query, res.docs)
	}
}

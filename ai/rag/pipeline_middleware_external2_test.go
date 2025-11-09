package rag_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/openai/openai-go/v3/option"
	"github.com/qdrant/go-client/qdrant"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Tangerg/lynx/ai/extensions/models/openai"
	qdrant2 "github.com/Tangerg/lynx/ai/extensions/vectorstores/qdrant"
	"github.com/Tangerg/lynx/ai/media/document"
	"github.com/Tangerg/lynx/ai/model"
	"github.com/Tangerg/lynx/ai/model/chat"
	"github.com/Tangerg/lynx/ai/model/embedding"
	"github.com/Tangerg/lynx/ai/rag"
	"github.com/Tangerg/lynx/ai/rag/document/refiners"
	"github.com/Tangerg/lynx/ai/rag/document/retrievers"
	"github.com/Tangerg/lynx/ai/rag/query/augmenters"
	"github.com/Tangerg/lynx/ai/rag/query/expanders"
	"github.com/Tangerg/lynx/ai/rag/query/transformers"
	"github.com/Tangerg/lynx/ai/vectorstore"
	pkgAssert "github.com/Tangerg/lynx/pkg/assert"
	"github.com/Tangerg/lynx/pkg/ptr"
)

const (
	baseURL    = "https://api.siliconflow.cn/v1"
	baseModel  = "Qwen/Qwen3-Embedding-0.6B"
	qdrantHost = "5e8d4810-fc8d-4c9c-b94d-f3831924a57c.us-east-1-1.aws.cloud.qdrant.io"
	qdrantPort = 6334

	indexCreationWait = 2 * time.Second
)

type testFixture struct {
	t              *testing.T
	client         *qdrant.Client
	embeddingModel embedding.Model
	collectionName string
}

func newTestFixture(t *testing.T) *testFixture {
	t.Helper()

	apiKey := os.Getenv("QDRANT_APIKEY")
	if apiKey == "" {
		t.Skip("QDRANT_APIKEY environment variable not set")
	}

	client, err := qdrant.NewClient(&qdrant.Config{
		Host:   qdrantHost,
		Port:   qdrantPort,
		APIKey: apiKey,
		UseTLS: true,
		Cloud:  true,
	})
	require.NoError(t, err, "failed to create Qdrant client")

	apiKey2 := os.Getenv("MODEL_APIKEY")
	if apiKey2 == "" {
		t.Skip("MODEL_APIKEY environment variable not set")
	}

	options, _ := embedding.NewOptions(baseModel)
	options.Dimensions = ptr.Pointer[int64](512)
	embeddingModel, err := openai.NewEmbeddingModel(
		model.NewApiKey(apiKey2),
		options,
		option.WithBaseURL(baseURL),
	)
	require.NoError(t, err, "failed to create embedding model")

	return &testFixture{
		t:              t,
		client:         client,
		embeddingModel: embeddingModel,
		collectionName: "rag_knowledge_base",
	}
}

func (f *testFixture) cleanup() {
	f.t.Helper()

	ctx := context.Background()
	exists, err := f.client.CollectionExists(ctx, f.collectionName)
	if err != nil {
		f.t.Logf("failed to check collection existence: %v", err)
		return
	}

	if exists {
		if err := f.client.DeleteCollection(ctx, f.collectionName); err != nil {
			f.t.Logf("failed to delete collection: %v", err)
		}
	}

	if err := f.client.Close(); err != nil {
		f.t.Logf("failed to close client: %v", err)
	}
}

func (f *testFixture) createStore(config *qdrant2.VectorStoreConfig) *qdrant2.VectorStore {
	f.t.Helper()

	if config.Client == nil {
		config.Client = f.client
	}
	if config.CollectionName == "" {
		config.CollectionName = f.collectionName
	}
	if config.EmbeddingModel == nil {
		config.EmbeddingModel = f.embeddingModel
	}
	if config.DocumentBatcher == nil {
		config.DocumentBatcher = document.NewNop()
	}

	store, err := qdrant2.NewVectorStore(config)
	require.NoError(f.t, err)
	return store
}

func (f *testFixture) insertRAGKnowledge() {
	f.t.Helper()

	ctx := context.Background()
	store := f.createStore(&qdrant2.VectorStoreConfig{
		InitializeSchema:     true,
		StoreDocumentContent: true,
	})

	multilingualDocs := createMultilingualDocuments()

	for lang, docs := range multilingualDocs {
		f.t.Logf("Inserting %d documents for language: %s", len(docs), lang)

		req := pkgAssert.Must(vectorstore.NewCreateRequest(docs))
		err := store.Create(ctx, req)
		require.NoError(f.t, err, "failed to insert %s documents", lang)
	}

	f.t.Log("Waiting for index creation...")
	time.Sleep(indexCreationWait)
	f.t.Log("Knowledge base initialized successfully")
}

func TestPipelineMiddleware_RAGKnowledge(t *testing.T) {
	fixture := newTestFixture(t)
	defer fixture.cleanup()

	t.Log("=== Step 1: Initializing Knowledge Base ===")
	fixture.insertRAGKnowledge()

	vectorStore := fixture.createStore(&qdrant2.VectorStoreConfig{
		InitializeSchema:     false,
		StoreDocumentContent: true,
	})

	config := newTestConfig()
	skipIfNoAPIKey(t, config)

	t.Log("=== Step 2: Setting up Transformers ===")
	compressionTransformer, err := transformers.NewCompressionTransformer(&transformers.CompressionTransformerConfig{
		ChatModel: newTestChatModel(t),
	})
	require.NoError(t, err)

	rewriteTransformer, err := transformers.NewRewriteTransformer(&transformers.RewriteTransformerConfig{
		ChatModel: newTestChatModel(t),
	})
	require.NoError(t, err)

	multiExpander, err := expanders.NewMultiExpander(&expanders.MultiExpanderConfig{
		ChatModel:       newTestChatModel(t),
		IncludeOriginal: true,
		NumberOfQueries: 2,
	})
	require.NoError(t, err)

	vectorStoreRetriever, err := retrievers.NewVectorStoreRetriever(&retrievers.VectorStoreRetrieverConfig{
		VectorStore: vectorStore,
		TopK:        5,
	})
	require.NoError(t, err)

	augmenter, err := augmenters.NewContextualAugmenter(&augmenters.ContextualAugmenterConfig{})
	require.NoError(t, err)

	t.Log("=== Step 3: Creating Pipeline Middleware ===")
	callMiddleware, streamMiddleware, err := rag.NewPipelineMiddleware(&rag.PipelineConfig{
		QueryTransformers: []rag.QueryTransformer{
			compressionTransformer,
			rewriteTransformer,
		},
		QueryExpander: multiExpander,
		DocumentRetrievers: []rag.DocumentRetriever{
			vectorStoreRetriever,
		},
		DocumentRefiners: []rag.DocumentRefiner{
			refiners.NewDeduplicationRefiner(),
			refiners.NewRankRefiner(3),
		},
		QueryAugmenter: augmenter,
	})
	require.NoError(t, err)

	chatClient, err := chat.NewClientWithModel(newTestChatModel(t))
	require.NoError(t, err)

	testQueries := []string{
		"什么是RAG?它有什么优势?",
		"RAG的核心组件有哪些?",
		"向量嵌入在RAG中起什么作用?",
		"RAG如何处理查询转换?",
		"RAG适用于哪些实际应用场景?",
	}

	for i, query := range testQueries {
		t.Logf("\n=== Test Query %d ===", i+1)
		t.Logf("Question: %s", query)

		request, err := chat.NewRequest([]chat.Message{
			chat.NewUserMessage(query),
		})
		require.NoError(t, err)

		ctx, cancel := context.WithTimeout(context.Background(), config.timeout)

		text, resp, err := chatClient.
			ChatRequest(request).
			WithMiddlewares(callMiddleware, streamMiddleware).
			Call().
			Text(ctx)

		cancel()

		require.NoError(t, err, "Query %d failed", i+1)
		assert.NotEmpty(t, text, "Response should not be empty for query %d", i+1)

		docs, ok := resp.Metadata.Get(rag.DocumentContextKey)
		assert.True(t, ok, "Should have retrieved documents for query %d", i+1)
		assert.NotNil(t, docs, "Documents should not be nil for query %d", i+1)

		documents, ok := docs.([]*document.Document)
		assert.True(t, ok, "Documents should be of correct type for query %d", i+1)
		assert.NotEmpty(t, documents, "Should have at least one document for query %d", i+1)
		assert.LessOrEqual(t, len(documents), 3, "Should be limited by rank refiner for query %d", i+1)

		t.Log("\n--- Response ---")
		t.Logf("%s", text)

		t.Log("\n--- Retrieved Documents ---")
		for j, doc := range documents {
			lang, _ := doc.Metadata["language"].(string)
			docType, _ := doc.Metadata["type"].(string)
			t.Logf("Document %d: ID=%s, Language=%s, Type=%s",
				j+1, doc.ID, lang, docType)
			t.Logf("Content preview: %s...", doc.Text)
		}

		t.Log("\n" + strings.Repeat("=", 80))

		if i < len(testQueries)-1 {
			time.Sleep(2 * time.Second)
		}
	}
}

func TestPipelineMiddleware_RAGKnowledge2(t *testing.T) {
	fixture := newTestFixture(t)
	defer fixture.cleanup()

	t.Log("=== Step 1: Initializing Knowledge Base ===")
	fixture.insertRAGKnowledge()

	vectorStore := fixture.createStore(&qdrant2.VectorStoreConfig{
		InitializeSchema:     false,
		StoreDocumentContent: true,
	})

	config := newTestConfig()
	skipIfNoAPIKey(t, config)

	t.Log("=== Step 2: Setting up Transformers ===")
	compressionTransformer, err := transformers.NewCompressionTransformer(&transformers.CompressionTransformerConfig{
		ChatModel: newTestChatModel(t),
	})
	require.NoError(t, err)

	rewriteTransformer, err := transformers.NewRewriteTransformer(&transformers.RewriteTransformerConfig{
		ChatModel: newTestChatModel(t),
	})
	require.NoError(t, err)

	multiExpander, err := expanders.NewMultiExpander(&expanders.MultiExpanderConfig{
		ChatModel:       newTestChatModel(t),
		IncludeOriginal: true,
		NumberOfQueries: 2,
	})
	require.NoError(t, err)

	vectorStoreRetriever, err := retrievers.NewVectorStoreRetriever(&retrievers.VectorStoreRetrieverConfig{
		VectorStore: vectorStore,
		TopK:        5,
	})
	require.NoError(t, err)

	augmenter, err := augmenters.NewContextualAugmenter(&augmenters.ContextualAugmenterConfig{})
	require.NoError(t, err)

	t.Log("=== Step 3: Creating Pipeline Middleware ===")
	callMiddleware, streamMiddleware, err := rag.NewPipelineMiddleware(&rag.PipelineConfig{
		QueryTransformers: []rag.QueryTransformer{
			compressionTransformer,
			rewriteTransformer,
		},
		QueryExpander: multiExpander,
		DocumentRetrievers: []rag.DocumentRetriever{
			vectorStoreRetriever,
		},
		DocumentRefiners: []rag.DocumentRefiner{
			refiners.NewDeduplicationRefiner(),
			refiners.NewRankRefiner(3),
		},
		QueryAugmenter: augmenter,
	})
	require.NoError(t, err)

	chatClient, err := chat.NewClientWithModel(newTestChatModel(t))
	require.NoError(t, err)

	request, err := chat.NewRequest([]chat.Message{
		chat.NewUserMessage("什么是RAG?"),
		chat.NewAssistantMessage("RAG 是 Retrieval-Augmented Generation 的缩写，它是一种将信息检索与文本生成相结合的技术，以提供更加准确和上下文相关的响应。"),
		chat.NewUserMessage("那么它的主要组件是什么?"),
		chat.NewAssistantMessage("RAG架构由三个主要组件组成：查询处理、文档检索和响应生成。"),
		chat.NewUserMessage("展开说说"),
	})
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), config.timeout)

	text, resp, err := chatClient.
		ChatRequest(request).
		WithMiddlewares(callMiddleware, streamMiddleware).
		Call().
		Text(ctx)

	cancel()

	require.NoError(t, err, "Query failed")
	assert.NotEmpty(t, text, "Response should not be empty for query")

	docs, ok := resp.Metadata.Get(rag.DocumentContextKey)
	assert.True(t, ok, "Should have retrieved documents for query")
	assert.NotNil(t, docs, "Documents should not be nil for query")

	documents, ok := docs.([]*document.Document)
	assert.True(t, ok, "Documents should be of correct type for query")
	assert.NotEmpty(t, documents, "Should have at least one document for query")
	assert.LessOrEqual(t, len(documents), 3, "Should be limited by rank refiner for query")

	t.Log("\n--- Response ---")
	t.Logf("%s", text)

	t.Log("\n--- Retrieved Documents ---")
	for j, doc := range documents {
		lang, _ := doc.Metadata["language"].(string)
		docType, _ := doc.Metadata["type"].(string)
		t.Logf("Document %d: ID=%s, Language=%s, Type=%s",
			j+1, doc.ID, lang, docType)
		t.Logf("Content preview: %s...", doc.Text)
	}

	t.Log("\n" + strings.Repeat("=", 80))
}

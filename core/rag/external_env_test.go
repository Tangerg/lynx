package rag_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/openai/openai-go/v3/option"
	"github.com/qdrant/go-client/qdrant"
	"github.com/stretchr/testify/require"

	"github.com/Tangerg/lynx/ai/extensions/models/openai"
	qdrant2 "github.com/Tangerg/lynx/ai/extensions/vectorstores/qdrant"
	"github.com/Tangerg/lynx/ai/media/document"
	"github.com/Tangerg/lynx/ai/model"
	"github.com/Tangerg/lynx/ai/model/chat"
	"github.com/Tangerg/lynx/ai/model/embedding"
	"github.com/Tangerg/lynx/ai/vectorstore"
	pkgAssert "github.com/Tangerg/lynx/pkg/assert"
	"github.com/Tangerg/lynx/pkg/ptr"
)

const (
	defaultBaseURL        = "https://api.siliconflow.cn/v1"
	defaultChatModel      = "Qwen/Qwen2.5-7B-Instruct"
	defaultEmbeddingModel = "Qwen/Qwen3-Embedding-0.6B"
	apiKeyEnvVar          = "API_KEY"
	defaultTimeout        = 60 * time.Second
	qdrantHost            = "5e8d4810-fc8d-4c9c-b94d-f3831924a57c.us-east-1-1.aws.cloud.qdrant.io"
	qdrantPort            = 6334
)

type testConfig struct {
	baseURL string
	model   string
	apiKey  string
	timeout time.Duration
}

func newTestConfig() *testConfig {
	return &testConfig{
		baseURL: getEnvOrDefault("TEST_BASE_URL", defaultBaseURL),
		model:   getEnvOrDefault("TEST_MODEL", defaultChatModel),
		apiKey:  getEnvOrDefault(apiKeyEnvVar, ""),
		timeout: defaultTimeout,
	}
}

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func skipIfNoAPIKey(t *testing.T, config *testConfig) {
	if config.apiKey == "" {
		t.Skipf("Skipping integration test: %s not set", apiKeyEnvVar)
	}
}

func newTestChatModel(t *testing.T) *openai.ChatModel {
	t.Helper()
	config := newTestConfig()
	skipIfNoAPIKey(t, config)

	defaultOptions, err := chat.NewOptions(config.model)
	require.NoError(t, err)

	chatModel, err := openai.NewChatModel(
		&openai.ChatModelConfig{
			ApiKey:         model.NewApiKey(config.apiKey),
			DefaultOptions: defaultOptions,
			RequestOptions: []option.RequestOption{
				option.WithBaseURL(config.baseURL),
			},
		},
	)
	require.NoError(t, err)

	return chatModel
}

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
	})
	require.NoError(t, err, "failed to create Qdrant client")

	apiKey2 := os.Getenv("MODEL_APIKEY")
	if apiKey2 == "" {
		t.Skip("MODEL_APIKEY environment variable not set")
	}

	options, _ := embedding.NewOptions(defaultEmbeddingModel)
	options.Dimensions = ptr.Pointer[int64](512)
	embeddingModel, err := openai.NewEmbeddingModel(
		&openai.EmbeddingModelConfig{
			ApiKey:         model.NewApiKey(apiKey2),
			DefaultOptions: options,
			RequestOptions: []option.RequestOption{
				option.WithBaseURL(defaultBaseURL),
			},
		},
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
	f.t.Log("Knowledge base initialized successfully")
}

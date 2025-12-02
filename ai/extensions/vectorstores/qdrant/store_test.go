package qdrant

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/openai/openai-go/v3/option"
	"github.com/qdrant/go-client/qdrant"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Tangerg/lynx/ai/extensions/models/openai"
	"github.com/Tangerg/lynx/ai/media/document"
	"github.com/Tangerg/lynx/ai/model"
	"github.com/Tangerg/lynx/ai/model/embedding"
	"github.com/Tangerg/lynx/ai/vectorstore"
	"github.com/Tangerg/lynx/ai/vectorstore/filter"
	"github.com/Tangerg/lynx/ai/vectorstore/filter/ast"
	pkgAssert "github.com/Tangerg/lynx/pkg/assert"
)

const (
	baseURL    = "https://api.siliconflow.cn/v1"
	baseModel  = "BAAI/bge-m3"
	qdrantHost = "5e8d4810-fc8d-4c9c-b94d-f3831924a57c.us-east-1-1.aws.cloud.qdrant.io"
	qdrantPort = 6334

	indexCreationWait = 2 * time.Second
	operationWait     = 3 * time.Second
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
		t.Skip("apiKey environment variable not set")
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
		t.Skip("apiKey environment variable not set")
	}
	embeddingModel, err := openai.NewEmbeddingModel(
		&openai.EmbeddingModelConfig{
			ApiKey:         model.NewApiKey(apiKey2),
			DefaultOptions: pkgAssert.Must(embedding.NewOptions(baseModel)),
			RequestOptions: []option.RequestOption{
				option.WithBaseURL(baseURL),
			},
		},
	)
	require.NoError(t, err, "failed to create embedding model")

	return &testFixture{
		t:              t,
		client:         client,
		embeddingModel: embeddingModel,
		collectionName: generateCollectionName(),
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

func (f *testFixture) createStore(config *VectorStoreConfig) *VectorStore {
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

	store, err := NewVectorStore(config)
	require.NoError(f.t, err)
	return store
}

func (f *testFixture) createIndexes() {
	f.t.Helper()
	ctx := context.Background()

	indexes := map[string]qdrant.FieldType{
		"index":    qdrant.FieldType_FieldTypeInteger,
		"category": qdrant.FieldType_FieldTypeKeyword,
		"topic":    qdrant.FieldType_FieldTypeKeyword,
		"year":     qdrant.FieldType_FieldTypeInteger,
	}

	for fieldName, fieldType := range indexes {
		_, err := f.client.CreateFieldIndex(ctx, &qdrant.CreateFieldIndexCollection{
			CollectionName: f.collectionName,
			FieldName:      fieldName,
			FieldType:      fieldType.Enum(),
		})
		require.NoError(f.t, err, "failed to create index for %s", fieldName)
	}
}

func (f *testFixture) insertTestDocuments(count int) []*document.Document {
	f.t.Helper()

	docs := createTestDocuments(count)
	req := pkgAssert.Must(vectorstore.NewCreateRequest(docs))

	ctx := context.Background()
	store := f.createStore(&VectorStoreConfig{
		InitializeSchema:     true,
		StoreDocumentContent: true,
	})

	err := store.Create(ctx, req)
	require.NoError(f.t, err)

	time.Sleep(indexCreationWait)
	return docs
}

func generateCollectionName() string {
	return fmt.Sprintf("test_collection_%d_%d", time.Now().Unix(), time.Now().Nanosecond())
}

func createTestDocuments(count int) []*document.Document {
	docs := make([]*document.Document, count)
	for i := 0; i < count; i++ {
		doc := pkgAssert.Must(document.NewDocument(
			fmt.Sprintf("This is test document number %d about artificial intelligence and machine learning", i),
			nil,
		))
		doc.ID = uuid.NewString()
		doc.Metadata = map[string]any{
			"index":    i,
			"category": "tech",
			"topic":    "AI",
			"year":     2024,
		}
		docs[i] = doc
	}
	return docs
}

func TestVectorStoreConfig_Validate(t *testing.T) {
	fixture := newTestFixture(t)
	defer fixture.cleanup()

	tests := []struct {
		name    string
		config  func() *VectorStoreConfig
		wantErr string
	}{
		{
			name:    "nil config",
			config:  func() *VectorStoreConfig { return nil },
			wantErr: "config is nil",
		},
		{
			name: "missing client",
			config: func() *VectorStoreConfig {
				return &VectorStoreConfig{
					CollectionName:  "test",
					EmbeddingModel:  fixture.embeddingModel,
					DocumentBatcher: document.NewNop(),
				}
			},
			wantErr: "client is required",
		},
		{
			name: "missing collection name",
			config: func() *VectorStoreConfig {
				return &VectorStoreConfig{
					Client:          fixture.client,
					EmbeddingModel:  fixture.embeddingModel,
					DocumentBatcher: document.NewNop(),
				}
			},
			wantErr: "collection name is required",
		},
		{
			name: "missing embedding model",
			config: func() *VectorStoreConfig {
				return &VectorStoreConfig{
					Client:          fixture.client,
					CollectionName:  "test",
					DocumentBatcher: document.NewNop(),
				}
			},
			wantErr: "embedding model is required",
		},
		{
			name: "missing document batcher",
			config: func() *VectorStoreConfig {
				return &VectorStoreConfig{
					Client:         fixture.client,
					CollectionName: "test",
					EmbeddingModel: fixture.embeddingModel,
				}
			},
			wantErr: "document batcher is required",
		},
		{
			name: "valid config",
			config: func() *VectorStoreConfig {
				return &VectorStoreConfig{
					Client:          fixture.client,
					CollectionName:  "test",
					EmbeddingModel:  fixture.embeddingModel,
					DocumentBatcher: document.NewNop(),
				}
			},
			wantErr: "",
		},
		{
			name: "valid config with nil context",
			config: func() *VectorStoreConfig {
				return &VectorStoreConfig{
					Context:         nil,
					Client:          fixture.client,
					CollectionName:  "test",
					EmbeddingModel:  fixture.embeddingModel,
					DocumentBatcher: document.NewNop(),
				}
			},
			wantErr: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := tt.config()
			err := config.Validate()

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			} else {
				require.NoError(t, err)
				if config != nil && config.Context == nil {
					assert.NotNil(t, config.Context)
				}
			}
		})
	}
}

func TestNewVectorStore(t *testing.T) {
	fixture := newTestFixture(t)
	defer fixture.cleanup()

	tests := []struct {
		name   string
		config *VectorStoreConfig
		verify func(*testing.T, *VectorStore)
	}{
		{
			name: "create store with schema initialization",
			config: &VectorStoreConfig{
				InitializeSchema: true,
			},
			verify: func(t *testing.T, store *VectorStore) {
				ctx := context.Background()
				exists, err := fixture.client.CollectionExists(ctx, store.collectionName)
				require.NoError(t, err)
				assert.True(t, exists)
			},
		},
		{
			name: "create store without schema initialization",
			config: &VectorStoreConfig{
				InitializeSchema: false,
			},
			verify: func(t *testing.T, store *VectorStore) {
				assert.NotNil(t, store)
			},
		},
		{
			name: "create store with content storage enabled",
			config: &VectorStoreConfig{
				InitializeSchema:     true,
				StoreDocumentContent: true,
			},
			verify: func(t *testing.T, store *VectorStore) {
				assert.True(t, store.storeDocumentContent)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := fixture.createStore(tt.config)
			require.NotNil(t, store)
			assert.Equal(t, fixture.collectionName, store.collectionName)

			if tt.verify != nil {
				tt.verify(t, store)
			}
		})
	}
}

func TestVectorStore_Create(t *testing.T) {
	fixture := newTestFixture(t)
	defer fixture.cleanup()

	store := fixture.createStore(&VectorStoreConfig{
		InitializeSchema: true,
	})
	fixture.createIndexes()

	tests := []struct {
		name    string
		docs    func() []*document.Document
		wantErr bool
	}{
		{
			name:    "create single document",
			docs:    func() []*document.Document { return createTestDocuments(1) },
			wantErr: false,
		},
		{
			name:    "create multiple documents",
			docs:    func() []*document.Document { return createTestDocuments(5) },
			wantErr: false,
		},
		{
			name:    "create empty documents",
			docs:    func() []*document.Document { return []*document.Document{} },
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			docs := tt.docs()
			req, err := vectorstore.NewCreateRequest(docs)

			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			err = store.Create(context.Background(), req)
			assert.NoError(t, err)
		})
	}
}

func TestVectorStore_Retrieve(t *testing.T) {
	fixture := newTestFixture(t)
	defer fixture.cleanup()

	store := fixture.createStore(&VectorStoreConfig{
		InitializeSchema:     true,
		StoreDocumentContent: true,
	})
	fixture.createIndexes()
	fixture.insertTestDocuments(5)

	tests := []struct {
		name       string
		query      string
		topK       int
		minScore   float64
		filter     ast.Expr
		wantCount  func(int) bool
		wantErr    bool
		verifyDocs func(*testing.T, []*document.Document)
	}{
		{
			name:      "basic retrieval",
			query:     "artificial intelligence",
			topK:      5,
			minScore:  0.0,
			wantCount: func(count int) bool { return count <= 5 },
			wantErr:   false,
			verifyDocs: func(t *testing.T, docs []*document.Document) {
				for _, doc := range docs {
					assert.NotEmpty(t, doc.ID)
					assert.NotEmpty(t, doc.Text)
					assert.NotNil(t, doc.Metadata)
				}
			},
		},
		{
			name:      "retrieval with high threshold",
			query:     "machine learning algorithms",
			topK:      10,
			minScore:  0.5,
			wantCount: func(count int) bool { return count >= 0 },
			wantErr:   false,
			verifyDocs: func(t *testing.T, docs []*document.Document) {
				for _, doc := range docs {
					assert.GreaterOrEqual(t, doc.Score, 0.5)
				}
			},
		},
		{
			name:      "retrieval with metadata filter",
			query:     "artificial intelligence",
			topK:      5,
			minScore:  0.0,
			filter:    filter.EQ("category", "tech"),
			wantCount: func(count int) bool { return count <= 5 },
			wantErr:   false,
			verifyDocs: func(t *testing.T, docs []*document.Document) {
				for _, doc := range docs {
					assert.Equal(t, "tech", doc.Metadata["category"])
				}
			},
		},
		{
			name:     "retrieval with complex filter",
			query:    "AI and machine learning",
			topK:     5,
			minScore: 0.0,
			filter: filter.And(
				filter.EQ("category", "tech"),
				filter.EQ("topic", "AI"),
			),
			wantCount: func(count int) bool { return count <= 5 },
			wantErr:   false,
			verifyDocs: func(t *testing.T, docs []*document.Document) {
				for _, doc := range docs {
					assert.Equal(t, "tech", doc.Metadata["category"])
					assert.Equal(t, "AI", doc.Metadata["topic"])
				}
			},
		},
		{
			name:      "empty query",
			query:     "",
			topK:      5,
			minScore:  0.0,
			wantCount: func(count int) bool { return count == 0 },
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := vectorstore.NewRetrievalRequest(tt.query)
			if tt.wantErr && tt.query == "" {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)

			req.WithTopK(tt.topK).WithMinScore(tt.minScore)
			if tt.filter != nil {
				req.WithFilter(tt.filter)
			}

			docs, err := store.Retrieve(context.Background(), req)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, docs)

				if tt.wantCount != nil {
					assert.True(t, tt.wantCount(len(docs)), "unexpected document count: %d", len(docs))
				}

				if tt.verifyDocs != nil {
					tt.verifyDocs(t, docs)
				}
			}
		})
	}
}

func TestVectorStore_Delete(t *testing.T) {
	fixture := newTestFixture(t)
	defer fixture.cleanup()

	store := fixture.createStore(&VectorStoreConfig{
		InitializeSchema: true,
	})
	fixture.createIndexes()

	ctx := context.Background()
	testDocs := createTestDocuments(5)
	createReq := pkgAssert.Must(vectorstore.NewCreateRequest(testDocs))
	require.NoError(t, store.Create(ctx, createReq))

	time.Sleep(indexCreationWait)

	tests := []struct {
		name    string
		filter  ast.Expr
		wantErr bool
	}{
		{
			name:    "delete by category",
			filter:  filter.EQ("category", "tech"),
			wantErr: false,
		},
		{
			name:    "delete by index range",
			filter:  filter.LT("index", 2),
			wantErr: false,
		},
		{
			name: "delete by complex filter",
			filter: filter.And(
				filter.EQ("category", "tech"),
				filter.GE("index", 3),
			),
			wantErr: false,
		},
		{
			name:    "delete with nil filter",
			filter:  nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := vectorstore.NewDeleteRequest(tt.filter)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)

			err = store.Delete(ctx, req)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestVectorStore_Info(t *testing.T) {
	fixture := newTestFixture(t)
	defer fixture.cleanup()

	store := fixture.createStore(&VectorStoreConfig{
		InitializeSchema: false,
	})

	info := store.Info()
	assert.NotNil(t, info.NativeClient)
	assert.Equal(t, Provider, info.Provider)
	assert.IsType(t, &qdrant.Client{}, info.NativeClient)
}

func TestVectorStore_IntegrationWorkflow(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	fixture := newTestFixture(t)
	defer fixture.cleanup()

	store := fixture.createStore(&VectorStoreConfig{
		InitializeSchema:     true,
		StoreDocumentContent: true,
	})
	fixture.createIndexes()

	ctx := context.Background()

	t.Run("create documents", func(t *testing.T) {
		docs := createTestDocuments(20)
		req := pkgAssert.Must(vectorstore.NewCreateRequest(docs))
		err := store.Create(ctx, req)
		require.NoError(t, err)
	})

	time.Sleep(operationWait)

	t.Run("retrieve documents", func(t *testing.T) {
		req := pkgAssert.Must(vectorstore.NewRetrievalRequest("artificial intelligence and deep learning"))
		req.WithTopK(10).WithMinScore(0.3)

		docs, err := store.Retrieve(ctx, req)
		require.NoError(t, err)
		assert.NotEmpty(t, docs)
		assert.LessOrEqual(t, len(docs), 10)

		for _, doc := range docs {
			assert.NotEmpty(t, doc.Text)
			assert.GreaterOrEqual(t, doc.Score, 0.3)
		}
	})

	t.Run("retrieve with filter", func(t *testing.T) {
		req := pkgAssert.Must(vectorstore.NewRetrievalRequest("machine learning"))
		req.WithTopK(5).WithFilter(filter.LT("index", 10))

		docs, err := store.Retrieve(ctx, req)
		require.NoError(t, err)

		for _, doc := range docs {
			if index, ok := doc.Metadata["index"].(int); ok {
				assert.Less(t, index, 10)
			}
		}
	})

	t.Run("delete documents", func(t *testing.T) {
		req := pkgAssert.Must(vectorstore.NewDeleteRequest(filter.GE("index", 15)))
		err := store.Delete(ctx, req)
		require.NoError(t, err)
	})

	time.Sleep(indexCreationWait)

	t.Run("verify deletion", func(t *testing.T) {
		req := pkgAssert.Must(vectorstore.NewRetrievalRequest("artificial intelligence"))
		req.WithTopK(20).WithMinScore(0.0)

		docs, err := store.Retrieve(ctx, req)
		require.NoError(t, err)

		for _, doc := range docs {
			if index, ok := doc.Metadata["index"].(int); ok {
				assert.Less(t, index, 15)
			}
		}
	})
}

func TestVectorStore_PayloadConversion(t *testing.T) {
	fixture := newTestFixture(t)
	defer fixture.cleanup()

	store := fixture.createStore(&VectorStoreConfig{
		InitializeSchema: true,
	})

	tests := []struct {
		name     string
		metadata map[string]any
	}{
		{
			name: "simple types",
			metadata: map[string]any{
				"string": "value",
				"int":    42,
				"float":  3.14,
				"bool":   true,
			},
		},
		{
			name: "nested map",
			metadata: map[string]any{
				"nested": map[string]any{
					"key1": "value1",
					"key2": 123,
				},
			},
		},
		{
			name: "array values",
			metadata: map[string]any{
				"tags": []any{"tag1", "tag2", "tag3"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := pkgAssert.Must(document.NewDocument("test content", nil))
			doc.ID = uuid.NewString()
			doc.Metadata = tt.metadata

			payload, err := qdrant.TryValueMap(doc.Metadata)
			require.NoError(t, err)
			assert.NotNil(t, payload)

			convertedMetadata := store.convertPayloadToMetadata(payload)
			assert.NotNil(t, convertedMetadata)
		})
	}
}

func TestVectorStore_Close(t *testing.T) {
	fixture := newTestFixture(t)

	store := fixture.createStore(&VectorStoreConfig{
		InitializeSchema: false,
	})

	err := store.Close()
	assert.NoError(t, err)

}

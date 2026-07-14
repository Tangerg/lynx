package azureaisearch

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/google/uuid"

	"github.com/Tangerg/lynx/core/document"
	"github.com/Tangerg/lynx/core/model/embedding"
	"github.com/Tangerg/lynx/core/vectorstore"
	"github.com/Tangerg/lynx/core/vectorstore/filter/ast"
	"github.com/Tangerg/lynx/pkg/math"
	"github.com/Tangerg/lynx/vectorstores/internal/tracing"
)

const (
	Provider = "AzureAISearch"

	// DefaultAPIVersion targets the GA "2024-07-01" REST surface, the
	// first stable release that exposes the typed vector-query
	// payload used by the Lynx store.
	DefaultAPIVersion = "2024-07-01"

	// DefaultContentField / DefaultEmbeddingField / DefaultIDField
	// name the well-known fields written to and read from each
	// document. They must exist on the underlying index schema.
	DefaultContentField   = "content"
	DefaultEmbeddingField = "contentVector"
	DefaultIDField        = "id"
)

// StoreConfig contains configuration options for the Azure AI Search
// vector store. The store talks to the REST surface directly — Azure
// doesn't ship a typed Go SDK for the Search service.
type StoreConfig struct {
	// Context is used for the initial HTTP probe. Optional;
	// defaults to context.Background().
	Context context.Context

	// Endpoint is the search service URL, e.g.
	// "https://my-search.search.windows.net". Required.
	Endpoint string

	// APIKey is the admin API key. Required for both read and write.
	// Use Managed Identity / OAuth via [HTTPClient] for finer
	// authorization control.
	APIKey string

	// IndexName is the index to operate on. Required. The schema
	// must already contain the configured ID, content, vector, and
	// metadata fields — Azure AI Search index schemas are typed and
	// cannot be created lazily.
	IndexName string

	// APIVersion overrides the REST API version. Optional: defaults
	// to [DefaultAPIVersion].
	APIVersion string

	// IDField / ContentField / EmbeddingField name the well-known
	// fields on each document. Optional defaults apply.
	IDField        string
	ContentField   string
	EmbeddingField string

	// VectorProfileName is the index's vector search profile name.
	// Optional. Pure-vector queries don't require it, but the
	// the framework defaults match a profile called "default-profile".
	VectorProfileName string

	// EmbeddingModel produces vectors for the documents. Required.
	EmbeddingModel embedding.Model

	// DocumentBatcher batches documents before upsert. Required.
	DocumentBatcher document.Batcher

	// HTTPClient lets callers override transport (timeouts,
	// proxies, MSAL bearer-token injection). Optional: defaults to
	// http.DefaultClient.
	HTTPClient *http.Client
}

func (c *StoreConfig) Validate() error {
	if c.Endpoint == "" {
		return errors.New("azureaisearch: Endpoint is required")
	}
	if c.APIKey == "" {
		return errors.New("azureaisearch: APIKey is required")
	}
	if c.IndexName == "" {
		return errors.New("azureaisearch: IndexName is required")
	}
	if c.EmbeddingModel == nil {
		return errors.New("azureaisearch: EmbeddingModel is required")
	}
	if c.DocumentBatcher == nil {
		return errors.New("azureaisearch: DocumentBatcher is required")
	}
	return nil
}

// ApplyDefaults fills zero fields with documented defaults.
func (c *StoreConfig) ApplyDefaults() {
	if c.Context == nil {
		c.Context = context.Background()
	}
	c.APIVersion = cmp.Or(c.APIVersion, DefaultAPIVersion)
	c.IDField = cmp.Or(c.IDField, DefaultIDField)
	c.ContentField = cmp.Or(c.ContentField, DefaultContentField)
	c.EmbeddingField = cmp.Or(c.EmbeddingField, DefaultEmbeddingField)
	if c.HTTPClient == nil {
		c.HTTPClient = http.DefaultClient
	}
}

var _ vectorstore.Store = (*Store)(nil)

// Store is an Azure AI Search backed [vectorstore.Store] using the
// REST API.
type Store struct {
	endpoint        string
	apiKey          string
	indexName       string
	apiVersion      string
	idField         string
	contentField    string
	embeddingField  string
	vectorProfile   string
	embeddingModel  embedding.Model
	embeddingClient *embedding.Client
	documentBatcher document.Batcher
	httpClient      *http.Client
}

func NewStore(config StoreConfig) (*Store, error) {
	config.ApplyDefaults()
	if err := config.Validate(); err != nil {
		return nil, err
	}

	embeddingClient, err := embedding.NewClient(config.EmbeddingModel)
	if err != nil {
		return nil, fmt.Errorf("azureaisearch: failed to create embedding client: %w", err)
	}

	return &Store{
		endpoint:        strings.TrimRight(config.Endpoint, "/"),
		apiKey:          config.APIKey,
		indexName:       config.IndexName,
		apiVersion:      config.APIVersion,
		idField:         config.IDField,
		contentField:    config.ContentField,
		embeddingField:  config.EmbeddingField,
		vectorProfile:   config.VectorProfileName,
		embeddingModel:  config.EmbeddingModel,
		embeddingClient: embeddingClient,
		documentBatcher: config.DocumentBatcher,
		httpClient:      config.HTTPClient,
	}, nil
}

// Create embeds documents and uploads them via the
// /indexes/<index>/docs/index endpoint.
func (s *Store) Create(ctx context.Context, req *vectorstore.CreateRequest) (err error) {
	if err = req.Validate(); err != nil {
		return fmt.Errorf("azureaisearch: invalid create request: %w", err)
	}

	ctx, span := tracing.StartCreate(ctx, "azureaisearch", len(req.Documents))
	defer func() { tracing.Finish(span, err) }()

	var batchedDocs [][]*document.Document
	batchedDocs, err = s.documentBatcher.Batch(ctx, req.Documents)
	if err != nil {
		return fmt.Errorf("azureaisearch: failed to batch documents: %w", err)
	}

	for _, docs := range batchedDocs {
		vectors, _, err := s.embeddingClient.
			EmbedWithDocuments(docs).
			Call().
			Embeddings(ctx)
		if err != nil {
			return fmt.Errorf("azureaisearch: failed to generate embeddings: %w", err)
		}

		actions := make([]map[string]any, 0, len(docs))
		for i, doc := range docs {
			id := doc.ID
			if id == "" {
				id = uuid.NewString()
			}
			payload := map[string]any{
				"@search.action": "mergeOrUpload",
				s.idField:        id,
				s.contentField:   doc.Text,
				s.embeddingField: math.ConvertSlice[float64, float32](vectors[i]),
			}
			// Top-level metadata fields — caller is responsible for
			// having declared them in the index schema.
			for k, v := range doc.Metadata {
				payload[k] = v
			}
			actions = append(actions, payload)
		}

		body := map[string]any{"value": actions}
		path := fmt.Sprintf("/indexes/%s/docs/index", url.PathEscape(s.indexName))
		if _, err := s.do(ctx, http.MethodPost, path, body); err != nil {
			return fmt.Errorf("azureaisearch: index documents: %w", err)
		}
	}
	return nil
}

// Retrieve runs a hybrid vector query — the call is pure vector when
// no filter is set, otherwise the filter rides along as the OData
// `$filter` clause.
func (s *Store) Retrieve(ctx context.Context, req *vectorstore.RetrievalRequest) (docs []vectorstore.Match, err error) {
	if err = req.Validate(); err != nil {
		return nil, fmt.Errorf("azureaisearch: invalid retrieval request: %w", err)
	}

	ctx, span := tracing.StartRetrieve(ctx, "azureaisearch", req.TopK, req.MinScore)
	defer func() { tracing.RecordRetrieveResult(span, err, len(docs)) }()

	var vector []float64
	vector, _, err = s.embeddingClient.
		EmbedWithText(req.Query).
		Call().
		Embedding(ctx)
	if err != nil {
		return nil, fmt.Errorf("azureaisearch: failed to embed query: %w", err)
	}
	queryVec := math.ConvertSlice[float64, float32](vector)

	filterStr, err := s.buildFilter(req.Filter)
	if err != nil {
		return nil, err
	}

	vectorQuery := map[string]any{
		"kind":   "vector",
		"vector": queryVec,
		"k":      req.TopK,
		"fields": s.embeddingField,
	}
	body := map[string]any{
		"count":         false,
		"top":           req.TopK,
		"vectorQueries": []any{vectorQuery},
	}
	if filterStr != "" {
		body["filter"] = filterStr
	}

	path := fmt.Sprintf("/indexes/%s/docs/search", url.PathEscape(s.indexName))
	raw, err := s.do(ctx, http.MethodPost, path, body)
	if err != nil {
		return nil, fmt.Errorf("azureaisearch: search: %w", err)
	}

	var parsed struct {
		Value []map[string]any `json:"value"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("azureaisearch: decode search response: %w", err)
	}

	docs = make([]vectorstore.Match, 0, len(parsed.Value))
	for _, row := range parsed.Value {
		match := s.toMatch(row)
		if match.Score < req.MinScore {
			continue
		}
		docs = append(docs, match)
	}
	return docs, nil
}

// Delete removes documents matching the filter expression. The
// service has no filter-based delete, so matching ids are enumerated
// first and then deleted in a batch.
func (s *Store) Delete(ctx context.Context, req *vectorstore.DeleteRequest) (err error) {
	if err = req.Validate(); err != nil {
		return fmt.Errorf("azureaisearch: invalid delete request: %w", err)
	}

	ctx, span := tracing.StartDelete(ctx, "azureaisearch")
	defer func() { tracing.Finish(span, err) }()

	filterStr, err := s.buildFilter(req.Filter)
	if err != nil {
		return err
	}
	if filterStr == "" {
		return errors.New("azureaisearch: refusing to delete on empty filter")
	}

	// Page through ids matching the filter.
	const pageSize = 1000
	ids := make([]string, 0, pageSize)
	skip := 0
	for {
		body := map[string]any{
			"select": s.idField,
			"filter": filterStr,
			"top":    pageSize,
			"skip":   skip,
		}
		path := fmt.Sprintf("/indexes/%s/docs/search", url.PathEscape(s.indexName))
		raw, err := s.do(ctx, http.MethodPost, path, body)
		if err != nil {
			return fmt.Errorf("azureaisearch: enumerate ids: %w", err)
		}
		var parsed struct {
			Value []map[string]any `json:"value"`
		}
		if err := json.Unmarshal(raw, &parsed); err != nil {
			return fmt.Errorf("azureaisearch: decode id page: %w", err)
		}
		if len(parsed.Value) == 0 {
			break
		}
		for _, row := range parsed.Value {
			if id, ok := row[s.idField].(string); ok {
				ids = append(ids, id)
			}
		}
		if len(parsed.Value) < pageSize {
			break
		}
		skip += len(parsed.Value)
	}

	if len(ids) == 0 {
		return nil
	}

	// Batch deletes in groups of 1000 (Azure AI Search's per-request
	// document cap).
	for start := 0; start < len(ids); start += 1000 {
		end := start + 1000
		if end > len(ids) {
			end = len(ids)
		}
		actions := make([]map[string]any, 0, end-start)
		for _, id := range ids[start:end] {
			actions = append(actions, map[string]any{
				"@search.action": "delete",
				s.idField:        id,
			})
		}
		body := map[string]any{"value": actions}
		path := fmt.Sprintf("/indexes/%s/docs/index", url.PathEscape(s.indexName))
		if _, err := s.do(ctx, http.MethodPost, path, body); err != nil {
			return fmt.Errorf("azureaisearch: delete batch: %w", err)
		}
	}
	return nil
}

func (s *Store) buildFilter(filter ast.Expr) (string, error) {
	if filter == nil {
		return "", nil
	}
	v := NewVisitor()
	if err := v.Visit(filter); err != nil {
		return "", fmt.Errorf("azureaisearch: convert filter: %w", err)
	}
	return v.Result(), nil
}

func (s *Store) toMatch(row map[string]any) vectorstore.Match {
	doc := &document.Document{}
	if id, ok := row[s.idField].(string); ok {
		doc.ID = id
	}
	if text, ok := row[s.contentField].(string); ok {
		doc.Text = text
	}
	// @search.score is what AI Search returns for vector results —
	// it's already clamped roughly to [0, 1] for cosine.
	score, _ := row["@search.score"].(float64)

	// Metadata is everything except the reserved fields and the
	// embedding vector itself.
	meta := make(map[string]any, len(row))
	for k, v := range row {
		switch k {
		case s.idField, s.contentField, s.embeddingField,
			"@search.score", "@search.rerankerScore", "@search.highlights",
			"@search.captions":
			continue
		}
		meta[k] = v
	}
	if len(meta) > 0 {
		doc.Metadata = meta
	}
	return vectorstore.Match{Document: doc, Score: score}
}

// do issues a JSON request to the Search REST surface and returns the
// raw response body on success.
func (s *Store) do(ctx context.Context, method, path string, body any) ([]byte, error) {
	u := fmt.Sprintf("%s%s?api-version=%s", s.endpoint, path, url.QueryEscape(s.apiVersion))

	var reqBody io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("encode request: %w", err)
		}
		reqBody = bytes.NewReader(buf)
	}

	req, err := http.NewRequestWithContext(ctx, method, u, reqBody)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("api-key", s.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("status=%d body=%s", resp.StatusCode, string(respBody))
	}
	return respBody, nil
}

func (s *Store) Metadata() vectorstore.StoreMetadata {
	return vectorstore.StoreMetadata{
		NativeClient: s.httpClient,
		Provider:     Provider,
	}
}

func (s *Store) Close() error { return nil }

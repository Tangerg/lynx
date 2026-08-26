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

	"github.com/Tangerg/lynx/core/document"
	"github.com/Tangerg/lynx/core/embedding"
	"github.com/Tangerg/lynx/core/embeddingclient"
	"github.com/Tangerg/lynx/core/metadata"
	"github.com/Tangerg/lynx/core/vectorstore"
	"github.com/Tangerg/lynx/core/vectorstore/filter"
)

// SimilarityMetric records the metric configured on the existing Azure AI
// Search vector field.
type SimilarityMetric string

const (
	SimilarityCosine    SimilarityMetric = "cosine"
	SimilarityDot       SimilarityMetric = "dotProduct"
	SimilarityEuclidean SimilarityMetric = "euclidean"
)

func (metric SimilarityMetric) score(raw float64) vectorstore.Score {
	switch metric {
	case SimilarityCosine:
		// Azure emits 1/(1+cosine_distance). Recover cosine similarity,
		// then apply Lynx's [-1,1] to [0,1] normalization.
		return vectorstore.ScoreFromCosineSimilarity(2 - 1/raw)
	case SimilarityDot, SimilarityEuclidean:
		// Azure documents both native vector scores as [0,1].
		return vectorstore.ScoreFromValue(raw)
	default:
		return vectorstore.ScoreFromValue(raw)
	}
}

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
	DocumentBatcher vectorstore.Batcher

	// SimilarityMetric must match the metric in the index's vector-search
	// algorithm configuration. Required because @search.score is metric-specific.
	SimilarityMetric SimilarityMetric

	// HTTPClient lets callers override transport (timeouts,
	// proxies, MSAL bearer-token injection). Optional: defaults to
	// http.DefaultClient.
	HTTPClient *http.Client
}

func (c StoreConfig) Validate() error {
	c.applyDefaults()
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
	if c.SimilarityMetric == "" {
		return errors.New("azureaisearch: SimilarityMetric is required")
	}
	switch c.SimilarityMetric {
	case SimilarityCosine, SimilarityDot, SimilarityEuclidean:
	default:
		return fmt.Errorf("azureaisearch: unsupported SimilarityMetric %q", c.SimilarityMetric)
	}
	return nil
}

// applyDefaults fills zero fields with documented defaults.
func (c *StoreConfig) applyDefaults() {
	c.APIVersion = cmp.Or(c.APIVersion, DefaultAPIVersion)
	c.IDField = cmp.Or(c.IDField, DefaultIDField)
	c.ContentField = cmp.Or(c.ContentField, DefaultContentField)
	c.EmbeddingField = cmp.Or(c.EmbeddingField, DefaultEmbeddingField)
	if c.HTTPClient == nil {
		c.HTTPClient = http.DefaultClient
	}
}

var (
	_ vectorstore.Indexer       = (*Store)(nil)
	_ vectorstore.Searcher      = (*Store)(nil)
	_ vectorstore.FilterDeleter = (*Store)(nil)
)

// Store implements vector-store capabilities through the Azure AI Search REST
// API.
type Store struct {
	endpoint         string
	apiKey           string
	indexName        string
	apiVersion       string
	idField          string
	contentField     string
	embeddingField   string
	vectorProfile    string
	embeddingClient  embeddingclient.Client
	documentBatcher  vectorstore.Batcher
	similarityMetric SimilarityMetric
	httpClient       *http.Client
}

func NewStore(config StoreConfig) (*Store, error) {
	config.applyDefaults()
	if err := config.Validate(); err != nil {
		return nil, err
	}

	embeddingClient, err := embeddingclient.New(config.EmbeddingModel)
	if err != nil {
		return nil, fmt.Errorf("azureaisearch: create embedding client: %w", err)
	}

	return &Store{
		endpoint:         strings.TrimRight(config.Endpoint, "/"),
		apiKey:           config.APIKey,
		indexName:        config.IndexName,
		apiVersion:       config.APIVersion,
		idField:          config.IDField,
		contentField:     config.ContentField,
		embeddingField:   config.EmbeddingField,
		vectorProfile:    config.VectorProfileName,
		embeddingClient:  embeddingClient,
		documentBatcher:  config.DocumentBatcher,
		similarityMetric: config.SimilarityMetric,
		httpClient:       config.HTTPClient,
	}, nil
}

// Index embeds documents and uploads them via the
// /indexes/<index>/docs/index endpoint.
func (s *Store) Index(ctx context.Context, request *vectorstore.IndexRequest) (err error) {
	if err := request.Validate(); err != nil {
		return fmt.Errorf("azureaisearch.Store.Index: %w", err)
	}

	var batches []*vectorstore.IndexRequest
	batches, err = request.Batch(ctx, s.documentBatcher)
	if err != nil {
		return fmt.Errorf("azureaisearch: batch documents: %w", err)
	}

	for _, batch := range batches {
		docs := batch.Documents
		vectors, err := s.embeddingClient.EmbedDocuments(ctx, docs)
		if err != nil {
			return fmt.Errorf("azureaisearch: embed documents: %w", err)
		}

		actions := make([]map[string]any, 0, len(docs))
		for i, doc := range docs {
			id := doc.ID
			metadataValues, err := doc.Metadata.Values()
			if err != nil {
				return fmt.Errorf("azureaisearch: decode metadata for %s: %w", id, err)
			}
			payload := map[string]any{
				"@search.action": "mergeOrUpload",
				s.idField:        id,
				s.contentField:   doc.Text,
				s.embeddingField: embedding.Float32Vector(vectors[i]),
			}
			// Top-level metadata fields — caller is responsible for
			// having declared them in the index schema.
			for k, v := range metadataValues {
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

// Search runs a hybrid vector query — the call is pure vector when
// no filter is set, otherwise the filter rides along as the OData
// `$filter` clause.
func (s *Store) Search(ctx context.Context, req *vectorstore.SearchRequest) (response *vectorstore.SearchResponse, err error) {
	var docs []*vectorstore.SearchResult
	if err = req.Validate(); err != nil {
		return nil, fmt.Errorf("azureaisearch.Store.Search: %w", err)
	}

	defer func() {
		if err == nil {
			err = response.ValidateFor(req)
		}
	}()

	var vector []float64
	vector, err = s.embeddingClient.EmbedText(ctx, req.Query)
	if err != nil {
		return nil, fmt.Errorf("azureaisearch: embed query: %w", err)
	}
	queryVec := embedding.Float32Vector(vector)

	filterStr, err := s.buildFilter(req.Options.Filter)
	if err != nil {
		return nil, err
	}

	vectorQuery := map[string]any{
		"kind":   "vector",
		"vector": queryVec,
		"k":      req.Options.TopK,
		"fields": s.embeddingField,
	}
	body := map[string]any{
		"count":         false,
		"top":           req.Options.TopK,
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

	docs = make([]*vectorstore.SearchResult, 0, len(parsed.Value))
	for _, row := range parsed.Value {
		match, err := s.toMatch(row)
		if err != nil {
			return nil, err
		}
		if match.Score < req.Options.MinScore {
			continue
		}
		docs = append(docs, match)
	}
	return &vectorstore.SearchResponse{Results: docs}, nil
}

// Delete removes documents matching the filter expression. The
// service has no filter-based delete, so matching ids are enumerated
// first and then deleted in a batch.

func (s *Store) DeleteWhere(ctx context.Context, expr filter.Predicate) (err error) {
	if expr == nil {
		return vectorstore.ErrMissingFilter
	}
	if err = expr.Validate(); err != nil {
		return fmt.Errorf("azureaisearch.Store.DeleteWhere: %w", err)
	}

	filterStr, err := s.buildFilter(expr)
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

func (s *Store) buildFilter(filter filter.Predicate) (string, error) {
	if filter == nil {
		return "", nil
	}
	v := NewVisitor()
	if err := filter.Accept(v); err != nil {
		return "", fmt.Errorf("azureaisearch: convert filter: %w", err)
	}
	return v.Result(), nil
}

func (s *Store) toMatch(row map[string]any) (*vectorstore.SearchResult, error) {
	doc := &document.Document{}
	id, ok := row[s.idField].(string)
	if !ok || id == "" {
		return nil, fmt.Errorf("azureaisearch: result is missing string field %q", s.idField)
	}
	text, ok := row[s.contentField].(string)
	if !ok || text == "" {
		return nil, fmt.Errorf("azureaisearch: result is missing string field %q", s.contentField)
	}
	doc.ID = id
	doc.Text = text
	rawScore, ok := row["@search.score"].(float64)
	if !ok {
		return nil, errors.New("azureaisearch: result is missing numeric @search.score")
	}
	score := s.similarityMetric.score(rawScore)

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
		var err error
		doc.Metadata, err = metadata.FromValues(meta)
		if err != nil {
			return nil, fmt.Errorf("azureaisearch: convert metadata: %w", err)
		}
	}
	return &vectorstore.SearchResult{Document: doc, Score: score}, nil
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

func (s *Store) Close() error { return nil }

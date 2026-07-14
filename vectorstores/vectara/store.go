package vectara

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
	"github.com/Tangerg/lynx/core/metadata"
	"github.com/Tangerg/lynx/core/vectorstore"
	"github.com/Tangerg/lynx/core/vectorstore/filter/ast"
	"github.com/Tangerg/lynx/vectorstores"
	"github.com/Tangerg/lynx/vectorstores/internal/tracing"
)

const (
	Provider = "Vectara"

	// DefaultEndpoint is Vectara's public REST endpoint.
	DefaultEndpoint = "https://api.vectara.io"

	// DefaultAPIVersion targets the v2 API surface.
	DefaultAPIVersion = "v2"
)

// StoreConfig contains configuration options for the Vectara vector
// store. Vectara is a managed RAG service that handles embedding,
// chunking, and retrieval internally — the store sends raw text to
// the API and does NOT need an [embedding.Model]. This is unlike
// every other lynx vector store.
type StoreConfig struct {
	Context context.Context

	// Endpoint is the Vectara API endpoint. Optional: defaults to
	// [DefaultEndpoint].
	Endpoint string

	// APIKey is the Vectara API key. Required.
	APIKey string

	// CorpusKey identifies the Vectara corpus. Required.
	CorpusKey string

	// DocumentBatcher batches documents before upload. Required.
	DocumentBatcher vectorstores.Batcher

	// MetadataPrefix overrides the metadata accessor prefix used by
	// the filter visitor. Optional: defaults to "doc" so filters
	// address `doc.<key>` paths.
	MetadataPrefix string

	// HTTPClient lets callers override transport. Optional:
	// defaults to http.DefaultClient.
	HTTPClient *http.Client
}

func (c *StoreConfig) Validate() error {
	if c.APIKey == "" {
		return errors.New("vectara: APIKey is required")
	}
	if c.CorpusKey == "" {
		return errors.New("vectara: CorpusKey is required")
	}
	if c.DocumentBatcher == nil {
		return errors.New("vectara: DocumentBatcher is required")
	}
	return nil
}

// ApplyDefaults fills zero fields with documented defaults.
func (c *StoreConfig) ApplyDefaults() {
	if c.Context == nil {
		c.Context = context.Background()
	}
	c.Endpoint = cmp.Or(c.Endpoint, DefaultEndpoint)
	if c.MetadataPrefix == "" {
		c.MetadataPrefix = "doc"
	}
	if c.HTTPClient == nil {
		c.HTTPClient = http.DefaultClient
	}
}

var _ vectorstore.Store = (*Store)(nil)

// Store is a Vectara-backed [vectorstore.Store] implementation. Note
// that Vectara handles embedding internally; the user's text is sent
// raw and Vectara generates its own vectors per its configured
// embedder.
type Store struct {
	endpoint        string
	apiKey          string
	corpusKey       string
	metadataPrefix  string
	documentBatcher vectorstores.Batcher
	httpClient      *http.Client
}

func NewStore(config StoreConfig) (*Store, error) {
	config.ApplyDefaults()
	if err := config.Validate(); err != nil {
		return nil, err
	}
	return &Store{
		endpoint:        strings.TrimRight(config.Endpoint, "/"),
		apiKey:          config.APIKey,
		corpusKey:       config.CorpusKey,
		metadataPrefix:  config.MetadataPrefix,
		documentBatcher: config.DocumentBatcher,
		httpClient:      config.HTTPClient,
	}, nil
}

// Create uploads documents to the corpus via Vectara's index API. The
// service performs its own embedding internally, so no embedding
// client is required here.
func (s *Store) Create(ctx context.Context, req *vectorstore.CreateRequest) (err error) {
	if err = req.Validate(); err != nil {
		return fmt.Errorf("vectara: invalid create request: %w", err)
	}

	ctx, span := tracing.StartCreate(ctx, "vectara", len(req.Documents))
	defer func() { tracing.Finish(span, err) }()

	var batchedDocs [][]*document.Document
	batchedDocs, err = s.documentBatcher.Batch(ctx, req.Documents)
	if err != nil {
		return fmt.Errorf("vectara: failed to batch documents: %w", err)
	}

	path := fmt.Sprintf("/%s/corpora/%s/documents",
		DefaultAPIVersion, url.PathEscape(s.corpusKey))

	for _, docs := range batchedDocs {
		for _, doc := range docs {
			id := doc.ID
			if id == "" {
				id = uuid.NewString()
			}
			metadataValues, err := doc.Metadata.Values()
			if err != nil {
				return fmt.Errorf("vectara: decode metadata for %s: %w", id, err)
			}
			payload := map[string]any{
				"id":       id,
				"type":     "core",
				"metadata": metaOrEmpty(metadataValues),
				"document_parts": []any{
					map[string]any{"text": doc.Text},
				},
			}
			if _, err := s.do(ctx, http.MethodPost, path, payload); err != nil {
				return fmt.Errorf("vectara: upload %s: %w", id, err)
			}
		}
	}
	return nil
}

// Retrieve runs a Vectara semantic search.
func (s *Store) Retrieve(ctx context.Context, req *vectorstore.RetrievalRequest) (docs []vectorstore.Match, err error) {
	if err = req.Validate(); err != nil {
		return nil, fmt.Errorf("vectara: invalid retrieval request: %w", err)
	}

	ctx, span := tracing.StartRetrieve(ctx, "vectara", req.TopK, req.MinScore)
	defer func() { tracing.RecordRetrieveResult(span, err, len(docs)) }()

	searchOpts := map[string]any{
		"limit": req.TopK,
	}
	filterFragment, err := s.buildFilter(req.Filter)
	if err != nil {
		return nil, err
	}
	if filterFragment != "" {
		searchOpts["metadata_filter"] = filterFragment
	}

	payload := map[string]any{
		"query":  req.Query,
		"search": searchOpts,
	}

	path := fmt.Sprintf("/%s/corpora/%s/query",
		DefaultAPIVersion, url.PathEscape(s.corpusKey))
	raw, err := s.do(ctx, http.MethodPost, path, payload)
	if err != nil {
		return nil, fmt.Errorf("vectara: query: %w", err)
	}

	var parsed struct {
		SearchResults []struct {
			Text       string         `json:"text"`
			Score      float64        `json:"score"`
			DocumentID string         `json:"document_id"`
			Metadata   map[string]any `json:"document_metadata"`
		} `json:"search_results"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("vectara: decode query response: %w", err)
	}

	docs = make([]vectorstore.Match, 0, len(parsed.SearchResults))
	for _, hit := range parsed.SearchResults {
		if hit.Score < req.MinScore {
			continue
		}
		metadata, err := metadata.FromValues(hit.Metadata)
		if err != nil {
			return nil, fmt.Errorf("vectara: encode metadata: %w", err)
		}
		docs = append(docs, vectorstore.Match{
			Document: &document.Document{ID: hit.DocumentID, Text: hit.Text, Metadata: metadata},
			Score:    hit.Score,
		})
	}
	return docs, nil
}

// Delete removes documents matching the filter via Vectara's
// document-level delete endpoint. Vectara has no bulk filter-delete,
// so matching ids are enumerated first, then deleted one-by-one.
func (s *Store) Delete(ctx context.Context, req *vectorstore.DeleteRequest) (err error) {
	if err = req.Validate(); err != nil {
		return fmt.Errorf("vectara: invalid delete request: %w", err)
	}

	ctx, span := tracing.StartDelete(ctx, "vectara")
	defer func() { tracing.Finish(span, err) }()

	filterFragment, err := s.buildFilter(req.Filter)
	if err != nil {
		return err
	}
	if filterFragment == "" {
		return errors.New("vectara: refusing to delete on empty filter")
	}

	listPath := fmt.Sprintf("/%s/corpora/%s/documents?metadata_filter=%s&limit=100",
		DefaultAPIVersion, url.PathEscape(s.corpusKey), url.QueryEscape(filterFragment))

	for {
		raw, err := s.do(ctx, http.MethodGet, listPath, nil)
		if err != nil {
			return fmt.Errorf("vectara: list documents: %w", err)
		}
		var parsed struct {
			Documents []struct {
				ID string `json:"id"`
			} `json:"documents"`
			Metadata struct {
				PageKey string `json:"page_key"`
			} `json:"metadata"`
		}
		if err := json.Unmarshal(raw, &parsed); err != nil {
			return fmt.Errorf("vectara: decode list response: %w", err)
		}
		if len(parsed.Documents) == 0 {
			return nil
		}
		for _, doc := range parsed.Documents {
			delPath := fmt.Sprintf("/%s/corpora/%s/documents/%s",
				DefaultAPIVersion, url.PathEscape(s.corpusKey), url.PathEscape(doc.ID))
			if _, err := s.do(ctx, http.MethodDelete, delPath, nil); err != nil {
				return fmt.Errorf("vectara: delete %s: %w", doc.ID, err)
			}
		}
		if parsed.Metadata.PageKey == "" {
			return nil
		}
		listPath = fmt.Sprintf("/%s/corpora/%s/documents?metadata_filter=%s&limit=100&page_key=%s",
			DefaultAPIVersion, url.PathEscape(s.corpusKey),
			url.QueryEscape(filterFragment), url.QueryEscape(parsed.Metadata.PageKey))
	}
}

func (s *Store) buildFilter(filter ast.Expr) (string, error) {
	if filter == nil {
		return "", nil
	}
	v := NewVisitor(s.metadataPrefix)
	if err := v.Visit(filter); err != nil {
		return "", fmt.Errorf("vectara: convert filter: %w", err)
	}
	return v.Result(), nil
}

func (s *Store) do(ctx context.Context, method, path string, body any) ([]byte, error) {
	u := s.endpoint + path

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
	req.Header.Set("x-api-key", s.apiKey)
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

func metaOrEmpty(m map[string]any) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	return m
}

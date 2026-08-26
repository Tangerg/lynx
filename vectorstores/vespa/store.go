package vespa

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
	"github.com/Tangerg/lynx/core/metadata"
	"github.com/Tangerg/lynx/core/vectorstore"
	"github.com/Tangerg/lynx/core/vectorstore/filter"
	"github.com/Tangerg/lynx/embeddingclient"
)

const (
	Provider = "Vespa"

	// DefaultContentField names the document field that stores the
	// raw text.
	DefaultContentField = "content"

	// DefaultEmbeddingField names the document field that stores the
	// vector tensor.
	DefaultEmbeddingField = "embedding"

	// DefaultIDField names the field used for the Lynx document id.
	DefaultIDField = "doc_id"

	// DefaultQueryTensorName names the rank-profile query tensor.
	DefaultQueryTensorName = "q"
)

// StoreConfig contains configuration options for the Vespa vector
// store. Vespa uses an HTTP REST surface; the store assumes the
// schema (the .sd file) is provisioned out of band — Vespa schema
// management is YAML/SDL and lives in the application package.
type StoreConfig struct {
	// Endpoint is the Vespa container endpoint (Document API + search
	// API), e.g. "https://my-app.aws-us-east-1c.z.vespa-app.cloud" or
	// "http://localhost:8080". Required.
	Endpoint string

	// SchemaName is the document type name (matches the schema name
	// in the .sd file). Required.
	SchemaName string

	// Namespace is the document-id namespace component. Required by
	// the Vespa document-id grammar but commonly defaults to the
	// schema name.
	Namespace string

	// ContentCluster names the content cluster targeted by visit
	// API delete-by-filter calls. Required for delete to work.
	ContentCluster string

	// EmbeddingField / ContentField / IDField name the well-known
	// schema fields the store writes to. Optional defaults apply.
	EmbeddingField string
	ContentField   string
	IDField        string

	// QueryTensorName is the query tensor declared by RankingProfile. Optional:
	// defaults to [DefaultQueryTensorName].
	QueryTensorName string

	// RankingProfile is the Vespa rank profile used for nearest-neighbor
	// scoring. It must rank by closeness(field, <EmbeddingField>), whose
	// relevance is in [0, 1]. Required: Vespa's built-in default profile uses
	// nativeRank and does not represent vector similarity.
	RankingProfile string

	// EmbeddingModel produces vectors for the documents. Required.
	EmbeddingModel embedding.Model

	// DocumentBatcher batches documents before upload. Required.
	DocumentBatcher vectorstore.Batcher

	// HTTPClient lets callers override transport (timeouts,
	// proxies, mTLS for Vespa Cloud). Optional: defaults to
	// http.DefaultClient.
	HTTPClient *http.Client
}

func (c StoreConfig) Validate() error {
	c.applyDefaults()
	if c.Endpoint == "" {
		return errors.New("vespa: Endpoint is required")
	}
	if c.SchemaName == "" {
		return errors.New("vespa: SchemaName is required")
	}
	if c.RankingProfile == "" {
		return errors.New("vespa: RankingProfile is required")
	}
	if c.EmbeddingModel == nil {
		return errors.New("vespa: EmbeddingModel is required")
	}
	if c.DocumentBatcher == nil {
		return errors.New("vespa: DocumentBatcher is required")
	}
	return c.validateIdentifiers()
}

func (c StoreConfig) validateIdentifiers() error {
	if err := identifier(c.SchemaName).validate("SchemaName"); err != nil {
		return err
	}
	if err := identifier(c.Namespace).validate("Namespace"); err != nil {
		return err
	}
	if err := identifier(c.EmbeddingField).validate("EmbeddingField"); err != nil {
		return err
	}
	if err := identifier(c.ContentField).validate("ContentField"); err != nil {
		return err
	}
	if err := identifier(c.IDField).validate("IDField"); err != nil {
		return err
	}
	if err := identifier(c.QueryTensorName).validate("QueryTensorName"); err != nil {
		return err
	}
	if err := identifier(c.RankingProfile).validate("RankingProfile"); err != nil {
		return err
	}
	if c.ContentCluster == "" {
		return nil
	}
	return identifier(c.ContentCluster).validate("ContentCluster")
}

// applyDefaults fills zero fields with documented defaults.
func (c *StoreConfig) applyDefaults() {
	if c.Namespace == "" {
		c.Namespace = c.SchemaName
	}
	c.EmbeddingField = cmp.Or(c.EmbeddingField, DefaultEmbeddingField)
	c.ContentField = cmp.Or(c.ContentField, DefaultContentField)
	c.IDField = cmp.Or(c.IDField, DefaultIDField)
	c.QueryTensorName = cmp.Or(c.QueryTensorName, DefaultQueryTensorName)
	if c.HTTPClient == nil {
		c.HTTPClient = http.DefaultClient
	}
}

var (
	_ vectorstore.Indexer       = (*Store)(nil)
	_ vectorstore.Searcher      = (*Store)(nil)
	_ vectorstore.FilterDeleter = (*Store)(nil)
)

// Store implements vector-store capabilities through Vespa's REST API.
type Store struct {
	endpoint        string
	schemaName      string
	namespace       string
	contentCluster  string
	embeddingField  string
	contentField    string
	idField         string
	queryTensorName string
	rankingProfile  string
	embeddingClient embeddingclient.Client
	documentBatcher vectorstore.Batcher
	httpClient      *http.Client
}

func NewStore(config StoreConfig) (*Store, error) {
	config.applyDefaults()
	if err := config.Validate(); err != nil {
		return nil, err
	}
	embeddingClient, err := embeddingclient.New(config.EmbeddingModel)
	if err != nil {
		return nil, fmt.Errorf("vespa: create embedding client: %w", err)
	}
	return &Store{
		endpoint:        strings.TrimRight(config.Endpoint, "/"),
		schemaName:      config.SchemaName,
		namespace:       config.Namespace,
		contentCluster:  config.ContentCluster,
		embeddingField:  config.EmbeddingField,
		contentField:    config.ContentField,
		idField:         config.IDField,
		queryTensorName: config.QueryTensorName,
		rankingProfile:  config.RankingProfile,
		embeddingClient: embeddingClient,
		documentBatcher: config.DocumentBatcher,
		httpClient:      config.HTTPClient,
	}, nil
}

// Index embeds documents and PUTs them through the Vespa Document
// API. Each PUT is `POST /document/v1/<namespace>/<schema>/docid/<id>`.
func (s *Store) Index(ctx context.Context, request *vectorstore.IndexRequest) (err error) {
	if err := request.Validate(); err != nil {
		return fmt.Errorf("vespa.Store.Index: %w", err)
	}

	var batches []*vectorstore.IndexRequest
	batches, err = request.Batch(ctx, s.documentBatcher)
	if err != nil {
		return fmt.Errorf("vespa: batch documents: %w", err)
	}

	for _, batch := range batches {
		docs := batch.Documents
		vectors, err := s.embeddingClient.EmbedDocuments(ctx, docs)
		if err != nil {
			return fmt.Errorf("vespa: embed documents: %w", err)
		}
		for i, doc := range docs {
			id := doc.ID
			fields := map[string]any{
				s.idField:        id,
				s.contentField:   doc.Text,
				s.embeddingField: map[string]any{"values": embedding.Float32Vector(vectors[i])},
			}
			for k, v := range doc.Metadata {
				fields[k] = v
			}
			body := map[string]any{"fields": fields}
			path := fmt.Sprintf("/document/v1/%s/%s/docid/%s",
				url.PathEscape(s.namespace), url.PathEscape(s.schemaName), url.PathEscape(id))
			if _, err := s.do(ctx, http.MethodPost, path, body); err != nil {
				return fmt.Errorf("vespa: PUT document %s: %w", id, err)
			}
		}
	}
	return nil
}

// Search runs a nearestNeighbor YQL query.
func (s *Store) Search(ctx context.Context, req *vectorstore.SearchRequest) (response *vectorstore.SearchResponse, err error) {
	var docs []*vectorstore.SearchResult
	if err = req.Validate(); err != nil {
		return nil, fmt.Errorf("vespa.Store.Search: %w", err)
	}

	defer func() {
		if err == nil {
			err = response.ValidateFor(req)
		}
	}()

	var vector []float64
	vector, err = s.embeddingClient.EmbedText(ctx, req.Query)
	if err != nil {
		return nil, fmt.Errorf("vespa: embed query: %w", err)
	}
	queryVec := embedding.Float32Vector(vector)

	filterFragment, err := s.buildFilter(req.Options.Filter)
	if err != nil {
		return nil, err
	}

	nn := fmt.Sprintf("{targetHits:%d}nearestNeighbor(%s, %s)",
		req.Options.TopK, s.embeddingField, s.queryTensorName)
	yql := fmt.Sprintf("select * from %s where %s", s.schemaName, nn)
	if filterFragment != "" {
		yql = yql + " and " + filterFragment
	}

	body := map[string]any{
		"yql":  yql,
		"hits": req.Options.TopK,
		fmt.Sprintf("input.query(%s)", s.queryTensorName): map[string]any{"values": queryVec},
		"ranking": s.rankingProfile,
	}

	raw, err := s.do(ctx, http.MethodPost, "/search/", body)
	if err != nil {
		return nil, fmt.Errorf("vespa: search: %w", err)
	}

	var parsed struct {
		Root struct {
			Children []struct {
				ID        string         `json:"id"`
				Relevance *float64       `json:"relevance"`
				Fields    map[string]any `json:"fields"`
			} `json:"children"`
		} `json:"root"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("vespa: decode search response: %w", err)
	}

	docs = make([]*vectorstore.SearchResult, 0, len(parsed.Root.Children))
	for _, hit := range parsed.Root.Children {
		if hit.Relevance == nil {
			return nil, errors.New("vespa: search hit is missing relevance")
		}
		// Vespa relevance for nearestNeighbor is the configured
		// distance metric's similarity directly (cosine: [0, 1]).
		score := vectorstore.ScoreFromValue(*hit.Relevance)
		if score < req.Options.MinScore {
			continue
		}
		doc, err := s.toDocument(hit.ID, hit.Fields)
		if err != nil {
			return nil, err
		}
		docs = append(docs, &vectorstore.SearchResult{Document: doc, Score: score})
	}
	return &vectorstore.SearchResponse{Results: docs}, nil
}

// Delete removes documents matching the filter expression via the
// `selection` parameter on the Document API.
//
// Vespa selection expressions live under their own mini language;
// rather than translate the AST a second way, the approach routes
// through a YQL search to enumerate ids, then deletes them.

func (s *Store) DeleteWhere(ctx context.Context, expr filter.Predicate) (err error) {
	if expr == nil {
		return vectorstore.ErrMissingFilter
	}
	if err = expr.Validate(); err != nil {
		return fmt.Errorf("vespa.Store.DeleteWhere: %w", err)
	}

	filterFragment, err := s.buildFilter(expr)
	if err != nil {
		return err
	}
	if filterFragment == "" {
		return errors.New("vespa: refusing to delete on empty filter")
	}

	const pageSize = 500
	offset := 0
	for {
		yql := fmt.Sprintf("select %s from %s where %s",
			s.idField, s.schemaName, filterFragment)
		body := map[string]any{
			"yql":    yql,
			"hits":   pageSize,
			"offset": offset,
		}
		raw, err := s.do(ctx, http.MethodPost, "/search/", body)
		if err != nil {
			return fmt.Errorf("vespa: enumerate ids: %w", err)
		}
		var parsed struct {
			Root struct {
				Children []struct {
					Fields map[string]any `json:"fields"`
				} `json:"children"`
			} `json:"root"`
		}
		if err := json.Unmarshal(raw, &parsed); err != nil {
			return fmt.Errorf("vespa: decode id page: %w", err)
		}
		if len(parsed.Root.Children) == 0 {
			return nil
		}
		for _, hit := range parsed.Root.Children {
			id, _ := hit.Fields[s.idField].(string)
			if id == "" {
				continue
			}
			path := fmt.Sprintf("/document/v1/%s/%s/docid/%s",
				url.PathEscape(s.namespace), url.PathEscape(s.schemaName), url.PathEscape(id))
			if _, err := s.do(ctx, http.MethodDelete, path, nil); err != nil {
				return fmt.Errorf("vespa: delete %s: %w", id, err)
			}
		}
		if len(parsed.Root.Children) < pageSize {
			return nil
		}
		offset += len(parsed.Root.Children)
	}
}

func (s *Store) buildFilter(filter filter.Predicate) (string, error) {
	if filter == nil {
		return "", nil
	}
	v := NewVisitor("")
	if err := filter.Accept(v); err != nil {
		return "", fmt.Errorf("vespa: convert filter: %w", err)
	}
	return v.Result(), nil
}

func (s *Store) toDocument(rawID string, fields map[string]any) (*document.Document, error) {
	doc := &document.Document{}
	if id, ok := fields[s.idField].(string); ok {
		doc.ID = id
	} else {
		// Fall back to the Vespa-native id like "id:namespace:schema::docid".
		if idx := strings.LastIndex(rawID, "::"); idx > 0 {
			doc.ID = rawID[idx+2:]
		} else {
			doc.ID = rawID
		}
	}
	if doc.ID == "" {
		return nil, errors.New("vespa: search hit has no stable document ID")
	}
	text, ok := fields[s.contentField].(string)
	if !ok || text == "" {
		return nil, fmt.Errorf("vespa: document %q is missing string field %q", doc.ID, s.contentField)
	}
	doc.Text = text

	meta := make(map[string]any, len(fields))
	for k, v := range fields {
		switch k {
		case s.idField, s.contentField, s.embeddingField:
			continue
		}
		meta[k] = v
	}
	if len(meta) > 0 {
		var err error
		doc.Metadata, err = metadata.FromValues(meta)
		if err != nil {
			return nil, fmt.Errorf("vespa: convert metadata: %w", err)
		}
	}
	return doc, nil
}

// do executes a JSON request against the Vespa endpoint.
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

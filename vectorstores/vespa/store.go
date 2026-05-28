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

	"github.com/google/uuid"

	"github.com/Tangerg/lynx/core/document"
	"github.com/Tangerg/lynx/core/model/embedding"
	"github.com/Tangerg/lynx/core/vectorstore"
	"github.com/Tangerg/lynx/core/vectorstore/filter/ast"
	"github.com/Tangerg/lynx/pkg/math"
	"github.com/Tangerg/lynx/vectorstores/internal/ident"
	"github.com/Tangerg/lynx/vectorstores/internal/tracing"
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
)

// safeIdentifier matches names safe for Vespa document-id paths and
// schema identifiers — alphanumerics plus underscore and hyphen.

// StoreConfig contains configuration options for the Vespa vector
// store. Vespa uses an HTTP REST surface; the store assumes the
// schema (the .sd file) is provisioned out of band — Vespa schema
// management is YAML/SDL and lives in the application package.
type StoreConfig struct {
	Context context.Context

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

	// EmbeddingModel produces vectors for the documents. Required.
	EmbeddingModel embedding.Model

	// DocumentBatcher batches documents before upload. Required.
	DocumentBatcher document.Batcher

	// HTTPClient lets callers override transport (timeouts,
	// proxies, mTLS for Vespa Cloud). Optional: defaults to
	// http.DefaultClient.
	HTTPClient *http.Client
}

func (c StoreConfig) Validate() error {
	if c.Endpoint == "" {
		return errors.New("vespa: Endpoint is required")
	}
	if c.SchemaName == "" {
		return errors.New("vespa: SchemaName is required")
	}
	if c.EmbeddingModel == nil {
		return errors.New("vespa: EmbeddingModel is required")
	}
	if c.DocumentBatcher == nil {
		return errors.New("vespa: DocumentBatcher is required")
	}
	if err := ident.CheckWithDash("vespa", map[string]string{
		"SchemaName":     c.SchemaName,
		"Namespace":      c.Namespace,
		"EmbeddingField": c.EmbeddingField,
		"ContentField":   c.ContentField,
		"IDField":        c.IDField,
	}); err != nil {
		return err
	}
	if c.ContentCluster != "" && !ident.PatternWithDash.MatchString(c.ContentCluster) {
		return fmt.Errorf("vespa: ContentCluster=%q must be a safe identifier", c.ContentCluster)
	}
	return nil
}

// ApplyDefaults fills zero fields with documented defaults.
func (c *StoreConfig) ApplyDefaults() {
	if c.Context == nil {
		c.Context = context.Background()
	}
	if c.Namespace == "" {
		c.Namespace = c.SchemaName
	}
	c.EmbeddingField = cmp.Or(c.EmbeddingField, DefaultEmbeddingField)
	c.ContentField = cmp.Or(c.ContentField, DefaultContentField)
	c.IDField = cmp.Or(c.IDField, DefaultIDField)
	if c.HTTPClient == nil {
		c.HTTPClient = http.DefaultClient
	}
}

var _ vectorstore.Store = (*Store)(nil)

// Store is a Vespa-backed [vectorstore.Store] implementation talking
// to Vespa over its REST API.
type Store struct {
	endpoint        string
	schemaName      string
	namespace       string
	contentCluster  string
	embeddingField  string
	contentField    string
	idField         string
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
		return nil, fmt.Errorf("vespa: failed to create embedding client: %w", err)
	}
	return &Store{
		endpoint:        strings.TrimRight(config.Endpoint, "/"),
		schemaName:      config.SchemaName,
		namespace:       config.Namespace,
		contentCluster:  config.ContentCluster,
		embeddingField:  config.EmbeddingField,
		contentField:    config.ContentField,
		idField:         config.IDField,
		embeddingModel:  config.EmbeddingModel,
		embeddingClient: embeddingClient,
		documentBatcher: config.DocumentBatcher,
		httpClient:      config.HTTPClient,
	}, nil
}

// Create embeds documents and PUTs them through the Vespa Document
// API. Each PUT is `POST /document/v1/<namespace>/<schema>/docid/<id>`.
func (s *Store) Create(ctx context.Context, req *vectorstore.CreateRequest) (err error) {
	if err = req.Validate(); err != nil {
		return fmt.Errorf("vespa: invalid create request: %w", err)
	}

	ctx, span := tracing.StartCreate(ctx, "vespa", len(req.Documents))
	defer func() { tracing.Finish(span, err) }()

	var batchedDocs [][]*document.Document
	batchedDocs, err = s.documentBatcher.Batch(ctx, req.Documents)
	if err != nil {
		return fmt.Errorf("vespa: failed to batch documents: %w", err)
	}

	for _, docs := range batchedDocs {
		vectors, _, err := s.embeddingClient.
			EmbedWithDocuments(docs).
			Call().
			Embeddings(ctx)
		if err != nil {
			return fmt.Errorf("vespa: failed to generate embeddings: %w", err)
		}
		for i, doc := range docs {
			id := doc.ID
			if id == "" {
				id = uuid.NewString()
			}
			fields := map[string]any{
				s.idField:        id,
				s.contentField:   doc.Text,
				s.embeddingField: map[string]any{"values": math.ConvertSlice[float64, float32](vectors[i])},
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

// Retrieve runs a nearestNeighbor YQL query.
func (s *Store) Retrieve(ctx context.Context, req *vectorstore.RetrievalRequest) (docs []*document.Document, err error) {
	if err = req.Validate(); err != nil {
		return nil, fmt.Errorf("vespa: invalid retrieval request: %w", err)
	}

	ctx, span := tracing.StartRetrieve(ctx, "vespa", req.TopK, req.MinScore)
	defer func() { tracing.RecordRetrieveResult(span, err, len(docs)) }()

	var vector []float64
	vector, _, err = s.embeddingClient.
		EmbedWithText(req.Query).
		Call().
		Embedding(ctx)
	if err != nil {
		return nil, fmt.Errorf("vespa: failed to embed query: %w", err)
	}
	queryVec := math.ConvertSlice[float64, float32](vector)

	filterFragment, err := s.buildFilter(req.Filter)
	if err != nil {
		return nil, err
	}

	nn := fmt.Sprintf("{targetHits:%d}nearestNeighbor(%s, q)", req.TopK, s.embeddingField)
	yql := fmt.Sprintf("select * from %s where %s", s.schemaName, nn)
	if filterFragment != "" {
		yql = yql + " and " + filterFragment
	}

	body := map[string]any{
		"yql":            yql,
		"hits":           req.TopK,
		"input.query(q)": map[string]any{"values": queryVec},
		"ranking":        "default",
	}

	raw, err := s.do(ctx, http.MethodPost, "/search/", body)
	if err != nil {
		return nil, fmt.Errorf("vespa: search: %w", err)
	}

	var parsed struct {
		Root struct {
			Children []struct {
				ID        string         `json:"id"`
				Relevance float64        `json:"relevance"`
				Fields    map[string]any `json:"fields"`
			} `json:"children"`
		} `json:"root"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("vespa: decode search response: %w", err)
	}

	docs = make([]*document.Document, 0, len(parsed.Root.Children))
	for _, hit := range parsed.Root.Children {
		// Vespa relevance for nearestNeighbor is the configured
		// distance metric's similarity directly (cosine: [0, 1]).
		score := hit.Relevance
		if score < req.MinScore {
			continue
		}
		doc := s.toDocument(hit.ID, hit.Fields, score)
		docs = append(docs, doc)
	}
	return docs, nil
}

// Delete removes documents matching the filter expression via the
// `selection` parameter on the Document API.
//
// Vespa selection expressions live under their own mini language;
// rather than translate the AST a second way, we route through a
// YQL search to enumerate ids, then delete them.
func (s *Store) Delete(ctx context.Context, req *vectorstore.DeleteRequest) (err error) {
	if err = req.Validate(); err != nil {
		return fmt.Errorf("vespa: invalid delete request: %w", err)
	}

	ctx, span := tracing.StartDelete(ctx, "vespa")
	defer func() { tracing.Finish(span, err) }()

	filterFragment, err := s.buildFilter(req.Filter)
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

func (s *Store) buildFilter(filter ast.Expr) (string, error) {
	if filter == nil {
		return "", nil
	}
	v := NewVisitor("")
	v.Visit(filter)
	if err := v.Error(); err != nil {
		return "", fmt.Errorf("vespa: convert filter: %w", err)
	}
	return v.Result(), nil
}

func (s *Store) toDocument(rawID string, fields map[string]any, score float64) *document.Document {
	doc := &document.Document{Score: score}
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
	if text, ok := fields[s.contentField].(string); ok {
		doc.Text = text
	}

	meta := make(map[string]any, len(fields))
	for k, v := range fields {
		switch k {
		case s.idField, s.contentField, s.embeddingField:
			continue
		}
		meta[k] = v
	}
	if len(meta) > 0 {
		doc.Metadata = meta
	}
	return doc
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

func (s *Store) Metadata() vectorstore.StoreMetadata {
	return vectorstore.StoreMetadata{
		NativeClient: s.httpClient,
		Provider:     Provider,
	}
}

func (s *Store) Close() error { return nil }

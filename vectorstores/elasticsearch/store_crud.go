package elasticsearch

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	stdmath "math"

	"github.com/elastic/go-elasticsearch/v8/esapi"

	"github.com/Tangerg/lynx/core/document"
	"github.com/Tangerg/lynx/core/metadata"
	"github.com/Tangerg/lynx/core/vectorstore"
	"github.com/Tangerg/lynx/core/vectorstore/filter"
	"github.com/Tangerg/lynx/pkg/math"
	"github.com/Tangerg/lynx/vectorstores/internal/batching"
	"github.com/Tangerg/lynx/vectorstores/internal/docio"
	"github.com/Tangerg/lynx/vectorstores/internal/scores"
)

func (s *Store) Add(ctx context.Context, docs []*document.Document) (err error) {
	if err := docio.ValidateDocuments(docs); err != nil {
		return fmt.Errorf("elasticsearch.Store.Add: %w", err)
	}

	var batchedDocs [][]*document.Document
	batchedDocs, err = batching.Batch(ctx, s.documentBatcher, docs)
	if err != nil {
		return fmt.Errorf("elasticsearch: batch documents: %w", err)
	}

	for _, docs := range batchedDocs {
		vectors, err := s.embeddingClient.EmbedDocuments(ctx, docs)
		if err != nil {
			return fmt.Errorf("elasticsearch: embed documents: %w", err)
		}

		var body bytes.Buffer
		for i, doc := range docs {
			id := doc.ID

			actionLine, encErr := json.Marshal(map[string]any{
				"index": map[string]any{
					"_index": s.indexName,
					"_id":    id,
				},
			})
			if encErr != nil {
				return fmt.Errorf("elasticsearch: encode bulk action: %w", encErr)
			}

			docBody := map[string]any{
				s.contentField:   doc.Text,
				s.embeddingField: math.ConvertSlice[float64, float32](vectors[i]),
			}
			if s.metadataField != "" {
				docBody[s.metadataField] = doc.Metadata
			} else {
				for k, v := range doc.Metadata {
					docBody[k] = v
				}
			}
			docLine, encErr := json.Marshal(docBody)
			if encErr != nil {
				return fmt.Errorf("elasticsearch: encode bulk doc: %w", encErr)
			}

			body.Write(actionLine)
			body.WriteByte('\n')
			body.Write(docLine)
			body.WriteByte('\n')
		}

		resp, err := s.client.Bulk(
			bytes.NewReader(body.Bytes()),
			s.client.Bulk.WithContext(ctx),
		)
		if err != nil {
			return fmt.Errorf("elasticsearch: bulk: %w", err)
		}
		if err = parseBulkResponse(resp); err != nil {
			return err
		}
	}
	return nil
}

// Search runs a KNN search over the embedding field. Optional
// metadata filtering is expressed via a query_string clause.
func (s *Store) Search(ctx context.Context, req vectorstore.SearchRequest) (docs []vectorstore.Match, err error) {
	if err = req.Validate(); err != nil {
		return nil, fmt.Errorf("elasticsearch.Store.Search: %w", err)
	}

	defer func() {
		if err == nil {
			err = req.ValidateMatches(docs)
		}
	}()

	var vector []float64
	vector, err = s.embeddingClient.EmbedText(ctx, req.Query)
	if err != nil {
		return nil, fmt.Errorf("elasticsearch: embed query: %w", err)
	}
	queryVec := math.ConvertSlice[float64, float32](vector)

	knn := map[string]any{
		"field":          s.embeddingField,
		"query_vector":   queryVec,
		"k":              req.TopK,
		"num_candidates": int(stdmath.Ceil(float64(req.TopK) * s.numCandidatesMul)),
	}

	filterQuery, err := s.buildFilterQuery(req.Filter)
	if err != nil {
		return nil, err
	}
	if filterQuery != "" {
		knn["filter"] = map[string]any{
			"query_string": map[string]any{"query": filterQuery},
		}
	}

	body := map[string]any{
		"size": req.TopK,
		"knn":  knn,
	}
	buf, err := jsonReader(body)
	if err != nil {
		return nil, err
	}

	resp, err := s.client.Search(
		s.client.Search.WithContext(ctx),
		s.client.Search.WithIndex(s.indexName),
		s.client.Search.WithBody(buf),
	)
	if err != nil {
		return nil, fmt.Errorf("elasticsearch: search %s: %w", s.indexName, err)
	}
	defer resp.Body.Close()
	if resp.IsError() {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("elasticsearch: search %s: status=%d body=%s",
			s.indexName, resp.StatusCode, string(body))
	}

	var parsed searchResponse
	if err = json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("elasticsearch: decode search response: %w", err)
	}

	docs = make([]vectorstore.Match, 0, len(parsed.Hits.Hits))
	for _, hit := range parsed.Hits.Hits {
		score := s.normalizeScore(hit.Score)
		if score < req.MinScore {
			continue
		}
		doc, err := s.toDocument(hit)
		if err != nil {
			return nil, err
		}
		docs = append(docs, vectorstore.Match{Document: doc, Score: score})
	}
	return docs, nil
}

// Delete removes documents matching the filter via delete_by_query.
func (s *Store) DeleteWhere(ctx context.Context, expr filter.Predicate) (err error) {
	if expr == nil {
		return vectorstore.ErrMissingFilter
	}
	if err = filter.Validate(expr); err != nil {
		return fmt.Errorf("elasticsearch.Store.DeleteWhere: %w", err)
	}

	var filterQuery string
	filterQuery, err = s.buildFilterQuery(expr)
	if err != nil {
		return err
	}
	if filterQuery == "" {
		return errors.New("elasticsearch: refusing to delete on empty filter")
	}

	body := map[string]any{
		"query": map[string]any{
			"query_string": map[string]any{"query": filterQuery},
		},
	}
	buf, err := jsonReader(body)
	if err != nil {
		return err
	}

	resp, err := s.client.DeleteByQuery(
		[]string{s.indexName},
		buf,
		s.client.DeleteByQuery.WithContext(ctx),
	)
	if err != nil {
		return fmt.Errorf("elasticsearch: delete_by_query %s: %w", s.indexName, err)
	}
	defer resp.Body.Close()
	if resp.IsError() {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("elasticsearch: delete_by_query %s: status=%d body=%s",
			s.indexName, resp.StatusCode, string(respBody))
	}
	return nil
}

// DeleteIDs removes documents by their _id via a single bulk request
// carrying one delete action per id. An empty slice is a no-op; unknown
// ids are silently ignored (the bulk delete reports `not_found` rather
// than an error). Implements [vectorstore.IDDeleter].
func (s *Store) DeleteIDs(ctx context.Context, ids []string) (err error) {
	if len(ids) == 0 {
		return nil
	}

	var body bytes.Buffer
	for _, id := range ids {
		var actionLine []byte
		actionLine, err = json.Marshal(map[string]any{
			"delete": map[string]any{
				"_index": s.indexName,
				"_id":    id,
			},
		})
		if err != nil {
			return fmt.Errorf("elasticsearch: encode bulk delete action: %w", err)
		}
		body.Write(actionLine)
		body.WriteByte('\n')
	}

	resp, err := s.client.Bulk(
		bytes.NewReader(body.Bytes()),
		s.client.Bulk.WithContext(ctx),
	)
	if err != nil {
		return fmt.Errorf("elasticsearch: bulk delete: %w", err)
	}
	return parseBulkDeleteResponse(resp)
}

// buildFilterQuery converts the AST filter into a Lucene query string
// for `query_string`. Returns "" when filter is nil.
func (s *Store) buildFilterQuery(filter filter.Predicate) (string, error) {
	if filter == nil {
		return "", nil
	}
	v := NewVisitor(s.metadataField)
	if err := v.Visit(filter); err != nil {
		return "", fmt.Errorf("elasticsearch: convert filter: %w", err)
	}
	return v.Result(), nil
}

// normalizeScore validates Elasticsearch's already normalized dense-vector
// score. cosine and dot_product return (1+similarity)/2; l2_norm returns
// 1/(1+distance²). All three are in [0,1] with higher values ranked first.
func (s *Store) normalizeScore(score float64) float64 {
	return scores.Bounded(score)
}

func (s *Store) toDocument(hit searchHit) (*document.Document, error) {
	if hit.ID == "" {
		return nil, errors.New("elasticsearch: search hit is missing _id")
	}
	doc := &document.Document{ID: hit.ID}
	if hit.Source == nil {
		return nil, fmt.Errorf("elasticsearch: search hit %s is missing _source", hit.ID)
	}

	// Pull the document text from the configured content field.
	content, ok := hit.Source[s.contentField].(string)
	if !ok || content == "" {
		return nil, fmt.Errorf("elasticsearch: search hit %s is missing string field %q", hit.ID, s.contentField)
	}
	doc.Text = content

	if s.metadataField != "" {
		if rawMeta, ok := hit.Source[s.metadataField]; ok {
			if m, ok := rawMeta.(map[string]any); ok {
				var err error
				doc.Metadata, err = metadata.FromValues(m)
				if err != nil {
					return nil, fmt.Errorf("elasticsearch: convert metadata: %w", err)
				}
			}
		}
	} else {
		// Metadata was flattened onto the root — strip the
		// reserved fields and surface the rest.
		meta := make(map[string]any, len(hit.Source))
		for k, v := range hit.Source {
			if k == s.contentField || k == s.embeddingField {
				continue
			}
			meta[k] = v
		}
		if len(meta) > 0 {
			var err error
			doc.Metadata, err = metadata.FromValues(meta)
			if err != nil {
				return nil, fmt.Errorf("elasticsearch: convert metadata: %w", err)
			}
		}
	}
	return doc, nil
}

func (s *Store) Close() error { return nil }

// searchResponse / searchHit / bulkResponse mirror the slice of the
// Elasticsearch REST response the store actually consumes. We avoid
// the full typed client to keep the dependency footprint small.
type searchResponse struct {
	Hits struct {
		Hits []searchHit `json:"hits"`
	} `json:"hits"`
}

type searchHit struct {
	ID     string         `json:"_id"`
	Score  float64        `json:"_score"`
	Source map[string]any `json:"_source"`
}

type bulkResponse struct {
	Errors bool `json:"errors"`
	Items  []struct {
		Index *struct {
			ID     string         `json:"_id"`
			Status int            `json:"status"`
			Error  map[string]any `json:"error"`
		} `json:"index"`
	} `json:"items"`
}

func parseBulkResponse(resp *esapi.Response) error {
	defer resp.Body.Close()
	if resp.IsError() {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("elasticsearch: bulk: status=%d body=%s",
			resp.StatusCode, string(body))
	}
	var parsed bulkResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return fmt.Errorf("elasticsearch: decode bulk response: %w", err)
	}
	if !parsed.Errors {
		return nil
	}
	var firstErr, failedID string
	for _, item := range parsed.Items {
		if item.Index != nil && item.Index.Error != nil {
			failedID = item.Index.ID
			if reason, ok := item.Index.Error["reason"].(string); ok {
				firstErr = reason
			}
			break
		}
	}
	if firstErr == "" {
		firstErr = "unknown error"
	}
	return fmt.Errorf("elasticsearch: bulk failed on id=%s: %s", failedID, firstErr)
}

// bulkDeleteResponse mirrors the slice of a bulk response whose items
// carry a `delete` action. A missing id surfaces as status 404 with no
// error object, which is treated as success (idempotent delete).
type bulkDeleteResponse struct {
	Errors bool `json:"errors"`
	Items  []struct {
		Delete *struct {
			ID     string         `json:"_id"`
			Status int            `json:"status"`
			Error  map[string]any `json:"error"`
		} `json:"delete"`
	} `json:"items"`
}

func parseBulkDeleteResponse(resp *esapi.Response) error {
	defer resp.Body.Close()
	if resp.IsError() {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("elasticsearch: bulk delete: status=%d body=%s",
			resp.StatusCode, string(body))
	}
	var parsed bulkDeleteResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return fmt.Errorf("elasticsearch: decode bulk delete response: %w", err)
	}
	if !parsed.Errors {
		return nil
	}
	var firstErr, failedID string
	for _, item := range parsed.Items {
		if item.Delete != nil && item.Delete.Error != nil {
			failedID = item.Delete.ID
			if reason, ok := item.Delete.Error["reason"].(string); ok {
				firstErr = reason
			}
			break
		}
	}
	if firstErr == "" {
		firstErr = "unknown error"
	}
	return fmt.Errorf("elasticsearch: bulk delete failed on id=%s: %s", failedID, firstErr)
}

// jsonReader marshals v to JSON and returns it as an io.Reader.
func jsonReader(v any) (io.Reader, error) {
	buf, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("elasticsearch: encode request: %w", err)
	}
	return bytes.NewReader(buf), nil
}

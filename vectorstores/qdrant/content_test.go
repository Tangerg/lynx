package qdrant

import (
	"testing"

	qdrantclient "github.com/qdrant/go-client/qdrant"

	"github.com/Tangerg/scope/core/document"
)

func TestDocumentTextRoundTripsThroughReservedPayload(t *testing.T) {
	t.Parallel()

	store := &Store{distanceMetric: DistanceCosine}
	point, err := store.buildPointStruct(&document.Document{ID: "42", Text: "content"}, []float64{1, 0})
	if err != nil {
		t.Fatal(err)
	}
	if got := point.Payload[payloadDocumentContentKey].GetStringValue(); got != "content" {
		t.Fatalf("stored content = %q, want content", got)
	}

	matches, err := store.buildDocumentsFromPoints([]*qdrantclient.ScoredPoint{{
		Id:      point.Id,
		Payload: point.Payload,
		Score:   1,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 || matches[0].Document.Text != "content" {
		t.Fatalf("matches = %#v, want one document with content", matches)
	}
	if _, leaked := matches[0].Document.Metadata[payloadDocumentContentKey]; leaked {
		t.Fatal("reserved content payload leaked into document metadata")
	}
}

func TestQueryResultRequiresStoredDocumentText(t *testing.T) {
	t.Parallel()

	store := &Store{distanceMetric: DistanceCosine}
	_, err := store.buildDocumentsFromPoints([]*qdrantclient.ScoredPoint{{Id: qdrantclient.NewIDNum(42)}})
	if err == nil {
		t.Fatal("buildDocumentsFromPoints() accepted a result without document text")
	}
}

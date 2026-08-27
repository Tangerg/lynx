package pinecone

import (
	"testing"

	pineconeclient "github.com/pinecone-io/go-pinecone/v4/pinecone"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/Tangerg/scope/core/document"
)

func TestDocumentTextRoundTripsThroughReservedMetadata(t *testing.T) {
	t.Parallel()

	store := &Store{distanceMetric: DistanceCosine}
	vectors, err := store.buildVectors(
		[]*document.Document{{ID: "doc-1", Text: "content"}},
		[][]float64{{1, 0}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if got := vectors[0].Metadata.AsMap()[payloadDocumentContentKey]; got != "content" {
		t.Fatalf("stored content = %v, want content", got)
	}

	metadata, err := structpb.NewStruct(map[string]any{payloadDocumentContentKey: "content", "source": "test"})
	if err != nil {
		t.Fatal(err)
	}
	matches, err := store.buildDocumentsFromScoredVectors([]*pineconeclient.ScoredVector{{
		Vector: &pineconeclient.Vector{Id: "doc-1", Metadata: metadata},
		Score:  1,
	}}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 || matches[0].Document.Text != "content" {
		t.Fatalf("matches = %#v, want one document with content", matches)
	}
}

func TestQueryResultRequiresStoredDocumentText(t *testing.T) {
	t.Parallel()

	metadata, err := structpb.NewStruct(map[string]any{"source": "test"})
	if err != nil {
		t.Fatal(err)
	}
	store := &Store{distanceMetric: DistanceCosine}
	_, err = store.buildDocumentsFromScoredVectors([]*pineconeclient.ScoredVector{{
		Vector: &pineconeclient.Vector{Id: "doc-1", Metadata: metadata},
	}}, 0)
	if err == nil {
		t.Fatal("buildDocumentsFromScoredVectors() accepted a result without document text")
	}
}

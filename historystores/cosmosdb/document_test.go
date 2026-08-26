package cosmosdb

import (
	"slices"
	"testing"
)

func TestDocumentSequenceIsFixedWidthAndSortable(t *testing.T) {
	if got, want := formatSequence(42), "0000000000000000042"; got != want {
		t.Fatalf("formatSequence(42) = %q, want %q", got, want)
	}
	for _, sequence := range []string{"", "42", "000000000000000004x", "-000000000000000042"} {
		if validSequence(sequence) {
			t.Fatalf("validSequence(%q) = true", sequence)
		}
	}
	if !validSequence(formatSequence(42)) {
		t.Fatal("formatted sequence is invalid")
	}

	documents := []document{
		{ID: "b", Sequence: formatSequence(11)},
		{ID: "b", Sequence: formatSequence(10)},
		{ID: "a", Sequence: formatSequence(11)},
	}
	slices.SortFunc(documents, compareDocuments)
	if documents[0].Sequence != formatSequence(10) || documents[1].ID != "a" || documents[2].ID != "b" {
		t.Fatalf("sorted documents = %#v", documents)
	}
}

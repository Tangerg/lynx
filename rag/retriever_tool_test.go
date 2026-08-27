package rag_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/core/tool"
	"github.com/Tangerg/lynx/rag"
)

func TestRetrievalToolExposesStrictSchemaAndCandidates(t *testing.T) {
	doc := identifiedDocument(t, "go", "Go favors explicit composition.")
	retriever := &fakeRetriever{docs: []rag.Candidate{candidate(doc, 0.8)}}
	retrievalTool, err := rag.NewRetrievalTool(rag.RetrievalToolConfig{
		Name:        "search_knowledge",
		Description: "Search the project knowledge base.",
		Retriever:   retriever,
	})
	if err != nil {
		t.Fatal(err)
	}
	var executable tool.Tool = retrievalTool
	definition := executable.Definition()
	if definition.Name != "search_knowledge" || !strings.Contains(string(definition.InputSchema), `"query"`) {
		t.Fatalf("definition = %#v", definition)
	}

	raw, err := executable.Call(t.Context(), `{"query":"Go design"}`)
	if err != nil {
		t.Fatal(err)
	}
	var output rag.RetrievalToolOutput
	if err := json.Unmarshal([]byte(raw), &output); err != nil {
		t.Fatal(err)
	}
	if err := output.Validate(); err != nil {
		t.Fatal(err)
	}
	if len(output.Candidates) != 1 || output.Candidates[0].Document.ID != "go" || output.Candidates[0].Score != 0.8 {
		t.Fatalf("output = %#v", output)
	}
	if retriever.got != "Go design" {
		t.Fatalf("retriever query = %q", retriever.got)
	}
}

func TestRetrievalToolRejectsInvalidConfigurationAndArguments(t *testing.T) {
	if _, err := rag.NewRetrievalTool(rag.RetrievalToolConfig{}); !errors.Is(err, rag.ErrNilRetriever) {
		t.Fatalf("nil retriever error = %v", err)
	}
	if _, err := rag.NewRetrievalTool(rag.RetrievalToolConfig{Retriever: &fakeRetriever{}}); !errors.Is(err, tool.ErrInvalidTool) {
		t.Fatalf("invalid definition error = %v", err)
	}

	retrievalTool, err := rag.NewRetrievalTool(rag.RetrievalToolConfig{
		Name: "search", Description: "Search evidence.", Retriever: &fakeRetriever{},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, arguments := range []string{
		`{}`,
		`{"query":""}`,
		`{"query":"valid","unexpected":true}`,
	} {
		if _, err := retrievalTool.Call(t.Context(), arguments); err == nil {
			t.Fatalf("Call(%s) succeeded", arguments)
		}
	}
}

func TestRetrievalToolPreservesRetrieverErrors(t *testing.T) {
	want := errors.New("retrieval failed")
	retrievalTool, err := rag.NewRetrievalTool(rag.RetrievalToolConfig{
		Name:        "search",
		Description: "Search evidence.",
		Retriever: rag.RetrieverFunc(func(context.Context, rag.Query) (rag.Candidates, error) {
			return nil, want
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := retrievalTool.Call(t.Context(), `{"query":"question"}`); !errors.Is(err, want) {
		t.Fatalf("retrieval error = %v", err)
	}
}

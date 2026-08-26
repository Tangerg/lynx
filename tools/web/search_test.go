package web

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// fakeSearcher is a test double for the [Searcher] SPI. It records
// the last request it received and returns a canned response. This
// is the only mocking in the package — searcher impls themselves are
// tested against the real upstream via env-keyed integration tests.
type fakeSearcher struct {
	last *SearchRequest
	resp *SearchResponse
	err  error
}

func (f *fakeSearcher) Search(_ context.Context, req *SearchRequest) (*SearchResponse, error) {
	f.last = req
	if f.err != nil {
		return nil, f.err
	}
	return f.resp, nil
}

func TestSearchNewTool_NilSearcher(t *testing.T) {
	_, err := NewSearchTool(nil)
	if !errors.Is(err, ErrMissingSearcher) {
		t.Errorf("NewSearchTool(nil): err = %v, want ErrMissingSearcher", err)
	}
}

func TestSearchTool_Definition(t *testing.T) {
	tool, err := NewSearchTool(&fakeSearcher{})
	if err != nil {
		t.Fatal(err)
	}
	def := tool.Definition()
	if def.Name != "web_search" {
		t.Errorf("Name = %q, want %q", def.Name, "web_search")
	}
	if len(def.InputSchema) == 0 {
		t.Error("InputSchema is empty")
	}
}

func TestSearchTool_Call_HappyPath(t *testing.T) {
	searcher := &fakeSearcher{

		resp: &SearchResponse{
			Query: "kittens",
			Results: []*SearchResult{
				{Title: "Cats", URL: "https://example.com/cats", Snippet: "purr"},
			},
		},
	}
	tool, _ := NewSearchTool(searcher)

	body, err := tool.Call(t.Context(), `{"query":"kittens","max_results":3,"recency":"week"}`)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	var resp SearchResponse
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatalf("Unmarshal: %v\nbody=%s", err, body)
	}
	if resp.Query != "kittens" {
		t.Errorf("SearchResponse.Query = %q", resp.Query)
	}
	if len(resp.Results) != 1 || resp.Results[0].Title != "Cats" {
		t.Errorf("SearchResponse.Results = %+v", resp.Results)
	}

	if searcher.last == nil {
		t.Fatal("searcher.Search not called")
	}
	if searcher.last.Query != "kittens" {
		t.Errorf("searcher got query %q", searcher.last.Query)
	}
	if searcher.last.MaxResults != 3 {
		t.Errorf("MaxResults forwarded = %d, want 3", searcher.last.MaxResults)
	}
	if searcher.last.Recency != RecencyWeek {
		t.Errorf("Recency forwarded = %q, want week", searcher.last.Recency)
	}
}

func TestSearchTool_Call_EmptyQuery(t *testing.T) {
	tool, _ := NewSearchTool(&fakeSearcher{})
	_, err := tool.Call(t.Context(), `{"query":""}`)
	if err == nil {
		t.Fatal("Call empty query: want schema error")
	}
	_, err = tool.Call(t.Context(), `{"query":"   "}`)
	if !errors.Is(err, ErrEmptyQuery) {
		t.Errorf("Call blank query: err = %v, want ErrEmptyQuery", err)
	}
}

func TestSearchTool_Call_BadJSON(t *testing.T) {
	tool, _ := NewSearchTool(&fakeSearcher{})
	if _, err := tool.Call(t.Context(), `{bad json`); err == nil {
		t.Fatal("want error on bad JSON")
	}
}

func TestSearchTool_Call_EnforcesAdvertisedContract(t *testing.T) {
	searcher := &fakeSearcher{}
	tool, _ := NewSearchTool(searcher)
	for _, arguments := range []string{
		`{"query":"runtime","limit":3}`,
		`{"query":"runtime","max_results":21}`,
		`{"query":"runtime","recency":"recent"}`,
	} {
		searcher.last = nil
		if _, err := tool.Call(t.Context(), arguments); err == nil {
			t.Errorf("Call(%s): want contract error", arguments)
		}
		if searcher.last != nil {
			t.Errorf("Call(%s): invalid arguments reached searcher", arguments)
		}
	}
}

func TestSearchTool_Call_DomainsMutuallyExclusive(t *testing.T) {
	tool, _ := NewSearchTool(&fakeSearcher{})
	_, err := tool.Call(t.Context(),
		`{"query":"x","allowed_domains":["a.com"],"blocked_domains":["b.com"]}`)
	if !errors.Is(err, ErrDomainsBothSides) {
		t.Errorf("err = %v, want ErrDomainsBothSides", err)
	}
}

func TestSearchTool_Call_SearcherError(t *testing.T) {
	searcher := &fakeSearcher{err: errors.New("upstream boom")}
	tool, _ := NewSearchTool(searcher)
	_, err := tool.Call(t.Context(), `{"query":"hello"}`)
	if err == nil {
		t.Fatal("want error when searcher fails")
	}
	if !strings.Contains(err.Error(), "upstream boom") {
		t.Errorf("err = %v, want wrapped 'upstream boom'", err)
	}
}

func TestSearchRequest_QueryWithSiteOperators(t *testing.T) {
	cases := []struct {
		name    string
		query   string
		allowed []string
		blocked []string
		want    string
	}{
		{"plain", "kittens", nil, nil, "kittens"},
		{"allow one", "kittens", []string{"reddit.com"}, nil, "kittens site:reddit.com"},
		{"allow many", "kittens", []string{"a.com", "b.com"}, nil, "kittens site:a.com site:b.com"},
		{"block one", "kittens", nil, []string{"pinterest.com"}, "kittens -site:pinterest.com"},
		{"both (caller filters)", "x", []string{"a"}, []string{"b"}, "x site:a -site:b"},
		{"skip empty strings", "x", []string{"", "a", ""}, []string{""}, "x site:a"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			request := &SearchRequest{Query: tc.query, AllowedDomains: tc.allowed, BlockedDomains: tc.blocked}
			got := request.QueryWithSiteOperators()
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestSearchRecency_Validate(t *testing.T) {
	if err := RecencyWeek.Validate(); err != nil {
		t.Fatalf("RecencyWeek.Validate() error = %v", err)
	}
	if err := Recency("recent").Validate(); !errors.Is(err, ErrInvalidRecency) {
		t.Fatalf("Recency(recent).Validate() error = %v, want ErrInvalidRecency", err)
	}
}

func TestSearchTool_Call_DomainsForwarded(t *testing.T) {
	searcher := &fakeSearcher{resp: &SearchResponse{Query: "q", Results: nil}}
	tool, _ := NewSearchTool(searcher)
	if _, err := tool.Call(t.Context(),
		`{"query":"x","allowed_domains":["github.com","stackoverflow.com"]}`); err != nil {
		t.Fatalf("Call: %v", err)
	}
	if got := searcher.last.AllowedDomains; len(got) != 2 || got[0] != "github.com" {
		t.Errorf("AllowedDomains forwarded = %v", got)
	}
}

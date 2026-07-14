package websearch

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// fakeProvider is a test double for the [Provider] SPI. It records
// the last request it received and returns a canned response. This
// is the only mocking in the package — provider impls themselves are
// tested against the real upstream via env-keyed integration tests.
type fakeProvider struct {
	name string
	last *Request
	resp *Response
	err  error
}

func (f *fakeProvider) Name() string { return f.name }

func (f *fakeProvider) Search(_ context.Context, req *Request) (*Response, error) {
	f.last = req
	if f.err != nil {
		return nil, f.err
	}
	return f.resp, nil
}

func TestNewTool_NilProvider(t *testing.T) {
	_, err := NewTool(nil)
	if !errors.Is(err, ErrMissingProvider) {
		t.Errorf("NewTool(nil): err = %v, want ErrMissingProvider", err)
	}
}

func TestTool_Definition(t *testing.T) {
	tool, err := NewTool(&fakeProvider{name: "stub"})
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

func TestTool_Call_HappyPath(t *testing.T) {
	prov := &fakeProvider{
		name: "stub",
		resp: &Response{
			Query: "kittens",
			Results: []*Result{
				{Title: "Cats", URL: "https://example.com/cats", Snippet: "purr"},
			},
		},
	}
	tool, _ := NewTool(prov)

	body, err := tool.Call(t.Context(), `{"query":"kittens","max_results":3,"recency":"week"}`)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	var resp Response
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatalf("Unmarshal: %v\nbody=%s", err, body)
	}
	if resp.Query != "kittens" {
		t.Errorf("Response.Query = %q", resp.Query)
	}
	if len(resp.Results) != 1 || resp.Results[0].Title != "Cats" {
		t.Errorf("Response.Results = %+v", resp.Results)
	}

	if prov.last == nil {
		t.Fatal("provider.Search not called")
	}
	if prov.last.Query != "kittens" {
		t.Errorf("provider got query %q", prov.last.Query)
	}
	if prov.last.MaxResults != 3 {
		t.Errorf("MaxResults forwarded = %d, want 3", prov.last.MaxResults)
	}
	if prov.last.Recency != RecencyWeek {
		t.Errorf("Recency forwarded = %q, want week", prov.last.Recency)
	}
}

func TestTool_Call_EmptyQuery(t *testing.T) {
	tool, _ := NewTool(&fakeProvider{name: "stub"})
	_, err := tool.Call(t.Context(), `{"query":""}`)
	if !errors.Is(err, ErrEmptyQuery) {
		t.Errorf("Call empty query: err = %v, want ErrEmptyQuery", err)
	}
}

func TestTool_Call_BadJSON(t *testing.T) {
	tool, _ := NewTool(&fakeProvider{name: "stub"})
	if _, err := tool.Call(t.Context(), `{bad json`); err == nil {
		t.Fatal("want error on bad JSON")
	}
}

func TestTool_Call_DomainsMutuallyExclusive(t *testing.T) {
	tool, _ := NewTool(&fakeProvider{name: "stub"})
	_, err := tool.Call(t.Context(),
		`{"query":"x","allowed_domains":["a.com"],"blocked_domains":["b.com"]}`)
	if !errors.Is(err, ErrDomainsBothSides) {
		t.Errorf("err = %v, want ErrDomainsBothSides", err)
	}
}

func TestTool_Call_ProviderError(t *testing.T) {
	prov := &fakeProvider{name: "stub", err: errors.New("upstream boom")}
	tool, _ := NewTool(prov)
	_, err := tool.Call(t.Context(), `{"query":"hello"}`)
	if err == nil {
		t.Fatal("want error when provider fails")
	}
	if !strings.Contains(err.Error(), "upstream boom") {
		t.Errorf("err = %v, want wrapped 'upstream boom'", err)
	}
}

func TestBuildSiteOperatorQuery(t *testing.T) {
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
			got := BuildSiteOperatorQuery(tc.query, tc.allowed, tc.blocked)
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestTool_Call_DomainsForwarded(t *testing.T) {
	prov := &fakeProvider{name: "stub", resp: &Response{Query: "q", Results: nil}}
	tool, _ := NewTool(prov)
	if _, err := tool.Call(t.Context(),
		`{"query":"x","allowed_domains":["github.com","stackoverflow.com"]}`); err != nil {
		t.Fatalf("Call: %v", err)
	}
	if got := prov.last.AllowedDomains; len(got) != 2 || got[0] != "github.com" {
		t.Errorf("AllowedDomains forwarded = %v", got)
	}
}

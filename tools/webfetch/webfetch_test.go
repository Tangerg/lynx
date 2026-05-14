package webfetch

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

type fakeProvider struct {
	name string
	last *Request
	resp *Response
	err  error
}

func (f *fakeProvider) Name() string { return f.name }

func (f *fakeProvider) Fetch(_ context.Context, req *Request) (*Response, error) {
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
	if def.Name != "web_fetch" {
		t.Errorf("Name = %q, want %q", def.Name, "web_fetch")
	}
	if def.InputSchema == "" {
		t.Error("InputSchema is empty")
	}
}

func TestTool_Call_HappyPath(t *testing.T) {
	prov := &fakeProvider{
		name: "stub",
		resp: &Response{Content: "# Hello", Format: FormatMarkdown},
	}
	tool, _ := NewTool(prov)
	body, err := tool.Call(t.Context(), `{"url":"https://example.com","format":"markdown"}`)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	var resp Response
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatalf("Unmarshal: %v body=%s", err, body)
	}
	if resp.Content != "# Hello" {
		t.Errorf("Content = %q", resp.Content)
	}
	if resp.Format != FormatMarkdown {
		t.Errorf("Format = %q", resp.Format)
	}
	if prov.last == nil || prov.last.URL != "https://example.com" {
		t.Errorf("provider.Fetch not called as expected: %+v", prov.last)
	}
	if prov.last.Format != FormatMarkdown {
		t.Errorf("Format forwarded = %q", prov.last.Format)
	}
}

func TestTool_Call_EmptyURL(t *testing.T) {
	tool, _ := NewTool(&fakeProvider{name: "stub"})
	_, err := tool.Call(t.Context(), `{"url":""}`)
	if !errors.Is(err, ErrEmptyURL) {
		t.Errorf("Call empty url: err = %v, want ErrEmptyURL", err)
	}
}

func TestTool_Call_BadJSON(t *testing.T) {
	tool, _ := NewTool(&fakeProvider{name: "stub"})
	if _, err := tool.Call(t.Context(), `{bad json`); err == nil {
		t.Fatal("want error on bad JSON")
	}
}

func TestTool_Call_ProviderError(t *testing.T) {
	prov := &fakeProvider{name: "stub", err: errors.New("fetch boom")}
	tool, _ := NewTool(prov)
	_, err := tool.Call(t.Context(), `{"url":"https://example.com"}`)
	if err == nil {
		t.Fatal("want error when provider fails")
	}
	if !strings.Contains(err.Error(), "fetch boom") {
		t.Errorf("err = %v, want wrapped 'fetch boom'", err)
	}
}

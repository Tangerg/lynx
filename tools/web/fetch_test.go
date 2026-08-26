package web

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

type fakeFetcher struct {
	last *FetchRequest
	resp *FetchResponse
	err  error
}

func (f *fakeFetcher) Fetch(_ context.Context, req *FetchRequest) (*FetchResponse, error) {
	f.last = req
	if f.err != nil {
		return nil, f.err
	}
	return f.resp, nil
}

func TestFetchNewTool_NilFetcher(t *testing.T) {
	_, err := NewFetchTool(nil)
	if !errors.Is(err, ErrMissingFetcher) {
		t.Errorf("NewFetchTool(nil): err = %v, want ErrMissingFetcher", err)
	}
}

func TestFetchTool_Definition(t *testing.T) {
	tool, err := NewFetchTool(&fakeFetcher{})
	if err != nil {
		t.Fatal(err)
	}
	def := tool.Definition()
	if def.Name != "web_fetch" {
		t.Errorf("Name = %q, want %q", def.Name, "web_fetch")
	}
	if len(def.InputSchema) == 0 {
		t.Error("InputSchema is empty")
	}
}

func TestFetchTool_Call_HappyPath(t *testing.T) {
	fetcher := &fakeFetcher{

		resp: &FetchResponse{Content: "# Hello", Format: FormatMarkdown},
	}
	tool, _ := NewFetchTool(fetcher)
	body, err := tool.Call(t.Context(), `{"url":"https://example.com","format":"markdown"}`)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	var resp FetchResponse
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatalf("Unmarshal: %v body=%s", err, body)
	}
	if resp.Content != "# Hello" {
		t.Errorf("Content = %q", resp.Content)
	}
	if resp.Format != FormatMarkdown {
		t.Errorf("Format = %q", resp.Format)
	}
	if fetcher.last == nil || fetcher.last.URL != "https://example.com" {
		t.Errorf("fetcher.Fetch not called as expected: %+v", fetcher.last)
	}
	if fetcher.last.Format != FormatMarkdown {
		t.Errorf("Format forwarded = %q", fetcher.last.Format)
	}
}

func TestFetchTool_Call_EmptyURL(t *testing.T) {
	tool, _ := NewFetchTool(&fakeFetcher{})
	_, err := tool.Call(t.Context(), `{"url":""}`)
	if err == nil {
		t.Fatal("Call empty url: want schema error")
	}
	_, err = tool.Call(t.Context(), `{"url":"   "}`)
	if !errors.Is(err, ErrEmptyURL) {
		t.Errorf("Call blank url: err = %v, want ErrEmptyURL", err)
	}
}

func TestFetchTool_Call_BadJSON(t *testing.T) {
	tool, _ := NewFetchTool(&fakeFetcher{})
	if _, err := tool.Call(t.Context(), `{bad json`); err == nil {
		t.Fatal("want error on bad JSON")
	}
}

func TestFetchTool_Call_EnforcesAdvertisedContract(t *testing.T) {
	fetcher := &fakeFetcher{}
	tool, _ := NewFetchTool(fetcher)
	for _, arguments := range []string{
		`{"url":"https://example.com","response_format":"text"}`,
		`{"url":"https://example.com","format":"json"}`,
		`{"url":"relative/path"}`,
	} {
		fetcher.last = nil
		if _, err := tool.Call(t.Context(), arguments); err == nil {
			t.Errorf("Call(%s): want contract error", arguments)
		}
		if fetcher.last != nil {
			t.Errorf("Call(%s): invalid arguments reached fetcher", arguments)
		}
	}
}

func TestFetchTool_Call_FetcherError(t *testing.T) {
	fetcher := &fakeFetcher{err: errors.New("fetch boom")}
	tool, _ := NewFetchTool(fetcher)
	_, err := tool.Call(t.Context(), `{"url":"https://example.com"}`)
	if err == nil {
		t.Fatal("want error when fetcher fails")
	}
	if !strings.Contains(err.Error(), "fetch boom") {
		t.Errorf("err = %v, want wrapped 'fetch boom'", err)
	}
}

func TestFetchContentFormat(t *testing.T) {
	if got := ContentFormat("").Resolve(); got != FormatMarkdown {
		t.Fatalf("empty ContentFormat.Resolve() = %q, want %q", got, FormatMarkdown)
	}
	if err := FormatText.Validate(); err != nil {
		t.Fatalf("FormatText.Validate() error = %v", err)
	}
	if err := ContentFormat("json").Validate(); !errors.Is(err, ErrInvalidFormat) {
		t.Fatalf("ContentFormat(json).Validate() error = %v, want ErrInvalidFormat", err)
	}
}

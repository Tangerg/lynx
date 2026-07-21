package llm

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
)

func TestListRemoteModels(t *testing.T) {
	var gotPath, gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"qwen2.5"},{"id":"llama3.2"},{"id":""},{"id":"llama3.2"}]}`))
	}))
	defer srv.Close()

	// A trailing slash on the base URL must not double up before /models.
	ids, err := ListRemoteModels(t.Context(), srv.URL+"/v1/", "sk-test")
	if err != nil {
		t.Fatalf("ListRemoteModels: %v", err)
	}
	// sorted, de-duplicated, empty id dropped.
	if want := []string{"llama3.2", "qwen2.5"}; !slices.Equal(ids, want) {
		t.Fatalf("ids = %v, want %v", ids, want)
	}
	if gotPath != "/v1/models" {
		t.Fatalf("probed path = %q, want /v1/models", gotPath)
	}
	if gotAuth != "Bearer sk-test" {
		t.Fatalf("auth header = %q, want Bearer sk-test", gotAuth)
	}
}

func TestListRemoteModelsNoKeyOmitsAuth(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer srv.Close()

	if _, err := ListRemoteModels(t.Context(), srv.URL, ""); err != nil {
		t.Fatalf("ListRemoteModels: %v", err)
	}
	if gotAuth != "" {
		t.Fatalf("auth header = %q, want none for a keyless local daemon", gotAuth)
	}
}

func TestListRemoteModelsNon200IsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	if _, err := ListRemoteModels(t.Context(), srv.URL, ""); err == nil {
		t.Fatal("expected an error on a non-200 probe, got nil")
	}
}

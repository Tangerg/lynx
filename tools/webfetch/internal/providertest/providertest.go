// Package providertest is a tiny shared harness for webfetch
// provider tests. Each provider's _test.go calls [Run] with its
// constructor and env-var name; all the boilerplate (skip-when-key-
// missing, smoke-fetch, JSON dump) lives here once.
package providertest

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/Tangerg/lynx/tools/webfetch"
)

// Factory builds a [webfetch.Provider] from an API key string.
type Factory func(apiKey string) (webfetch.Provider, error)

// Run executes the standard two-subtest suite for a webfetch
// provider:
//   - RequiresAPIKey: NewClient with an empty key must error
//   - Fetch: skip when envKey is unset; otherwise hit the live API
//     against https://example.com and log the marshalled response
//
// envKey is the OS environment variable holding the provider's API
// key (e.g., "JINA_KEY", "FIRECRAWL_KEY").
func Run(t *testing.T, envKey string, newClient Factory) {
	t.Helper()

	t.Run("RequiresAPIKey", func(t *testing.T) {
		if _, err := newClient(""); err == nil {
			t.Error("want error when APIKey is empty")
		}
	})

	t.Run("Fetch", func(t *testing.T) {
		key := os.Getenv(envKey)
		if key == "" {
			t.Skip(envKey + " not set")
		}
		c, err := newClient(key)
		if err != nil {
			t.Fatal(err)
		}
		resp, err := c.Fetch(t.Context(), &webfetch.Request{URL: "https://example.com"})
		if err != nil {
			t.Fatalf("Fetch: %v", err)
		}
		out, _ := json.MarshalIndent(resp, "", "  ")
		t.Log(string(out))
	})
}

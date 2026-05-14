// Package providertest is a tiny shared harness for websearch
// provider tests. Each provider's _test.go calls [Run] with its
// constructor and env-var name; all the boilerplate (skip-when-key-
// missing, smoke-search, JSON dump) lives here once.
package providertest

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/Tangerg/lynx/tools/websearch"
)

// Factory builds a [websearch.Provider] from an API key string.
type Factory func(apiKey string) (websearch.Provider, error)

// Run executes the standard two-subtest suite for a websearch
// provider:
//   - RequiresAPIKey: NewClient with an empty key must error
//   - Search: skip when envKey is unset; otherwise hit the live API
//     and log the marshalled response
//
// envKey is the OS environment variable holding the provider's API
// key (e.g., "TAVILY_KEY", "BRAVE_KEY").
func Run(t *testing.T, envKey string, newClient Factory) {
	t.Helper()

	t.Run("RequiresAPIKey", func(t *testing.T) {
		if _, err := newClient(""); err == nil {
			t.Error("want error when APIKey is empty")
		}
	})

	t.Run("Search", func(t *testing.T) {
		key := os.Getenv(envKey)
		if key == "" {
			t.Skip(envKey + " not set")
		}
		c, err := newClient(key)
		if err != nil {
			t.Fatal(err)
		}
		resp, err := c.Search(t.Context(), &websearch.Request{Query: "bitcoin", MaxResults: 3})
		if err != nil {
			t.Fatalf("Search: %v", err)
		}
		out, _ := json.MarshalIndent(resp, "", "  ")
		t.Log(string(out))
	})
}

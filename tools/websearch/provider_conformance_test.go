package websearch_test

import (
	"errors"
	"testing"

	"github.com/Tangerg/lynx/tools/websearch"
	"github.com/Tangerg/lynx/tools/websearch/brave"
	"github.com/Tangerg/lynx/tools/websearch/exa"
	"github.com/Tangerg/lynx/tools/websearch/firecrawl"
	"github.com/Tangerg/lynx/tools/websearch/jina"
	"github.com/Tangerg/lynx/tools/websearch/perplexity"
	"github.com/Tangerg/lynx/tools/websearch/serper"
	"github.com/Tangerg/lynx/tools/websearch/tavily"
)

func TestProvidersValidateRequests(t *testing.T) {
	constructors := map[string]func() (websearch.Provider, error){
		"brave":      func() (websearch.Provider, error) { return brave.NewClient(brave.Config{APIKey: "test"}) },
		"exa":        func() (websearch.Provider, error) { return exa.NewClient(exa.Config{APIKey: "test"}) },
		"firecrawl":  func() (websearch.Provider, error) { return firecrawl.NewClient(firecrawl.Config{APIKey: "test"}) },
		"jina":       func() (websearch.Provider, error) { return jina.NewClient(jina.Config{APIKey: "test"}) },
		"perplexity": func() (websearch.Provider, error) { return perplexity.NewClient(perplexity.Config{APIKey: "test"}) },
		"serper":     func() (websearch.Provider, error) { return serper.NewClient(serper.Config{APIKey: "test"}) },
		"tavily":     func() (websearch.Provider, error) { return tavily.NewClient(tavily.Config{APIKey: "test"}) },
	}
	for name, constructor := range constructors {
		t.Run(name, func(t *testing.T) {
			provider, err := constructor()
			if err != nil {
				t.Fatalf("construct provider: %v", err)
			}
			if _, err := provider.Search(t.Context(), nil); !errors.Is(err, websearch.ErrMissingRequest) {
				t.Fatalf("Search(nil) error = %v, want ErrMissingRequest", err)
			}
			if _, err := provider.Search(t.Context(), &websearch.Request{}); !errors.Is(err, websearch.ErrEmptyQuery) {
				t.Fatalf("Search(empty) error = %v, want ErrEmptyQuery", err)
			}
		})
	}
}

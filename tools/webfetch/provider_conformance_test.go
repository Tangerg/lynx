package webfetch_test

import (
	"errors"
	"testing"

	"github.com/Tangerg/lynx/tools/webfetch"
	"github.com/Tangerg/lynx/tools/webfetch/exa"
	"github.com/Tangerg/lynx/tools/webfetch/firecrawl"
	"github.com/Tangerg/lynx/tools/webfetch/jina"
	"github.com/Tangerg/lynx/tools/webfetch/tavily"
)

func TestProvidersValidateRequests(t *testing.T) {
	constructors := map[string]func() (webfetch.Provider, error){
		"exa":       func() (webfetch.Provider, error) { return exa.NewClient(exa.Config{APIKey: "test"}) },
		"firecrawl": func() (webfetch.Provider, error) { return firecrawl.NewClient(firecrawl.Config{APIKey: "test"}) },
		"jina":      func() (webfetch.Provider, error) { return jina.NewClient(jina.Config{APIKey: "test"}) },
		"tavily":    func() (webfetch.Provider, error) { return tavily.NewClient(tavily.Config{APIKey: "test"}) },
	}
	for name, constructor := range constructors {
		t.Run(name, func(t *testing.T) {
			provider, err := constructor()
			if err != nil {
				t.Fatalf("construct provider: %v", err)
			}
			if _, err := provider.Fetch(t.Context(), nil); !errors.Is(err, webfetch.ErrMissingRequest) {
				t.Fatalf("Fetch(nil) error = %v, want ErrMissingRequest", err)
			}
			if _, err := provider.Fetch(t.Context(), &webfetch.Request{}); !errors.Is(err, webfetch.ErrEmptyURL) {
				t.Fatalf("Fetch(empty) error = %v, want ErrEmptyURL", err)
			}
		})
	}
}

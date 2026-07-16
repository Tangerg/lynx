package tavily

import (
	"testing"

	"github.com/Tangerg/lynx/tools/websearch"
	"github.com/Tangerg/lynx/tools/websearch/internal/providertest"
)

func TestProvider(t *testing.T) {
	providertest.Run(t, "TAVILY_KEY", func(k string) (websearch.Provider, error) {
		return NewClient(Config{APIKey: k})
	})
}

package tavily

import (
	"testing"

	"github.com/Tangerg/lynx/tools/webfetch"
	"github.com/Tangerg/lynx/tools/webfetch/internal/providertest"
)

func TestProvider(t *testing.T) {
	providertest.Run(t, "TAVILY_KEY", func(k string) (webfetch.Provider, error) {
		return NewClient(Config{APIKey: k})
	})
}

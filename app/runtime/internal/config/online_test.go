package config

import (
	"reflect"
	"strings"
	"testing"

	"github.com/spf13/viper"
)

func TestLoadOnline_EnvOverridesYAML(t *testing.T) {
	t.Setenv("LYRA_JINA_API_KEY", "jina-env")
	t.Setenv("LYRA_TAVILY_API_KEY", "tavily-env")
	t.Setenv("LYRA_HTTP_ALLOWED_HOSTS", "api.github.com, *.example.com ")
	t.Setenv("LYRA_SOURCEGRAPH_ENDPOINT", "https://sourcegraph.example.com")
	t.Setenv("LYRA_SOURCEGRAPH_TOKEN", "sg-env")

	v := viper.New()
	v.SetConfigType("yaml")
	if err := v.ReadConfig(strings.NewReader(`
online:
  jinaApiKey: jina-yaml
  tavilyApiKey: tavily-yaml
  httpAllowedHosts: ["yaml.example.com"]
  sourcegraphEndpoint: https://yaml-sourcegraph.example.com
  sourcegraphToken: sg-yaml
`)); err != nil {
		t.Fatalf("read config: %v", err)
	}

	got := loadOnline(v)
	want := OnlineConfig{
		JinaAPIKey:          "jina-env",
		TavilyAPIKey:        "tavily-env",
		HTTPAllowedHosts:    []string{"api.github.com", "*.example.com"},
		SourcegraphEndpoint: "https://sourcegraph.example.com",
		SourcegraphToken:    "sg-env",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("loadOnline = %+v, want %+v", got, want)
	}
}

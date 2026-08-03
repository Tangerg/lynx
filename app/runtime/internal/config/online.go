package config

import (
	"cmp"
	"os"
	"strings"

	"github.com/spf13/viper"
)

// loadOnline reads the optional provider-tool credentials. yaml under
// `online:`; the LYRA_* env vars take precedence over yaml, matching
// the overall source ordering (env over file).
func loadOnline(v *viper.Viper) OnlineConfig {
	jina := cmp.Or(os.Getenv("LYRA_JINA_API_KEY"), v.GetString("online.jinaApiKey"))
	tavily := cmp.Or(os.Getenv("LYRA_TAVILY_API_KEY"), v.GetString("online.tavilyApiKey"))
	hosts := v.GetStringSlice("online.httpAllowedHosts")
	if env := os.Getenv("LYRA_HTTP_ALLOWED_HOSTS"); env != "" {
		hosts = splitHosts(env)
	}
	return OnlineConfig{
		JinaAPIKey:       jina,
		TavilyAPIKey:     tavily,
		HTTPAllowedHosts: hosts,
	}
}

// splitHosts parses the comma-separated LYRA_HTTP_ALLOWED_HOSTS value.
func splitHosts(raw string) []string {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
